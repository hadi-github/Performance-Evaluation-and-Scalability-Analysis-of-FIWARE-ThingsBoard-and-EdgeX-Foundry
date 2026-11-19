package analysis

import (
	"encoding/hex"
	"encoding/json"
	"strings"
)

func provenanceClaim(r *Result, m map[string]json.RawMessage, c Configuration) Claim {
	var git, k6 map[string]string
	var scripts string
	_ = json.Unmarshal(m["git"], &git)
	_ = json.Unmarshal(m["k6"], &k6)
	_ = json.Unmarshal(m["k6_scripts_sha256"], &scripts)
	var reasons []string
	if !hexLength(git["sha"], 40) && !hexLength(git["sha"], 64) || git["dirty"] != "false" {
		reasons = append(reasons, "clean source commit not established")
	}
	if !imageDigest(k6["digest"]) || !hexLength(scripts, 64) {
		reasons = append(reasons, "generator image digest or executed script hash unavailable")
	}
	if c.ID == "" || !hexLength(c.EffectiveSHA256, 64) || c.CPUAllocationK <= 0 || len(c.Replicas) == 0 || len(c.Images) == 0 {
		reasons = append(reasons, "effective configuration hash, CPU budget, actual replicas or platform images missing")
	}
	for _, n := range c.Replicas {
		if n <= 0 {
			reasons = append(reasons, "invalid replica count")
		}
	}
	for _, image := range c.Images {
		if !imageDigest(image) {
			reasons = append(reasons, "platform image digest unavailable")
		}
	}
	if len(reasons) > 0 {
		return Claim{NotAssessed, reasons}
	}
	return claim(Pass, "provenance fields present; comparison still requires matching workload, versions, configuration, profile and endpoint")
}

func hexLength(s string, n int) bool {
	if len(s) != n {
		return false
	}
	_, e := hex.DecodeString(s)
	return e == nil
}
func imageDigest(s string) bool {
	parts := strings.Split(s, "@sha256:")
	return len(parts) == 2 && parts[0] != "" && hexLength(parts[1], 64)
}
