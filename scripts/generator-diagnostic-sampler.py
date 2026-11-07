#!/usr/bin/env python3
"""Capture one-second load-host scheduling, cgroup, socket, and k6 telemetry during a run.

Run alongside benchctl on the load host. Does not change workload or container settings.
The raw JSONL is diagnostic; benchctl's health gates remain authoritative.
"""
import argparse
import json
import os
import pathlib
import shutil
import subprocess
import time
import urllib.parse
import urllib.request


def command(*args, errors=None):
    try:
        result = subprocess.run(args, text=True, capture_output=True, timeout=2)
        if result.returncode:
            if errors is not None:
                errors.append({'command': list(args), 'returncode': result.returncode, 'stderr': result.stderr[-500:]})
            return None
        return result.stdout.strip()
    except (OSError, subprocess.TimeoutExpired) as exc:
        if errors is not None:
            errors.append({'command': list(args), 'error': type(exc).__name__})
        return None


def prom_query(base, expression):
    if not base:
        return None
    url = base.rstrip('/') + '/api/v1/query?' + urllib.parse.urlencode({'query': expression})
    try:
        with urllib.request.urlopen(url, timeout=1) as response:
            return json.load(response)
    except Exception as exc:
        return {'error': type(exc).__name__}


def require_vector(base, expression, predicate=lambda row: True):
    response = prom_query(base, expression)
    if not isinstance(response, dict) or response.get('status') != 'success':
        raise RuntimeError(f'Prometheus query failed: {expression}: {response}')
    rows = response.get('data', {}).get('result', [])
    if not any(predicate(row) for row in rows):
        raise RuntimeError(f'Prometheus query returned no required matching series: {expression}')
    return response


