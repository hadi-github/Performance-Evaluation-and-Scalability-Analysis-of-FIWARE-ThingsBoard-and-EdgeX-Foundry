# Renaming the campaign directory: what drifted and why it is acceptable

Status: **recorded consequence of a deliberate repository change, 2026-11-13.**
This note exists so a reviewer reading an old receipt understands why its
recorded hashes no longer match the tree, instead of concluding the evidence was
tampered with.

## What changed

`workloads/campaigns/fixed-points-20261108/` was renamed to
`workloads/campaigns/fixed-points-w1-w4/`.

The date in the old name recorded when the directory was created. It said
nothing about what the campaign *is*, and it leaked our iteration history into a
path a reviewer has to type. The date survives where it is meaningful: `batch_id`
(`fixed-points-w1-w4-20261108`), every run ID (`w1-qual-ex-20261110e`) and every
plan filename (`w1dp-ex-r01-20261108.json`). Those are immutable attempt
identity, and the rule that an ID is never reused is unchanged.

## Why the frozen controls were not touched

`scope.json`, `run-matrix.json` and `qualification-gates.json` contain **no
reference to their own directory**. Verified: `git grep fixed-points-20261108`
matched zero JSON files. So the move did not alter a single byte of them.

```
scope.json               70be4f5f5723600275619056888b3039c84a3f213f4257905520542d5ec3fc5d
run-matrix.json          312a68fdf423588397be7cf592d0c6d29636c4e3239e64fb5701707ee49b1d8a
```

Both still match every receipt's recorded `scope` and `run_matrix` bindings.

## Which bindings drifted

Receipts store hashes keyed by **logical name**, not by path, so moving the
directory does not itself break a binding. What changed the hashes is the
reference update inside four files that name the path:

| Binding | Drifted | Cause |
|---|---|---|
| `launcher` | yes | `W_ROOT` and the enforced `scope` path |
| `qualification_stager` | yes | `W_ROOT` |
| `variant_stager` | yes | `DEFAULT_OUT` |
| `b0_launcher` | yes | environment-override change (separate commit) |
| `scope` | no | no self-reference |
| `run_matrix` | no | no self-reference |
| `integrity` | no | path-independent |
| `pool_policy` | no | path-independent |
| `aggregator` | no | path-independent |

## What this means for the receipts

- The three W3 receipts (`tb`, `fw`, `ex`) were **already consumed** except
  EdgeX. They remain an accurate record of what ran and under which configuration.
  Their drifted bindings mean only that the *executing tree* has since changed,
  which is true and is not a claim about the run.
- **EdgeX W3 was not launched**, and its receipt can no longer authorize a launch
  against the current tree. It must not be forced. If EdgeX W3 is run, it needs a
  new receipt recorded against the current hashes, exactly as
  `scripts/record-w-authorization.py` requires.
- Any W4 receipt will bind the current tree, so this drift is a one-time
  transition cost, not an accumulating problem.

## What was deliberately not done

- The seven older consumed receipts were **not** re-recorded or regenerated.
  Backfilling a receipt would assert an approval that was given for a different
  tree, which is worse than a documented stale hash.
- No receipt file was edited. Their `bindings_sha256` values are the record.
- Historical evidence directories under `results/` were not renamed. Those name
  events, not configuration; the date is the identity, and `scope.json` pins 19
  of them by path and hash.

## Verification after the change

```
python tests        170 passed (2 skipped)
python offline        8 passed
go test -short       18 packages ok
workload-campaign    rows=60 problems=0 authorized=False
preregistration      OK
```

`execution_authorized` remains `false` and the matrix remains
`PROPOSED_NOT_LAUNCH_CLEARED`.
