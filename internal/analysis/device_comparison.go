package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

func validateW2Reference(rd reader, ref *W2Reference) Claim {
	if ref == nil || ref.Capacity == nil || !finite(*ref.Capacity) || *ref.Capacity <= 0 {
		return claim(NotAssessed, "missing w2_reference.capacity_messages_per_second: defined positive C_SLO(500,1) reference required")
	}
	if ref.Definition != "C_SLO(500,1)" || ref.Source == "" || ref.Path == "" || !hexLength(ref.SHA256, 64) {
		return claim(NotAssessed, "w2_reference requires definition C_SLO(500,1), source, path and sha256 of reference evidence")
	}
	b, e := rd.read("w2_reference", ref.Path)
	if e != nil {
		return claim(NotAssessed, "w2_reference evidence file unavailable: "+ref.Path)
	}
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != ref.SHA256 {
		return claim(Invalid, "w2_reference evidence SHA-256 mismatch")
	}
	seen := map[float64]bool{}
	for _, point := range ref.RatePoints {
		if (point.Fraction != .5 && point.Fraction != .75) || seen[point.Fraction] || !finite(point.Rate) || point.Rate <= 0 || point.Source == "" {
			return claim(Invalid, "W2 rate_points require distinct fractions 0.5/0.75, positive rates and explicit sources")
		}
		seen[point.Fraction] = true
	}
	return claim(Pass, "explicit sourced reference and evidence hash supplied; not inferred from comparison inputs or independently requalified")
}

type DeviceStudyCell struct {
	RunID                string            `json:"run_id"`
	Step                 int               `json:"step"`
	Devices              int               `json:"devices"`
	OfferedRate          float64           `json:"offered_rate_messages_per_second"`
	ReferenceFraction    *float64          `json:"reference_fraction,omitempty"`
	ActualReferenceRatio *float64          `json:"actual_rate_to_reference_ratio,omitempty"`
	Validity             Claim             `json:"design_cell_validity"`
	Metrics              map[string]Metric `json:"metrics"`
	Claims               map[string]Claim  `json:"claims"`
}

type DeviceStudy struct {
	Key            string               `json:"key"`
	ProfileID      string               `json:"profile_id"`
	Design         DeviceComparisonSpec `json:"design"`
	Reference      *W2Reference         `json:"reference,omitempty"`
	DesignValidity Claim                `json:"design_validity"`
	ProfileVerdict Claim                `json:"profile_verdict"`
	Cells          []DeviceStudyCell    `json:"cells"`
}

