// Package reconcile joins a run's outcome ledger with what the endpoint stored, by message identity (run_id, seq).
//
// Ledger: every planned seq (all ticks, or the seed-selected ticks under thinning) is ack_success unless
// records.ndjson gives it another outcome. This is exact only for runs without dropped iterations, so such runs are
// refused rather than reconciled.
package reconcile

import (
	"fmt"
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
	AckLost                 []int64 `json:"ack_lost"`                  // acknowledged, absent from the store
	PersistedDespiteUnknown []int64 `json:"persisted_despite_unknown"` // timeout / transport error, but stored
	PersistedDespiteFailure []int64 `json:"persisted_despite_failure"` // explicit failure, but stored
	Duplicates              []int64 `json:"duplicates"`                // stored more than once
	DuplicateCopies         int64   `json:"duplicate_extra_copies"`
	Orphans                 []int64 `json:"orphans"` // stored seqs that were never planned
	OtherRunIDs             int64   `json:"stored_with_other_run_id"`
	Clean                   bool    `json:"clean"` // no loss, no duplicates, no orphans
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
	r, sum, _, err := report.Evaluate(dir)
	if err != nil {
		return nil, err
	}
	if sum == nil {
		return nil, fmt.Errorf("no k6 summary in %s", dir)
	}
	if sum.DroppedTotal > 0 {
		return nil, fmt.Errorf("run has %.0f dropped iterations: identities are not exact, reconciliation refused", sum.DroppedTotal)
	}
	recs, err := report.ReadNDJSON[report.Record](dir + "/records.ndjson")
	if err != nil {
		return nil, err
	}
	exp := planned(r)
	outcome := map[int64]string{}
	for _, rc := range recs {
		if rc.Outcome != "ack_success" {
			outcome[rc.Seq] = rc.Outcome
		}
	}
	res := &Result{RunID: r.RunID, Planned: int64(len(exp))}
	count := map[int64]int64{}
	for _, s := range stored {
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
	for _, v := range []*[]int64{&res.AckLost, &res.PersistedDespiteUnknown, &res.PersistedDespiteFailure, &res.Duplicates, &res.Orphans} {
		sort.Slice(*v, func(i, j int) bool { return (*v)[i] < (*v)[j] })
	}
	res.Clean = len(res.AckLost) == 0 && len(res.Duplicates) == 0 && len(res.Orphans) == 0
	return res, nil
}
