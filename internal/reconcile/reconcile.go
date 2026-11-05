// Package reconcile joins a run's outcome ledger with what the endpoint stored, by message identity (run_id, seq).
//
// Ledger: every planned seq (all ticks, or the seed-selected ticks under thinning) is ack_success unless
// records.ndjson gives it another outcome. This is exact only for runs without dropped iterations, so such runs are
// refused rather than reconciled.
package reconcile

import (
	"fmt"
	"os"
	"sort"

	"iotbench/internal/hash"
	"iotbench/internal/plan"
	"iotbench/internal/report"
)

// Stored is one message identity found in the store.
type Stored struct {
	RunID string
	Seq   int64
}

type Result struct {
	RunID                   string  `json:"run_id"`
	Planned                 int64   `json:"planned_messages"`
	AckSuccess              int64   `json:"ack_success"`
	ExplicitFailure         int64   `json:"explicit_failure"`
	Unknown                 int64   `json:"unknown"`
	StoredUnique            int64   `json:"stored_unique"`
	AckLost                 []int64 `json:"ack_lost"`                    // legacy field name: acknowledged identity absent from this export, NOT demonstrated permanent loss
	PersistedDespiteUnknown []int64 `json:"persisted_despite_unknown"`   // timeout / transport error, but stored
	PersistedDespiteFailure []int64 `json:"persisted_despite_failure"`   // explicit failure, but stored
	BoundaryNotStored       []int64 `json:"boundary_failure_not_stored"` // adapter-boundary failure absent from this export: not clean
	Duplicates              []int64 `json:"duplicates"`                  // stored more than once
	DuplicateCopies         int64   `json:"duplicate_extra_copies"`
	Orphans                 []int64 `json:"orphans"` // stored seqs that were never planned
	OtherRunIDs             int64   `json:"stored_with_other_run_id"`
	MalformedIdentities     []int64 `json:"stored_with_missing_identity"` // rows whose run_id is absent: unaccountable, never counted as stored messages
	Clean                   bool    `json:"clean"`                        // no loss, no duplicates, no orphans
}

func planned(r *plan.Resolved) map[int64]bool {
	set := make(map[int64]bool, r.TotalTicks)
	t := hash.Threshold(r.SendProb)
	for _, st := range r.Steps {
		for seq := st.SeqOffset; seq < st.SeqOffset+st.Ticks; seq++ {
			if r.Arrivals.Model != "thinning" || hash.Selected(r.Seed, uint32(seq), t) {
				set[seq] = true
			}
		}
	}
	return set
}

// Run reconciles run directory dir against the stored identities.
func Run(dir string, stored []Stored) (*Result, error) {
	r, sum, health, err := report.Evaluate(dir)
	if err != nil {
		return nil, err
	}
	if sum == nil {
		return nil, fmt.Errorf("no k6 summary in %s", dir)
	}
	if sum.DroppedTotal > 0 {
		return nil, fmt.Errorf("run has %.0f dropped iterations: identities are not exact, reconciliation refused", sum.DroppedTotal)
	}
	if health == nil || !health.Valid {
		return nil, fmt.Errorf("offered-load health is invalid; reconciliation cannot establish a capacity claim")
	}
	if _, err := os.Stat(dir + "/records.ndjson"); err != nil {
		return nil, fmt.Errorf("required outcome ledger unavailable: %w", err)
	}
	recs, err := report.ReadNDJSON[report.Record](dir + "/records.ndjson")
	if err != nil {
		return nil, err
	}
	exp := planned(r)
	outcome := map[int64]string{}
	boundary := map[int64]bool{}
	for _, rc := range recs {
		if rc.Outcome == "ack_success" {
			continue
		}
		switch rc.Outcome {
		case "explicit_failure", "unknown":
		default:
			return nil, fmt.Errorf("outcome ledger has unknown outcome category %q at seq %d", rc.Outcome, rc.Seq)
		}
		outcome[rc.Seq] = rc.Outcome
		if _, ok := report.BoundarySubtype(rc.Detail); ok {
			boundary[rc.Seq] = true
		}
	}
	res := &Result{RunID: r.RunID, Planned: int64(len(exp))}
	count := map[int64]int64{}
	for _, s := range stored {
		if s.RunID == "" && s.Seq >= 0 {
			// A row with a valid seq but no run_id identity: it cannot join the plan nor prove storage.
			res.MalformedIdentities = append(res.MalformedIdentities, s.Seq)
			continue
		}
		if s.RunID != r.RunID {
			res.OtherRunIDs++
			continue
		}
		count[s.Seq]++
	}
	res.StoredUnique = int64(len(count))
	for seq := range exp {
		o := outcome[seq]
		if o == "" {
			o = "ack_success"
		}
		n := count[seq]
		switch o {
		case "ack_success":
			res.AckSuccess++
			if n == 0 {
				res.AckLost = append(res.AckLost, seq)
			}
		case "explicit_failure":
			res.ExplicitFailure++
			if n > 0 {
				res.PersistedDespiteFailure = append(res.PersistedDespiteFailure, seq)
			} else if boundary[seq] {
				// A boundary failure is not an endpoint rejection: if the identity is also absent
				// from the store, the export cannot establish that the message was retained.
				res.BoundaryNotStored = append(res.BoundaryNotStored, seq)
			}
		case "unknown":
			res.Unknown++
			if n > 0 {
				res.PersistedDespiteUnknown = append(res.PersistedDespiteUnknown, seq)
			}
		}
	}
	for seq, n := range count {
		if !exp[seq] {
			res.Orphans = append(res.Orphans, seq)
		}
		if n > 1 {
			res.Duplicates = append(res.Duplicates, seq)
			res.DuplicateCopies += n - 1
		}
	}
	for _, v := range []*[]int64{&res.AckLost, &res.PersistedDespiteUnknown, &res.PersistedDespiteFailure, &res.BoundaryNotStored, &res.Duplicates, &res.Orphans, &res.MalformedIdentities} {
		sort.Slice(*v, func(i, j int) bool { return (*v)[i] < (*v)[j] })
	}
	res.Clean = len(res.AckLost) == 0 && len(res.BoundaryNotStored) == 0 && len(res.Duplicates) == 0 && len(res.Orphans) == 0 &&
		len(res.MalformedIdentities) == 0
	return res, nil
}
