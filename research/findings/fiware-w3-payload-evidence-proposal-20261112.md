# Proposal (NOT APPLIED): a payload-preserving FIWARE evidence path for W3

**Date:** 2026-11-12
**Status:** **PROPOSAL ONLY — nothing in this document has been applied.**
**Owner instruction this responds to:** *"you can also continue with fiware and leave the
unsupported metrics as unavailable and try to measure the most important parts as you can"*
plus the earlier answer *"Yes, prepare it as a proposal only"*.
**Blocks:** W3 gate 2 only. W3 gate 1's stored-payload half is unavailable on FIWARE today;
this proposal is how it could become available. The W3 FIWARE *measurement* runs regardless,
with the unavailable metrics recorded as unavailable (see §6).

---

## 1. The blocker, precisely

FIWARE's history tier is Orion → QuantumLeap → TimescaleDB. Both Orion subscriptions select
an explicit attribute list, and **neither selects `padding`**:

`results/feasible-campaign-20261106/run-feasible-campaign.py` (QuantumLeap / history route):

```python
body={'description':'bench quantumleap',
      'subject':{'entities':[{'idPattern':'.*','type':'Sensor'}],'condition':{'attrs':['seq']}},
      'notification':{'http':{'url':'http://87.107.191.107:8668/v2/notify'},
                      'attrs':['seq','run_id','scheduled_send_ms'],'attrsFormat':'normalized'}}
```

`scripts/outbound.sh` (sink route):

```bash
body=$(jq -nc --arg url "$SINK/fiware" \
  '{notification:{http:{url:$url},attrs:["seq","run_id","scheduled_send_ms"],attrsFormat:"keyValues"}}')
```

Consequence: QuantumLeap creates TimescaleDB columns only for the attributes it is told to
forward. `scripts/history-payload.sh` selects `t."padding"` for FIWARE, so with the current
subscription that column does not exist and the export fails rather than inventing a value —
which is the correct fail-closed behaviour, and is exactly what W3 gate 2 describes.

Note the asymmetry that makes this look like a possible oversight rather than a design
choice: **the generator already provisions and sends `padding` on FIWARE.** In
`workloads/k6/lib/adapters.js`, the UltraLight attribute `d` is mapped to a `padding` Text
attribute **only when an explicit payload plan is selected**:

```js
if (payloadSelected(cfg)) attrs.push({ object_id: 'd', name: 'padding', type: 'Text' });
```

So Orion receives `padding` and stores it as an entity attribute. It is only *history* and
*sink* that drop it, because their subscriptions name three attributes explicitly.

**Confirmed live on the deployed FIWARE stack (2026-11-12), not inferred from source:**

```
bench QL         -> ['seq','run_id','scheduled_send_ms','temperature']  normalized  inactive
bench sink       -> ['seq','run_id','scheduled_send_ms']                keyValues   active
bench quantumleap-> ['seq','run_id','scheduled_send_ms']                normalized  active
```

Both **active** routes omit `padding`. This is the exact defect W3 gate 2 names.

---

## 2. What a change would have to touch

| Artefact | Current | Would become |
|---|---|---|
| `run-feasible-campaign.py` QuantumLeap subscription `attrs` | `["seq","run_id","scheduled_send_ms"]` | `+ "padding"` |
| `scripts/outbound.sh` sink subscription `attrs` | `["seq","run_id","scheduled_send_ms"]` | `+ "padding"` |
| QuantumLeap schema | 3 typed columns | a `padding` `text` column appears on the next entity write |
| `scripts/history-payload.sh` fiware branch | already selects `t."padding"` | unchanged (starts working) |

Orion `attrsFormat` matters here and is already correct: `normalized` for QuantumLeap (which is
what makes QL materialise typed columns) and `keyValues` for the sink.

---

## 3. Why this is not a local W3-only edit

The subscriptions are **shared batch configuration**, not a W3 setting. They are created and
reconciled by the B0 launcher's FIWARE activation path, which also asserts the history route is
healthy. Two consequences:

1. **Every later FIWARE cell would run under the new subscription.** That is not automatically
   bad — it is strictly more evidence — but it means W3-FIWARE would no longer be comparable to
   the W1/W2 FIWARE cells that ran with three attributes. The frozen W3 gate says exactly this:
   *"Review and qualify a payload-preserving evidence path with common-batch/comparability
   treatment; never silently change only W3 or active B0."*
2. **B0 is active evidence.** Changing the subscription that produced B0's FIWARE history rows
   would make the current B0 FIWARE captures non-reproducible from the shipped configuration.

So the change must be applied **to the common configuration**, disclosed as a delta against the
B0-era subscription, and **not** stealth-edited under W3 alone.

---

## 4. Three candidate options

### Option A — add `padding` to both subscriptions (recommended if it is ever done)

```diff
- attrs=["seq","run_id","scheduled_send_ms"]
+ attrs=["seq","run_id","scheduled_send_ms","padding"]
```

- **Pro:** the only option that makes gate 1's *history* half true on FIWARE, per message, using
  the same `ValidateStored` check TB and EdgeX use.
- **Pro:** QuantumLeap creates the `text` column automatically on the next write; no schema
  migration is authored by hand.
- **Con:** it is a common-batch change. It needs a comparability note for W1/W2 FIWARE and a
  statement of what B0-FIWARE captures were taken under.
- **Con:** 1,024 extra chars per row in TimescaleDB. At W3's 22,500 messages that is ~23 MB of
  padding for the run — not a capacity risk, but it should be stated, not assumed.

