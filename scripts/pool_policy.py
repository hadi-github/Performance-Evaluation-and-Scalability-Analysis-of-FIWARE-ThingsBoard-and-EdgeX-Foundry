#!/usr/bin/env python3
"""Canonical generator VU-pool sizing. Single source of truth for the batch.

Why this module exists
---------------------
The generator is an open-loop constant-arrival executor. At rate ``r`` with a
client timeout ``t`` it must be able to hold ``r * t`` requests *in flight* at
once, or it starves itself: k6 reports those as ``dropped_iterations``, which is
indistinguishable in the ledger from loss that the platform caused. The pool is
therefore a statement about **the harness**, not about the SUT.

The original policy froze this at 150 VUs for every rate. That is correct at
25/s (25 * 5 s = 125 < 150) and structurally too small at 50/s (250 > 150), so
W1 could never be qualified no matter how healthy the platform was. The measured
proof is in ``research/findings/w1-vu-pool-binds-both-platforms.md``: ThingsBoard's
own qualification recorded ``implied_in_flight_at_p99: 243`` against a 150 pool,
and its measurement phase -- run at the same rate, once warm -- needed only ~11
VUs and dropped nothing.

Owner decision 2026-11-10: raise the pool to a rate-derived value rather than
accept a negative that the harness, not the platform, produced.

Derivation
----------
    bound  = rate_per_s * request_timeout_ms / 1000
    pool   = ceil(bound * HEADROOM / QUANTUM) * QUANTUM

``HEADROOM`` is 1.10 because B0's own 25/s cells peaked at 126 VUs against a
theoretical 125 -- a pool of exactly ``rate * timeout`` sits on the boundary and
would flip to INVALID on ordinary jitter. ``QUANTUM`` keeps the numbers round.

This yields 150 at 25/s, which is what B0 already used, so **no B0 cell changes
and no historical verdict is reinterpreted**. Only the 50/s workload (W1) moves.
"""

import math

#: Measurement-only jitter allowance. See module docstring for the evidence.
HEADROOM = 1.10
#: Round the pool up to a multiple of this so plans stay readable.
QUANTUM = 50
#: Frozen client timeout for the whole batch, in ms.
REQUEST_TIMEOUT_MS = 5000

#: Baseline rate of the B0 batch; the pool frozen against it defines v1.
B0_RATE_PER_S = 25

#: The rates the batch is allowed to run at. Anything else fails closed.
QUALIFIED_RATES = (25, 50, 100)

#: Deliberately explicit rather than computed at import time, so a change to the
#: formula cannot silently move a frozen number without touching this table too.
#: ``tests/pool_policy_test.py`` asserts the formula reproduces it exactly.
FROZEN_POOLS = {25: 150, 50: 300, 100: 550}

POLICY_V1 = "dispatch-integrity-v1"
POLICY_V2 = "dispatch-integrity-v2"
#: Current policy for W attempts. v1 remains valid for the historical B0 batch.
DEFAULT_POLICY = POLICY_V2


def bound_in_flight(rate_per_s, timeout_ms=REQUEST_TIMEOUT_MS):
    """Worst-case concurrent requests the generator must hold at this rate."""
    return float(rate_per_s) * float(timeout_ms) / 1000.0


def required_pool(rate_per_s, timeout_ms=REQUEST_TIMEOUT_MS):
    """Minimum pool that keeps ``rate * timeout`` from binding.

    Computed, not looked up, so an unqualified rate is still sized honestly
    rather than silently defaulting to a frozen number meant for another rate.
    """
    return int(math.ceil(bound_in_flight(rate_per_s, timeout_ms) * HEADROOM / QUANTUM) * QUANTUM)


def pool_for_rate(rate_per_s, policy=POLICY_V2, timeout_ms=REQUEST_TIMEOUT_MS):
    """Frozen pool for a rate under a policy, failing closed on unknown rates.

    ``dispatch-integrity-v1`` is retained only so the historical B0 batch can be
    re-evaluated byte-identically; it always answered 150. New work uses v2.
    """
    rate = int(rate_per_s)
    if policy == POLICY_V1:
        return FROZEN_POOLS[B0_RATE_PER_S]
    if policy not in (POLICY_V2,):
        raise ValueError(f'unknown pool policy: {policy!r}')
    if rate not in QUALIFIED_RATES:
        raise ValueError(f'no frozen pool for rate {rate}/s; qualified rates are {QUALIFIED_RATES}')
    return FROZEN_POOLS[rate]


def policy_for_rate(rate_per_s):
    """The policy label a plan at this rate must declare in its own plan file.

    v1's 150 is still the correct pool at 25/s, so a 25/s plan may keep the v1
    contract. A rate whose pool v1 would starve must declare v2.
    """
    rate = int(rate_per_s)
    if pool_for_rate(rate, POLICY_V2) == FROZEN_POOLS[B0_RATE_PER_S]:
        return POLICY_V1
    return POLICY_V2
