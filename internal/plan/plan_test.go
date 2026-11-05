package plan

import (
	"fmt"
	"strings"
	"testing"

	"iotbench/internal/protocol"
)

const base = `{"schema_version":1,"plan_id":"t","purpose":"x","seed":1,"adapter":"fixture",
"target":{"base_url":"http://127.0.0.1:1"},"devices":10,"request_timeout_ms":1000,"connection":{"keep_alive":true},
"arrivals":{"model":"constant"},"vus":{"pre_allocated":5,"max":5},%SCHED%,
"health":{"min_achieved_ratio":0.99,"send_lag_p99_ms":10,"early_tolerance_ms":2},
"histogram_ms":[0,1,5,10,100],"cleanup":false,"test_faults":{}}`

func load(t *testing.T, sched string) (*Plan, error) {
	t.Helper()
	return Decode([]byte(strings.Replace(base, "%SCHED%", sched, 1)))
}

func TestStrictDecoding(t *testing.T) {
	if _, err := Decode([]byte(strings.Replace(base, "%SCHED%", `"schedule":{"type":"steady","rate":1,"duration_s":1},"bogus":1`, 1))); err == nil {
		t.Fatal("unknown field accepted")
	}
	if _, err := Decode([]byte(strings.Replace(base, "%SCHED%", `"schedule":{"type":"steady","rate":1,"duration_s":1}`, 1) + "{}")); err == nil {
		t.Fatal("trailing data accepted")
	}
}

