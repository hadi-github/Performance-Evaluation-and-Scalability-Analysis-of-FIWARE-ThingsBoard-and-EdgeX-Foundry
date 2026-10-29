#!/usr/bin/env bash
# Re-run every k6 feasibility case of docs/methodology/k6-feasibility.md and print the analyses (about 5 minutes).
set -euo pipefail
cd "$(dirname "$0")/../.."
A=workloads/k6/feasibility/car-probe.js
B=workloads/k6/feasibility/slot-probe.js
R=tests/feasibility/run-case.sh
an() { go run ./cmd/feasibility-analyze "$@"; echo; }

$R A0-baseline        $A RATE=200 DURATION=10s PRE_VUS=20  MAX_VUS=20             && an -case results/feasibility/A0-baseline -rate 200
$R A1-vu-exhaustion   $A RATE=100 DURATION=10s PRE_VUS=5   MAX_VUS=5 DELAY_MS=200  && an -case results/feasibility/A1-vu-exhaustion -rate 100
$R A2-generator-delay $A RATE=500 DURATION=10s PRE_VUS=100 MAX_VUS=100 K6_CPUS=0.1 && an -case results/feasibility/A2-generator-delay -rate 500
$R A3-target-delay    $A RATE=100 DURATION=10s PRE_VUS=100 MAX_VUS=100 DELAY_MS=500 && an -case results/feasibility/A3-target-delay -rate 100

$R B0-baseline        $B RATE=200 DURATION=10s VUS=20                && an -probe slot -planned 2000 -case results/feasibility/B0-baseline -rate 200
$R B1-vu-exhaustion   $B RATE=100 DURATION=10s VUS=5 DELAY_MS=200    && an -probe slot -planned 1000 -case results/feasibility/B1-vu-exhaustion -rate 100
$R B2-generator-delay $B RATE=500 DURATION=10s VUS=100 K6_CPUS=0.1   && an -probe slot -planned 5000 -case results/feasibility/B2-generator-delay -rate 500
$R B3-target-delay    $B RATE=100 DURATION=10s VUS=100 DELAY_MS=500  && an -probe slot -planned 1000 -case results/feasibility/B3-target-delay -rate 100