### Option B — keep history at three attributes, add padding to the sink only

- **Pro:** narrower blast radius; the sink is the transient delivery surface, not B0's history.
- **Con:** the sink's `Record` **does not retain padding** (`internal/sink/sink.go` stores
  `run_id`, `seq`, `device_id`, `scheduled_send_ms`, receipt timing). Forwarding more attributes
  to the sink would prove *transport of* padding, not *storage of* padding, unless the sink schema
  is also extended — which changes a second frozen artefact and would perturb sink throughput
  measurements.
- **Verdict:** does not satisfy gate 1. It is at best a cheaper partial signal.

### Option C — no subscription change; read Orion latest-state

- **Pro:** zero configuration change; uses the platform's own store.
- **Con:** Orion latest-state holds only the **last** value per entity. With 1,000 devices and
  22,500 messages, that is ~22 messages per device, of which only 1 survives. It cannot prove
  per-message preservation.
- **Verdict:** a real but weak signal. Usable now, honestly labelled (§6), not a substitute for
  Option A.

---

## 5. If Option A is approved, the shape of the change

Not written here, because it is not authorized. The minimum it would need:

1. Edit both `attrs` lists in the launcher **and** `scripts/outbound.sh` in one change, so the
   two routes never disagree about which fields travel.
2. Record a new owner receipt binding the changed launcher (`b0_launcher` is a bound artefact),
   and delete the superseded receipt for that scope — receipts are hash-bound, so an edited
   launcher invalidates the old one by design.
3. Add a `comparability` note to the W3 FIWARE result naming the subscription delta, and to the
   W1/W2 FIWARE results if they are ever re-read as comparable.
4. Verify the QuantumLeap column exists **after** a run
   (`information_schema.columns ... column_name='padding'`), then run
   `scripts/history-payload.sh fiware <run>` and feed it to
   `benchctl payload-validate`, exactly as TB and EdgeX.
5. Confirm the 65 s Orion replica subscription-cache refresh is still honoured after the PATCH
   (the launcher already waits for it on the history route).

Risk if done carelessly: a PATCH to a live subscription with the wrong `attrsFormat` silently
drops typed columns, and QuantumLeap would then record *nothing* for that entity rather than
erroring. The existing route-health assertion checks the URL and status, **not** the attribute
list, so it would not catch that. Any approved change should add an assertion on `attrs`.

---

## 6. What FIWARE W3 measures **now**, with the rest marked unavailable

This is what the owner authorized for this session: run FIWARE, measure what is real, and label
the rest unavailable rather than passing it by omission.

| W3 gate | FIWARE today | Basis |
|---|---|---|
| 1a. Exactly 1,024 extra **logical** ASCII padding bytes, every message | **MEASURABLE** | generator `sample_request` accounting + `fixed-point-padding.test.mjs` |
| 1b. Stored-payload preservation, validated from the platform's own store | **UNAVAILABLE** | `padding` is not in either subscription; no typed column to read |
| 2. Payload-preserving evidence path reviewed/qualified | **UNAVAILABLE (this is the blocker)** | see §1; Option A is the fix |
| 3a. Logical bytes vs serialized adapter body bytes, reported separately | **MEASURABLE** | `k6-summary.json` → `sample_request` |
| 3b. Scoped **measured transport** bytes | **UNAVAILABLE** | no on-wire capture; HTTP-header arithmetic is an estimate and will be labelled as such |
| 4. Zero-padding native delta 1,022 vs logical 1,024 | **MEASURABLE** | proven offline in `fixed-point-padding.test.mjs` |
| Throughput / latency / ack / history identity completeness at 25/s | **MEASURABLE** | normal dispatch analysis |

The honest verdict for a clean FIWARE W3 is therefore **not** plain `VALID` and **not**
`INVALID_PLATFORM_FAILURE_OR_BACKLOG`. It should carry the measured outcome for the axes that
passed and an explicit `unavailable` block naming each unmet gate and why. The result record will
do that; no gate is silently upgraded.

One further signal is available at no configuration cost, and will be recorded as **weak
evidence, clearly labelled**: Orion latest-state (`/v2/entities?attrs=padding`) can be queried
after the run to show how many of the 1,000 entities hold a `padding` attribute and at what
length. That proves the platform *ingested and stored* the padded attribute; it cannot prove
per-message preservation, because only the last value per entity survives. Expect ~1,000/1,000
entities with a 1,024-char value if the path works end to end.

---

## 7. Recommendation

1. **Run FIWARE W3 now** with §6's labelling. The throughput, latency and delivery evidence is
   worth having and is independently valid.
2. **Do not** apply Option A in this session. It is a common-batch change that alters B0-era
   reproducibility and needs its own receipt and comparability note.
3. **Prepare Option A** as the reviewed change for a later decision, with the `attrs` assertion
   from §5.4 added to the route-health check so a wrong attribute list cannot pass silently.

## 8. Related

- `research/findings/w1-fiware-fails-at-50s-20261111.md` — FIWARE's rate-bound finding (unchanged)
- `research/findings/w2-2000-devices-measured-20261112.md` — FIWARE passes at 25/s with 2,000 devices
- `workloads/campaigns/fixed-points-20261108/qualification-gates.json` — the frozen W3 gates quoted in §1
- `scripts/history-payload.sh` — the FIWARE export that fails closed until a typed column exists
- `cmd/benchctl/payload.go` — the independent stored-payload check that would consume it