func TestStepsAndSeqOffsets(t *testing.T) {
	p, err := load(t, `"schedule":{"type":"steps","steps":[{"phase":"warmup","rate":50,"duration_s":4},{"phase":"a","rate":7,"duration_s":3}]}`)
	if err != nil {
		t.Fatal(err)
	}
	r, err := p.Resolve("run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Steps) != 2 || r.Steps[0].Ticks != 200 || r.Steps[1].Ticks != 21 || r.Steps[1].SeqOffset != 200 ||
		r.Steps[1].StartS != 4 || r.TotalTicks != 221 || r.DurationS != 7 {
		t.Fatalf("unexpected resolution: %+v", r.Steps)
	}
}

func TestSweepRefineOverload(t *testing.T) {
	cases := map[string][]float64{
		`"schedule":{"type":"sweep","start_rate":100,"factor":1.5,"n_steps":4,"step_s":2,"warmup_s":2}`:                             {100, 100, 150, 225, 338},
		`"schedule":{"type":"refine","lo":300,"hi":400,"n_steps":3,"step_s":2}`:                                                     {325, 350, 375},
		`"schedule":{"type":"overload","capacity":1000,"phases":[{"fraction":0.5,"duration_s":2},{"fraction":1.2,"duration_s":2}]}`: {500, 1200},
	}
	for sched, want := range cases {
		p, err := load(t, sched)
		if err != nil {
			t.Fatal(err)
		}
		r, err := p.Resolve("run-1")
		if err != nil {
			t.Fatalf("%s: %v", sched, err)
		}
		if len(r.Steps) != len(want) {
			t.Fatalf("%s: %d steps, want %d", sched, len(r.Steps), len(want))
		}
		for i, s := range r.Steps {
			if s.Rate != want[i] {
				t.Fatalf("%s: step %d rate %v, want %v", sched, i, s.Rate, want[i])
			}
		}
	}
}

func TestAdapterPayloadSelectionIsExplicitAndStrict(t *testing.T) {
	schedule := `"schedule":{"type":"steady","rate":1,"duration_s":1}`
	for _, offset := range []int{0, 1024, 4096} {
		p, err := load(t, schedule+`,"payload":{"encoding_id":"logical-json-ascii-v1","size_offset_bytes":`+itoa(offset)+`}`)
		if err != nil {
			t.Fatalf("offset %d: %v", offset, err)
		}
		r, err := p.Resolve("run-1")
		if err != nil {
			t.Fatalf("offset %d: %v", offset, err)
		}
		if r.Payload == nil || r.Payload.SizeOffsetBytes == nil || *r.Payload.SizeOffsetBytes != offset {
			t.Fatalf("resolved payload control lost for offset %d: %+v", offset, r.Payload)
		}
	}
	// Absence is the legacy path: no implicit payload and no implicit size point.
	p, err := load(t, schedule)
	if err != nil {
		t.Fatal(err)
	}
	if r, err := p.Resolve("run-1"); err != nil || r.Payload != nil {
		t.Fatalf("legacy plan gained payload: %+v %v", r, err)
	}
	for name, extra := range map[string]string{
		"wrong encoding":  `,"payload":{"encoding_id":"other","size_offset_bytes":0}`,
		"missing offset":  `,"payload":{"encoding_id":"logical-json-ascii-v1"}`,
		"negative offset": `,"payload":{"encoding_id":"logical-json-ascii-v1","size_offset_bytes":-2}`,
		"odd offset":      `,"payload":{"encoding_id":"logical-json-ascii-v1","size_offset_bytes":1}`,
		"huge offset":     `,"payload":{"encoding_id":"logical-json-ascii-v1","size_offset_bytes":1048577}`,
		"fractional":      `,"payload":{"encoding_id":"logical-json-ascii-v1","size_offset_bytes":1.5}`,
		"unknown field":   `,"payload":{"encoding_id":"logical-json-ascii-v1","size_offset_bytes":0,"target":1}`,
	} {
		p, err := load(t, schedule+extra)
		if err == nil {
			_, err = p.Resolve("run-1")
		}
		if err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}

func TestPayloadOffsetUsesActualEncoderBound(t *testing.T) {
	schedule := `"schedule":{"type":"steady","rate":1,"duration_s":1}`
	runID := "run-payload-bound"
	deviceID := payloadDeviceID(runID, 10)
	baseline, err := protocol.PayloadBaselineUpperBound(runID, deviceID, 0)
	if err != nil {
		t.Fatal(err)
	}
	worst := protocol.LogicalPayload{
		RunID: runID, Seq: 0, DeviceID: deviceID, ScheduledSendMS: 1000000000000000.6,
		Temperature: 45, Humidity: 80, Pressure: 1150,
	}
	worstBytes, err := protocol.CanonicalBytes(worst)
	if err != nil {
		t.Fatal(err)
	}
	if baseline < len(worstBytes) {
		t.Fatalf("plan baseline bound=%d undercounts actual worst-case payload=%d", baseline, len(worstBytes))
	}
	maxOffset, err := protocol.MaximumPayloadOffset(baseline)
	if err != nil {
		t.Fatal(err)
	}
	withOffset := func(offset int) (*Plan, error) {
		return load(t, schedule+`,"payload":{"encoding_id":"logical-json-ascii-v1","size_offset_bytes":`+itoa(offset)+`}`)
	}
	for _, offset := range []int{0, maxOffset - 2, maxOffset} {
		p, err := withOffset(offset)
		if err != nil {
			t.Fatalf("offset %d: %v", offset, err)
		}
		if _, err := p.Resolve(runID); err != nil {
			t.Fatalf("offset %d at/under encoder bound rejected: %v", offset, err)
		}
	}
	for _, offset := range []int{maxOffset + 1, maxOffset + 2, protocol.PayloadMaximumBytes} {
		p, err := withOffset(offset)
		if err == nil {
			_, err = p.Resolve(runID)
		}
		if err == nil || !strings.Contains(err.Error(), "size_offset_bytes") {
			t.Fatalf("offset %d accepted or not actionable: %v", offset, err)
		}
	}
	// The accepted maximum must resolve and actually encode the pressure-1150 worst case for the
	// plan's longest device identity.
	p, err := withOffset(maxOffset)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Resolve(runID); err != nil {
		t.Fatalf("accepted maximum %d rejected: %v", maxOffset, err)
	}
	out, err := protocol.BuildPayloadOffset(worst, 17, maxOffset)
	if err != nil {
		t.Fatalf("accepted maximum does not encode pressure 1150: %v", err)
	}
	if len(out) != len(worstBytes)+maxOffset {
		t.Fatalf("accepted maximum encoded length=%d want %d", len(out), len(worstBytes)+maxOffset)
	}
}

func itoa(v int) string {
	return fmt.Sprintf("%d", v)
}

func TestRejects(t *testing.T) {
	for _, sched := range []string{
		`"schedule":{"type":"steady","rate":0.3,"duration_s":1}`, // 0.3 slots: not an integer number of ticks
		`"schedule":{"type":"nope"}`,
		`"schedule":{"type":"refine","lo":400,"hi":300,"n_steps":1,"step_s":1}`,
	} {
		p, err := load(t, sched)
		if err != nil {
			continue
		}
		if _, err := p.Resolve("run-1"); err == nil {
			t.Fatalf("accepted %s", sched)
		}
	}
	p, _ := load(t, `"schedule":{"type":"steady","rate":1,"duration_s":1}`)
	if _, err := p.Resolve("EXPL-edgex-k4"); err != nil {
		t.Fatalf("upper-case run id rejected: %v", err)
	}
	if _, err := p.Resolve("bad id"); err == nil {
		t.Fatal("run id with a space accepted")
	}
}