def empirical_clock_steps(clock, count=10000):
    values = [clock() for _ in range(count)]
    positive = [b-a for a, b in zip(values, values[1:]) if b > a]
    return {'samples': count, 'unique_values': len(set(values)),
            'observed_min_positive_step_ns': min(positive) if positive else None,
            'observed_max_step_ns': max(positive) if positive else None}


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('run_id')
    p.add_argument('output', type=pathlib.Path)
    p.add_argument('--prometheus', default='http://127.0.0.1:9090')
    p.add_argument('--interval', type=float, default=1.0)
    p.add_argument('--wait-seconds', type=int, default=600)
    p.add_argument('--tail-seconds', type=int, default=930,
                   help='continue clock/Prometheus sampling after k6 exits, through receipt drain and late observation')
    p.add_argument('--ready-file', type=pathlib.Path,
                   help='created only after collector and Kafka/Prometheus preflight succeeds')
    a = p.parse_args()
    preflight_errors = []
    if os.geteuid() != 0:
        preflight_errors.append('not-root: cgroup and per-process socket observations unavailable')
    missing = [tool for tool in ('docker', 'nsenter', 'ss') if shutil.which(tool) is None]
    if missing:
        preflight_errors.append('missing-tools: ' + ', '.join(missing))
    if not command('docker', 'info'):
        preflight_errors.append('docker-daemon-unavailable')
    empirical_resolution = {'realtime': empirical_clock_steps(time.time_ns),
                            'monotonic': empirical_clock_steps(time.monotonic_ns)}
    try:
        require_vector(a.prometheus, 'up{job="kafka",platform="thingsboard"}',
                       lambda row: row.get('value', [None, None])[1] == '1')
    except Exception as exc:
        preflight_errors.append('kafka-target-unavailable: ' + str(exc))
    topics = 'tb_transport[.]api[.]requests|tb_core[.].*|tb_rule_engine[.]main[.].*'
    for metric in ('kafka_consumergroup_lag', 'kafka_consumergroup_current_offset', 'kafka_topic_partition_current_offset'):
        try:
            require_vector(a.prometheus, f'{metric}{{platform="thingsboard",topic=~"{topics}"}}')
        except Exception as exc:
            preflight_errors.append(f'{metric}-unavailable: {exc}')
    if a.ready_file:
        a.ready_file.parent.mkdir(parents=True, exist_ok=True)
        with a.ready_file.open('x') as ready:
            ready.write(json.dumps({'ready': True, 'host': command('hostname') or 'unknown',
                                    'realtime_resolution_ns': int(time.clock_getres(time.CLOCK_REALTIME)*1e9),
                                    'monotonic_resolution_ns': int(time.clock_getres(time.CLOCK_MONOTONIC)*1e9),
                                    'empirical_clock_steps': empirical_resolution,
                                    'optional_metric_preflight_errors': preflight_errors})+'\n')
    name = 'k6-' + a.run_id
    deadline = time.monotonic() + a.wait_seconds
    info_text = None
    while time.monotonic() < deadline and not info_text:
        info_text = command('docker', 'inspect', name)
        time.sleep(.2)
    if not info_text:
        raise SystemExit(f'k6 container {name} did not appear within {a.wait_seconds}s')
    a.output.parent.mkdir(parents=True, exist_ok=True)
    realtime_res_ns = int(time.clock_getres(time.CLOCK_REALTIME) * 1e9)
    monotonic_res_ns = int(time.clock_getres(time.CLOCK_MONOTONIC) * 1e9)
    prior_offset_interval = None
    container_seen = False
    container_exit_mono = None
    samples = 0
    collection_errors = []
    container_samples = 0
    step_candidates = 0
    sampling_overruns = 0
    collection_error_count = 0
    prometheus_errors = 0
    kafka_missing_samples = 0
    max_sample_gap_ns = 0
    prior_sample_mono = None
    k6_series_seen = set()
    k6_metric_sample_counts = {'k6_vus': 0, 'k6_http_req_connecting_p99': 0, 'k6_http_req_blocked_p99': 0}
    capture_start_wall = time.time_ns()
    capture_start_mono = time.monotonic_ns()
    with a.output.open('x') as out:
        while True:
            mono_before = time.monotonic_ns()
            wall = time.time_ns()
            mono_after = time.monotonic_ns()
            sample_mono = mono_before
            if prior_sample_mono is not None:
                max_sample_gap_ns = max(max_sample_gap_ns, sample_mono-prior_sample_mono)
            prior_sample_mono = sample_mono
            offset_interval = [wall-mono_after, wall-mono_before]
            step_discontinuity = (prior_offset_interval is not None and
                                  (offset_interval[0] > prior_offset_interval[1] or
                                   prior_offset_interval[0] > offset_interval[1]))
            prior_offset_interval = offset_interval
            # A missing container after it has run is the expected end-of-workload signal.
            info_text = command('docker', 'inspect', name)
            info = json.loads(info_text)[0] if info_text else None
            if info:
                container_seen = True
                container_samples += 1
            elif container_seen and container_exit_mono is None:
                container_exit_mono = time.monotonic()
            elif not container_seen:
                raise SystemExit(f'k6 container {name} disappeared before first sample')
            if not info and container_exit_mono is not None and time.monotonic() - container_exit_mono >= a.tail_seconds:
                break
            info = info or {}
            pid = int(info.get('State', {}).get('Pid') or 0)
            cgroup = None
            schedstat = None
            sockets = {}
            if pid:
                try:
                    cgroup_path = next(line.split('::', 1)[1] for line in pathlib.Path(f'/proc/{pid}/cgroup').read_text().splitlines() if '::' in line)
                    cgroup = pathlib.Path('/sys/fs/cgroup' + cgroup_path)
                    cgdir = cgroup
                    cpu_stat = {line.split()[0]: int(line.split()[1]) for line in (cgdir/'cpu.stat').read_text().splitlines()}
                    quota = (cgdir/'cpu.max').read_text().split()
                    cpuset = (cgdir/'cpuset.cpus.effective').read_text().strip()
                    ancestors = []
                    parent = cgdir.parent
                    while parent == pathlib.Path('/sys/fs/cgroup') or pathlib.Path('/sys/fs/cgroup') in parent.parents:
                        row = {'path': str(parent)}
                        for filename, key in (('cpu.max', 'cpu_max'), ('cpuset.cpus.effective', 'cpuset_cpus_effective')):
                            try:
                                row[key] = (parent/filename).read_text().strip().split()
                            except OSError:
                                row[key] = None
                        try:
                            row['cpu_stat'] = {line.split()[0]: int(line.split()[1])
                                               for line in (parent/'cpu.stat').read_text().splitlines()}
                        except OSError:
                            row['cpu_stat'] = None
                        ancestors.append(row)
                        if parent == pathlib.Path('/sys/fs/cgroup'):
                            break
                        parent = parent.parent
                    cgroup = {'path': cgroup_path, 'cpu_stat': cpu_stat,
                               'cpu_max': quota, 'cpuset_cpus_effective': cpuset,
                               'ancestor_constraints': ancestors}
                    # Prospective generator-integrity evidence. Restrict this to
                    # cgroup counters and the non-secret calibrated Go setting.
                    for filename, key in (('memory.current', 'memory_current_bytes'),
                                          ('memory.peak', 'memory_peak_bytes'),
                                          ('memory.swap.current', 'memory_swap_current_bytes')):
                        try:
                            cgroup[key] = int((cgdir/filename).read_text().strip())
                        except (OSError, ValueError):
                            cgroup[key] = None
                    try:
                        cgroup['memory_events'] = {line.split()[0]: int(line.split()[1])
                                                   for line in (cgdir/'memory.events').read_text().splitlines()}
                    except (OSError, ValueError):
                        cgroup['memory_events'] = None
                    thread_stats = []
                    for task in pathlib.Path(f'/proc/{pid}/task').iterdir():
                        try:
                            fields = (task/'schedstat').read_text().split()
                            thread_stats.append({'tid': int(task.name), 'runtime_ns': int(fields[0]),
                                                 'runqueue_wait_ns': int(fields[1]), 'timeslices': int(fields[2])})
                        except (OSError, ValueError, IndexError):
                            continue
                    schedstat = {'threads': thread_stats}
                    socket_lines = command('nsenter', '-t', str(pid), '-n', 'ss', '-Htanp', errors=collection_errors)
                    if socket_lines is not None:
                        states = [line.split()[0] for line in socket_lines.splitlines() if line.split()]
                        sockets = {'scope': 'load-host network namespace (k6 uses host networking)',
                                   'state_counts': {state: states.count(state) for state in sorted(set(states))},
                                   'k6_process_socket_lines': [line for line in socket_lines.splitlines()
                                                               if f'pid={pid},' in line]}
                except (OSError, ValueError, StopIteration, IndexError):
                    collection_errors.append({'area': 'cgroup-or-schedstat-or-sockets', 'error': 'read-or-parse-failed'})
            errors_this_sample, collection_errors = collection_errors, []
            record = {
                'load_realtime_ns': str(wall), 'load_monotonic_before_ns': str(mono_before),
                'load_monotonic_after_ns': str(mono_after),
                'clock_getres_ns': {'realtime': realtime_res_ns, 'monotonic': monotonic_res_ns},
                'realtime_minus_monotonic_interval_ns': [str(x) for x in offset_interval],
                'realtime_step_discontinuity_candidate': step_discontinuity,
                 'container_present': bool(info), 'container_id': info.get('Id'), 'container_pid': pid,
                 'container_running': info.get('State', {}).get('Running'),
                'container_cpuset_requested': info.get('HostConfig', {}).get('CpusetCpus'),
                'container_oom_killed': info.get('State', {}).get('OOMKilled'),
                'container_restarts': info.get('RestartCount'),
                'generator_gogc': next((entry.split('=', 1)[1] for entry in info.get('Config', {}).get('Env', [])
                                        if entry.startswith('GOGC=')), None),
                'cpu_stat': cgroup, 'thread_schedstat': schedstat, 'socket_states': sockets,
                'cpu_pressure': pathlib.Path('/proc/pressure/cpu').read_text().strip(),
                'k6_metrics': prom_query(a.prometheus,
                    '{__name__=~"k6_vus|k6_vus_max|k6_http_req_connecting_p50|k6_http_req_connecting_p99|k6_http_req_connecting_max|k6_http_req_blocked_p50|k6_http_req_blocked_p99|k6_http_req_blocked_max",run_id="' + a.run_id + '",name!="setup"}'),
                'kafka_offsets_and_lag': prom_query(a.prometheus,
                    '{__name__=~"kafka_consumergroup_lag|kafka_consumergroup_current_offset|kafka_topic_partition_current_offset",platform="thingsboard",topic=~"tb_transport[.]api[.]requests|tb_core[.].*|tb_rule_engine[.]main[.].*"}'),
                'kafka_target_up': prom_query(a.prometheus, 'up{job="kafka"}'),
                'collection_errors': errors_this_sample,
            }
            sample_done = time.monotonic_ns()
            record['sampler_elapsed_ns'] = str(sample_done-mono_before)
            record['sampling_interval_target_ns'] = int(a.interval*1e9)
            elapsed_ns = sample_done-mono_before
            record['sampling_overrun'] = elapsed_ns > int(a.interval*1e9)
            sampling_overruns += int(record['sampling_overrun'])
            step_candidates += int(step_discontinuity)
            collection_error_count += len(errors_this_sample)
            prometheus_errors += int(isinstance(record['k6_metrics'], dict) and 'error' in record['k6_metrics'])
            prometheus_errors += int(isinstance(record['kafka_offsets_and_lag'], dict) and 'error' in record['kafka_offsets_and_lag'])
            prometheus_errors += int(isinstance(record['kafka_target_up'], dict) and 'error' in record['kafka_target_up'])
            kafka_result = ((record['kafka_offsets_and_lag'] or {}).get('data', {}).get('result', [])
                            if isinstance(record['kafka_offsets_and_lag'], dict) else [])
            kafka_up = ((record['kafka_target_up'] or {}).get('data', {}).get('result', [])
                        if isinstance(record['kafka_target_up'], dict) else [])
            kafka_names = {row.get('metric', {}).get('__name__') for row in kafka_result}
            if not {'kafka_consumergroup_lag', 'kafka_consumergroup_current_offset',
                    'kafka_topic_partition_current_offset'}.issubset(kafka_names) or not any(
                    row.get('value', [None, None])[1] == '1' for row in kafka_up):
                kafka_missing_samples += 1
            k6_result = ((record['k6_metrics'] or {}).get('data', {}).get('result', [])
                         if isinstance(record['k6_metrics'], dict) else [])
            for row in k6_result:
                metric = row.get('metric', {}).get('__name__')
                k6_series_seen.add(metric)
                if metric in k6_metric_sample_counts:
                    k6_metric_sample_counts[metric] += 1
            out.write(json.dumps(record, separators=(',', ':')) + '\n')
            out.flush()
            samples += 1
            time.sleep(max(0, a.interval-elapsed_ns/1e9))
    capture_end_wall = time.time_ns()
    capture_end_mono = time.monotonic_ns()
    tail_elapsed = ((capture_end_mono-container_exit_mono*1e9) / 1e9
                    if container_exit_mono is not None else 0)
    status = {
        'schema_version': 1, 'run_id': a.run_id, 'host': command('hostname') or 'unknown',
        'output': str(a.output), 'samples': samples, 'container_samples': container_samples,
        'step_discontinuity_candidates': step_candidates, 'sampling_overruns': sampling_overruns,
        'collection_error_count': collection_error_count, 'prometheus_query_errors': prometheus_errors,
        'kafka_missing_samples': kafka_missing_samples, 'max_sample_gap_ns': max_sample_gap_ns,
        'k6_metric_sample_counts': k6_metric_sample_counts, 'k6_metric_names_seen': sorted(x for x in k6_series_seen if x),
        'capture_start_realtime_ns': str(capture_start_wall), 'capture_end_realtime_ns': str(capture_end_wall),
        'capture_elapsed_monotonic_ns': str(capture_end_mono-capture_start_mono),
        'post_container_tail_elapsed_seconds': tail_elapsed,
        'configured_post_container_tail_seconds': a.tail_seconds,
        'clock_getres_ns': {'realtime': realtime_res_ns, 'monotonic': monotonic_res_ns},
        'empirical_clock_steps': empirical_resolution,
        'sampling_complete': samples > 0 and container_samples > 0 and tail_elapsed >= a.tail_seconds,
        'optional_metrics_available': {
            'scheduler_cgroup_socket': os.geteuid() == 0 and not any(x.startswith('missing-tools:') or x == 'docker-daemon-unavailable' for x in preflight_errors),
            'k6_vu_connection': all(v > 0 for v in k6_metric_sample_counts.values()),
            'kafka_lag_offsets': kafka_missing_samples == 0 and prometheus_errors == 0 and not any('kafka' in x or 'kafka_' in x for x in preflight_errors),
        },
        'optional_metric_preflight_errors': preflight_errors,
        'optional_metric_collection_errors': collection_error_count,
        'optional_metric_query_errors': prometheus_errors,
        'optional_metric_sampling_overruns': sampling_overruns,
        'optional_metric_kafka_missing_samples': kafka_missing_samples,
        'complete': samples > 0 and container_samples > 0 and tail_elapsed >= a.tail_seconds,
    }
    status_path = a.output.with_suffix(a.output.suffix + '.status.json')
    with status_path.open('x') as status_file:
        status_file.write(json.dumps(status, indent=2)+'\n')
    if not status['complete']:
        raise SystemExit(f'incomplete generator diagnostic sampling; see {status_path}')


if __name__ == '__main__':
    main()