// Only the W2 design check crosses device populations. Measurements stay in
// separate cells; no p99 averaging or invented "high devices" cutoff is used.
func compareDevices(results []*Result) []DeviceStudy {
	groups := map[string]*DeviceStudy{}
	seenRuns := map[string]bool{}
	for _, r := range results {
		duplicate := seenRuns[r.RunID]
		seenRuns[r.RunID] = true
		for _, s := range r.Steps {
			for _, a := range s.Profiles {
				if a.Profile.DeviceComparison == nil {
					continue
				}
				var ref *W2Reference
				if r.CaptureSpecification != nil {
					ref = r.CaptureSpecification.W2Reference
				}
				config := r.Configuration
				config.EffectivePath = ""
				var refKey any
				if ref != nil {
					copy := *ref
					copy.Path = ""
					refKey = copy
				}
				qualifiers := map[string]any{}
				if ev := r.CaptureSpecification; ev != nil {
					qualifiers["sink_clock"] = ev.SinkClock
					qualifiers["drain_ms"] = ev.ObservationDeadlineMS - s.EndMS
					qualifiers["history_collection_ms"] = ev.History.CollectedAtMS - s.EndMS
				}
				b, _ := json.Marshal([]any{r.Platform, r.Synthetic, r.Seed, r.Interpretation, r.Provenance, config, a.Profile, s.Phase, s.EndMS - s.StartMS, refKey, qualifiers})
				h := sha256.Sum256(b)
				key := hex.EncodeToString(h[:])
				group := groups[key]
				if group == nil {
					group = &DeviceStudy{Key: key, ProfileID: a.Profile.ID, Design: *a.Profile.DeviceComparison, Reference: ref, ProfileVerdict: claim(NotAssessed, "W2 is a descriptive matched-rate device-population comparison, not a device-capacity cutoff or campaign verdict")}
					groups[key] = group
				}
				cell := DeviceStudyCell{RunID: r.RunID, Step: s.Index, Devices: s.Devices, OfferedRate: s.OfferedRate, Metrics: s.Metrics, Claims: s.Claims, Validity: claim(Pass, "matched-rate W2 cell with configured and independently observed population, N=1")}
				var missing []string
				if a.Profile.Source == "" {
					missing = append(missing, "W2 profile decision source missing")
				}
				if duplicate {
					missing = append(missing, "duplicate run input is not another study cell")
				}
				if r.Claims["comparability"].Status != Pass {
					missing = append(missing, "verified configuration/image/source provenance unavailable")
				}
				if s.Claims["offered_load"].Status != Pass {
					missing = append(missing, "valid offered-load evidence unavailable")
				}
				if r.Claims["w2_reference"].Status != Pass {
					missing = append(missing, r.Claims["w2_reference"].Reasons...)
					if len(r.Claims["w2_reference"].Reasons) == 0 {
						missing = append(missing, "defined reference-capacity evidence unavailable")
					}
				}
				if s.Devices != 100 && s.Devices != 10000 {
					missing = append(missing, "W2 needs configured population 100 or 10000")
				}
				if config.CPUAllocationK != 1 || len(config.Replicas) == 0 {
					missing = append(missing, "N=1 requires verified application CPU budget k=1 and replica inventory")
				}
				for service, n := range config.Replicas {
					if n != 1 {
						missing = append(missing, "N=1 requires one replica per service: "+service)
					}
				}
				observed := s.Metrics["workload.observed_history_devices"]
				if s.Claims["history"].Status != Pass || observed.Value == nil || *observed.Value != float64(s.Devices) {
					missing = append(missing, "complete independently retrieved history/window and observed population matching configured devices required")
				}
				if ref != nil && ref.Capacity != nil && *ref.Capacity > 0 && finite(*ref.Capacity) {
					ratio := s.OfferedRate / *ref.Capacity
					cell.ActualReferenceRatio = &ratio
					for _, fraction := range []float64{.5, .75} {
						target := fraction * *ref.Capacity
						for _, point := range ref.RatePoints {
							if point.Fraction == fraction {
								target = point.Rate
							}
						}
						if s.OfferedRate == target {
							v := fraction
							cell.ReferenceFraction = &v
							break
						}
					}
				}
				if cell.ReferenceFraction == nil {
					missing = append(missing, "offered rate does not match a defined 50%/75% reference rate; supply sourced rate_points for approximate rates (no tolerance guessed)")
				}
				if len(missing) > 0 {
					cell.Validity = Claim{NotAssessed, missing}
				}
				group.Cells = append(group.Cells, cell)
			}
		}
	}
	var out []DeviceStudy
	for _, key := range sortedKeys(groups) {
		g := groups[key]
		var missing []string
		for _, fraction := range g.Design.ReferenceFractions {
			for _, population := range g.Design.Populations {
				found := false
				for _, cell := range g.Cells {
					if cell.Validity.Status == Pass && cell.ReferenceFraction != nil && *cell.ReferenceFraction == fraction && cell.Devices == population {
						found = true
					}
				}
				if !found {
					missing = append(missing, fmt.Sprintf("missing eligible %d-device cell at reference fraction %g", population, fraction))
				}
			}
		}
		if len(missing) > 0 {
			g.DesignValidity = Claim{NotAssessed, missing}
		} else {
			g.DesignValidity = claim(Pass, "both populations observed at both matched aggregate reference rates, N=1; measurements remain separate, no effect significance or capacity verdict implied")
		}
		out = append(out, *g)
	}
	return out
}
