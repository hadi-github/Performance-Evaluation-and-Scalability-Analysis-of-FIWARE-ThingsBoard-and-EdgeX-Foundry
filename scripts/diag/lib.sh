# Shared helpers for the diagnostics sessions (sourced). Run from the workstation.
KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519_vps}"
J=(-o BatchMode=yes -o ConnectTimeout=60 -o ConnectionAttempts=3 -o ServerAliveInterval=15 -o IdentitiesOnly=yes -i "$KEY" -J root@ramonavm2)
v1() { ssh "${J[@]}" root@87.107.191.32 "$@"; }
# ConnectionAttempts retries only the TCP/SSH connection, before any remote command runs: never runs a step twice
v2() { ssh -o BatchMode=yes -o ConnectTimeout=60 -o ConnectionAttempts=3 -o ServerAliveInterval=15 root@ramonavm2 "$@"; }
R=/root/iot-bench-deploy
log() { printf '[%s] %s\n' "$(date -u +%T)" "$*"; }

# collect_run <platform> <plan> <run_id> [mongo_profile_rate]: collectors + run + SUT data next to the run on ramonavm1
collect_run() {
  local platform=$1 plan=$2 id=$3 prate=${4:-0}
  log "$id: collectors"
  v2 "cd $R && scripts/diag/collect.sh start $platform $id $prate"
  log "$id: run ($plan)"
  v1 "cd $R && scripts/run-with-sink.sh workloads/plans/$plan.json $id results/diag; tail -3 results/diag/$id.run.txt"
  v2 "cd $R && scripts/diag/collect.sh stop $id >/dev/null"
  v2 "cd /root/diag && tar cf - $id" | v1 "mkdir -p $R/results/diag/sut && tar xf - -C $R/results/diag/sut"
}

# explore_run <run_id> [extra benchctl explore flags]
explore_run() {
  local id=$1; shift
  v1 "cd $R/results/diag && ../../bin/benchctl explore -run $id -sink $id.sink.ndjson -cpu sut/$id/cpu.tsv $* | head -14" || true
}

# copy a SUT file into results/diag/sut/<run_id>/ on ramonavm1
to_v1() { local src=$1 id=$2; v2 "cat $src" | v1 "mkdir -p $R/results/diag/sut/$id && cat > $R/results/diag/sut/$id/$(basename "$src")"; }
