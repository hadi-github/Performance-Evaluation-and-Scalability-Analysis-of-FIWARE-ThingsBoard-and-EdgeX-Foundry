package offline

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"iotbench/internal/plan"
	"iotbench/internal/protocol"
	"iotbench/internal/runner"
)

// TestAdapterPayloadNodeProjection runs the real k6 payload/adapter projection helpers under Node.
// It is a synthetic offline fixture: the Node script starts only a loopback HTTP receiver.
func TestAdapterPayloadNodeProjection(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not available; adapter projection parity is exercised when Node is installed")
	}
	cmd := exec.Command(node, "adapter-payload.test.mjs")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node adapter payload test failed: %v\n%s", err, out)
	}
	var result struct {
		Status   string   `json:"status"`
		Vectors  int      `json:"vectors"`
		Adapters []string `json:"adapters"`
		Offsets  []int    `json:"offsets"`
	}
	if err = json.Unmarshal(bytes.TrimSpace(out), &result); err != nil {
		t.Fatalf("node output is not JSON: %v\n%s", err, out)
	}
	if result.Status != "ok" || result.Vectors != 12 || len(result.Adapters) != 4 || len(result.Offsets) != 3 {
		t.Fatalf("unexpected Node result: %+v", result)
	}
}

// TestAdapterPayloadPlanFlowsThroughOfflineRunner proves the explicit plan control reaches
// resolved.json and the manifest without changing protocol/profile hashes. The stored export is a
// synthetic projection of actual stored fields; padding is read back, never regenerated to prove it.
func TestAdapterPayloadPlanFlowsThroughOfflineRunner(t *testing.T) {
	offset := 1024
	p := &plan.Plan{
		SchemaVersion: 1, PlanID: "m1e-offline-payload", Purpose: "SYNTHETIC OFFLINE ENGINEERING FIXTURE; not live evidence",
		PilotOnly: true, Seed: 17, Adapter: "fixture",
		Target: plan.Target{BaseURL: "http://synthetic.invalid"}, Devices: 5, RequestTimeMs: 1000,
		Connection: plan.Connection{KeepAlive: true}, Arrivals: plan.Arrivals{Model: "constant"},
		VUs:      plan.VUs{PreAllocated: 5, Max: 5},
		Schedule: plan.Schedule{Type: "steps", Steps: []plan.StepSpec{{Phase: "measurement", Rate: 1, DurationS: 5}}},
		Health:   plan.Health{MinAchievedRatio: .99, SendLagP99Ms: 10, EarlyToleranceMs: 2},
		Payload:  &plan.PayloadPlan{EncodingID: protocol.PayloadEncodingID, SizeOffsetBytes: &offset},
	}
	planPath := filepath.Join(t.TempDir(), "plan.json")
	writeJSON(t, planPath, p)
	f, err := runner.PrepareOfflineFixture(runner.Options{PlanPath: planPath, RepoRoot: repoRoot(t), ResultsDir: t.TempDir(), RunID: "SYNTHETIC-m1e-plan"})
	if err != nil {
		t.Fatal(err)
	}
	if f.Resolved.Payload == nil || f.Resolved.Payload.SizeOffsetBytes == nil || *f.Resolved.Payload.SizeOffsetBytes != offset {
		t.Fatalf("resolved payload control missing: %+v", f.Resolved.Payload)
	}
	if f.Manifest.Resolved == nil || f.Manifest.Resolved.Payload == nil {
		t.Fatalf("manifest resolved payload control missing: %+v", f.Manifest.Resolved)
	}
	var manifestRaw map[string]any
	readJSON(t, filepath.Join(f.Dir, "manifest.json"), &manifestRaw)
	if _, ok := manifestRaw["resolved_plan"].(map[string]any)["payload"]; !ok {
		t.Fatal("resolved payload control was dropped from manifest")
	}

	// Actual stored-export projection: all eight canonical fields, including the padding value that
	// was emitted by BuildPayload. The three platform-native JSON adapters store this object directly;
	// the FIWARE script projects the same actual text fields. No padding is re-derived here.
	type stored struct {
		seq    uint32
		padded []byte
		want   protocol.LogicalPayload
	}
	var rows []stored
	for seq := uint32(0); seq < 3; seq++ {
		logical := protocol.LogicalPayload{
			RunID: "SYNTHETIC-m1e-plan", Seq: seq, DeviceID: "bench-SYNTHETIC-m1e-plan-0000" + string(rune('0'+seq)),
			ScheduledSendMS: 1700000000000 + float64(seq)*1000, Temperature: 20, Humidity: 40, Pressure: 1000,
		}
		base, err := protocol.CanonicalBytes(logical)
		if err != nil {
			t.Fatal(err)
		}
		padded, err := protocol.BuildPayload(logical, 17, len(base)+offset)
		if err != nil {
			t.Fatal(err)
		}
		var storedLogical protocol.LogicalPayload
		if err = json.Unmarshal(padded, &storedLogical); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, stored{seq: seq, padded: padded, want: storedLogical})
	}
	path := filepath.Join(t.TempDir(), "stored-export.ndjson")
	var export strings.Builder
	for _, row := range rows {
		export.Write(row.padded)
		export.WriteByte('\n')
	}
	if err = os.WriteFile(path, []byte(export.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range bytes.Split(bytes.TrimSpace(raw), []byte("\n")) {
		var identity struct {
			Seq uint32 `json:"seq"`
		}
		if err = json.Unmarshal(line, &identity); err != nil {
			t.Fatal(err)
		}
		if err = protocol.ValidateStored(rows[i].want, line); err != nil {
			t.Fatalf("actual stored export line %d rejected: %v", i, err)
		}
	}

	// Matching identity is not sufficient: omitted, truncated and changed padding must fail.
	first := rows[0].padded
	omitted := bytes.Replace(first, []byte(`,"padding":"`), []byte(`,"padding_removed":"`), 1)
	truncated := append(append([]byte(nil), first[:len(first)-2]...), first[len(first)-1])
	changed := append([]byte(nil), first...)
	changed[len(changed)-3] ^= 0x01
	for name, bad := range map[string][]byte{"omitted": omitted, "truncated": truncated, "changed": changed} {
		var identity struct {
			RunID string `json:"run_id"`
			Seq   uint32 `json:"seq"`
		}
		if err = json.Unmarshal(bad, &identity); err == nil {
			if identity.RunID != rows[0].want.RunID || identity.Seq != rows[0].seq {
				t.Fatalf("%s corruption unexpectedly changed identity", name)
			}
		}
		if err = protocol.ValidateStored(rows[0].want, bad); err == nil {
			t.Fatalf("%s padding was accepted", name)
		}
	}
}
