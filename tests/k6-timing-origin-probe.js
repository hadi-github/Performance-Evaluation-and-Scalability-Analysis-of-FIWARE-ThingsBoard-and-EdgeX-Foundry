import exec from 'k6/execution';
import { sleep } from 'k6';

export const options = {
  scenarios: { probe: { executor: 'constant-arrival-rate', rate: 2, timeUnit: '1s', duration: '1s', startTime: '1s', preAllocatedVUs: 1, maxVUs: 1 } },
};

export function setup() {
  sleep(0.25); // Deliberately nontrivial setup; verifies timing marks remain useful afterward.
  return { setupElapsedMs: exec.instance.currentTestRunDuration };
}

export default function (state) {
  const entry = exec.instance.currentTestRunDuration;
  sleep(0.02);
  const ready = exec.instance.currentTestRunDuration;
  sleep(0.02);
  const preHttp = exec.instance.currentTestRunDuration;
  if (!(state.setupElapsedMs > 0 && entry > state.setupElapsedMs && ready > entry && preHttp > ready)) {
    throw new Error('elapsed timing intervals did not advance monotonically after setup');
  }
  console.log(JSON.stringify({ probe: true, setupElapsedMs: state.setupElapsedMs, entryElapsedMs: entry, readyElapsedMs: ready, preHttpElapsedMs: preHttp }));
}
