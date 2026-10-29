# Interpretation correction (auto-20261029, owner instruction)

This correction applies to the auto-20261028 close-out report and any derived records. The original evidence
files are unchanged; only the interpretation is corrected.

1. **50 r/s is a valid tested point** and stays valid (thingsboard-n1-r50-20261027T222308Z: stored==sent
   4,502, 0 drops, 0 errors, SUT oom=0).
2. The **original 100 r/s result is INVALID due to VU starvation** (2,041 dropped iterations with default
   VUs; achieved 87.75/100) — it is **not** a platform FAIL and must not be read as one.
3. The **increased-VU result is recorded separately** (thingsboard-n1-r100-20261027T221939Z): zero loss
   (stored==sent 8,903) but 98 dropped iterations (1.1%) — still INVALID under the adopted dropped=0 gate.
   Its SLO evidence: platform latency tails (p99 ≈ 2 s observed at this rate) exceed the VU pool's coverage;
   the loss disappeared once VUs covered the tails, which locates the bottleneck in scheduling headroom, not
   in message durability.
4. These results are **not a platform-capacity bracket**. They are a generator-configuration finding at one
   platform configuration (k=2, 1024 MiB, ~2,400-device registry). No platform-capacity bracket exists yet.
5. **1280 MiB is a promising memory configuration**, not proof that the container limit was the sole
   bottleneck; the qualification under real load (this session, task B) is what tests it.
6. **Cross-host clock accuracy remains unverified** until the corrected live measurements succeed; the
   geometry-bias diagnosis explains the old numbers but does not measure the true offset.
