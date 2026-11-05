package sink

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"iotbench/internal/receipt"
)

func TestExtractPlatformBodies(t *testing.T) {
	cases := map[string]struct {
		body string
		want [][2]string
	}{
		"thingsboard rule-chain REST call (telemetry JSON)": {
			`{"run_id":"EXPL-tb-k4","seq":17,"scheduled_send_ms":1.5,"temperature":20}`, [][2]string{{"EXPL-tb-k4", "17"}}},
		"edgex app-service export (event with Object reading)": {
			`{"apiVersion":"v3","id":"x","deviceName":"d","readings":[{"resourceName":"reading","valueType":"Object",
			  "objectValue":{"run_id":"r1","seq":42,"temperature":1}}]}`, [][2]string{{"r1", "42"}}},
		"edgex wrapped event": {
			`{"apiVersion":"v3","event":{"readings":[{"objectValue":{"run_id":"r1","seq":"43"}}]}}`, [][2]string{{"r1", "43"}}},
		"orion notification keyValues": {
			`{"subscriptionId":"s","data":[{"id":"urn:a","type":"Sensor","seq":5,"run_id":"rf"},{"id":"urn:b","seq":6,"run_id":"rf"}]}`,
			[][2]string{{"rf", "5"}, {"rf", "6"}}},
		"orion notification normalized": {
			`{"data":[{"id":"urn:a","seq":{"type":"Number","value":7},"run_id":{"type":"Text","value":"rf"}}]}`, [][2]string{{"rf", "7"}}},
		"no identity":                        {`{"temperature":1}`, nil},
		"negative or fractional seq ignored": {`[{"run_id":"r","seq":-1},{"run_id":"r","seq":1.5}]`, nil},
	}
	for name, c := range cases {
		got, err := Extract([]byte(c.body))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(got) != len(c.want) {
			t.Fatalf("%s: got %v want %v", name, got, c.want)
		}
		seen := map[[2]string]bool{}
		for _, g := range got {
			seen[g] = true
		}
		for _, w := range c.want {
			if !seen[w] {
				t.Fatalf("%s: missing %v in %v", name, w, got)
			}
		}
	}
}

// Buffer overflow must never pass silently: with a bounded buffer and no drain, excess records are
// counted as overflow, every record still observes "received", and the counter is exact.
func TestServerBufferOverflowIsCountedNotSilent(t *testing.T) {
	// Same construction as New, but with no writer goroutine: the buffer cannot drain, which makes the
	// overflow path deterministic (New's writer drains concurrently in production).
	s := &Server{records: make(chan Record, 1), done: make(chan struct{}), seen: map[string]struct{}{}}
	srv := httptest.NewServer(s)
	for i := int64(1); i <= 5; i++ {
		res, err := http.Post(srv.URL+"/tb", "application/json",
			strings.NewReader(fmt.Sprintf(`{"run_id":"r","seq":%d}`, i)))
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("post %d: %v %v", i, err, res)
		}
		res.Body.Close()
	}
	srv.Close()
	if got := s.Overflow(); got != 4 { // capacity 1, five posted: one buffered, four dropped
		t.Fatalf("overflow=%d, want 4", got)
	}
	if s.Received() != 5 || s.dups.Load() != 0 {
		t.Fatalf("received=%d dups=%d, want 5/0", s.Received(), s.dups.Load())
	}
	// drain what was kept
	close(s.records)
	for len(s.records) > 0 {
		_ = <-s.records
	}
}

func TestServerRecordsAndDuplicates(t *testing.T) {
	var out bytes.Buffer
	s := New(&out, 100)
	srv := httptest.NewServer(s)
	post := func(path, body string) {
		res, err := http.Post(srv.URL+path, "application/json", strings.NewReader(body))
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("post: %v %v", err, res)
		}
		res.Body.Close()
	}
	post("/tb", `{"run_id":"r","seq":1}`)
	post("/tb", `{"run_id":"r","seq":2}`)
	post("/tb", `{"run_id":"r","seq":1}`) // duplicate
	post("/tb", `{"nothing":true}`)
	srv.Close()
	s.Close()
	var recs []Record
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var r Record
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			t.Fatal(err)
		}
		recs = append(recs, r)
	}
	if len(recs) != 3 || recs[2].Seq != 1 || !recs[2].Dup || recs[0].Dup || recs[0].Platform != "tb" || recs[0].RecvMs <= 0 || !recs[0].HasCompleteBodyReceipt() {
		t.Fatalf("records %+v", recs)
	}
	if s.dups.Load() != 1 || s.bad.Load() != 1 || s.Overflow() != 0 {
		t.Fatalf("counters dups=%d bad=%d overflow=%d", s.dups.Load(), s.bad.Load(), s.Overflow())
	}
}

// stagedBody reports when its final bytes have been delivered and can optionally fail instead of
// returning EOF. It lets the test prove the receipt timestamp is taken after the last body bytes.
type stagedBody struct {
	data []byte
	done bool
	err  error
}

func (b *stagedBody) Read(p []byte) (int, error) {
	if len(b.data) > 0 {
		n := copy(p, b.data)
		b.data = b.data[n:]
		if len(b.data) == 0 {
			b.done = true
		}
		return n, nil
	}
	if b.err != nil {
		return 0, b.err
	}
	return 0, io.EOF
}

func TestServerCompleteBodyReceiptIsAfterFinalBytesBeforeParse(t *testing.T) {
	var out bytes.Buffer
	s := New(&out, 10)
	body := &stagedBody{data: []byte(`{"run_id":"r","seq":1}`)}
	base := time.Unix(1700000000, 0)
	var clockCalls int
	var doneAtClock []bool
	s.now = func() time.Time {
		clockCalls++
		doneAtClock = append(doneAtClock, body.done)
		return base.Add(time.Duration(clockCalls) * time.Millisecond)
	}
	parseCalls := 0
	s.extract = func(b []byte) ([][2]string, error) {
		parseCalls++
		if clockCalls != 2 || !body.done {
			t.Fatalf("parse observed clockCalls=%d bodyDone=%v; receipt must be taken before parse", clockCalls, body.done)
		}
		return Extract(b)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://sink.invalid/tb", body)
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	s.Close()

	if clockCalls != 2 || parseCalls != 1 {
		t.Fatalf("clockCalls=%d parseCalls=%d, want 2/1", clockCalls, parseCalls)
	}
	if len(doneAtClock) != 2 || doneAtClock[0] || !doneAtClock[1] {
		t.Fatalf("body completion observed at clock calls %v, want [false true]", doneAtClock)
	}
	var got Record
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("records=%q", out.String())
	}
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatal(err)
	}
	if !got.HasCompleteBodyReceipt() || got.ReceiptCaptureSemantics != receipt.CaptureSemantics {
		t.Fatalf("record does not carry the versioned receipt semantics: %+v", got)
	}
	wantRecv := float64(base.Add(time.Millisecond).UnixNano()) / 1e6
	wantReceipt := float64(base.Add(2*time.Millisecond).UnixNano()) / 1e6
	if got.RecvMs != wantRecv || got.ReceiptCompleteBodyReadMS != wantReceipt {
		t.Fatalf("recv=%v receipt=%v want %v/%v", got.RecvMs, got.ReceiptCompleteBodyReadMS, wantRecv, wantReceipt)
	}
	if got.ReceiptCompleteBodyReadMS <= got.RecvMs {
		t.Fatalf("complete-body receipt %v is not after legacy arrival %v", got.ReceiptCompleteBodyReadMS, got.RecvMs)
	}
}

// errAfterReader returns its data and then a non-EOF error, modelling a truncated/failed body read.
type errAfterReader struct {
	data []byte
	err  error
}

func (r *errAfterReader) Read(p []byte) (int, error) {
	if len(r.data) > 0 {
		n := copy(p, r.data)
		r.data = r.data[n:]
		return n, nil
	}
	return 0, r.err
}

func TestServerIncompleteBodyEmitsNoRecordAndCountsDiagnostics(t *testing.T) {
	// The oversized case carries a valid identity and only trailing whitespace, so it would parse
	// successfully if the sink silently truncated it. Rejection must come from the body bound.
	validPrefix := []byte(`{"run_id":"r","seq":1}`)
	oversized := append(append([]byte(nil), validPrefix...), bytes.Repeat([]byte(" "), MaxBodyBytes+1-len(validPrefix))...)
	cases := []struct {
		name          string
		body          io.Reader
		wantOversized bool
	}{
		{"read error", &errAfterReader{data: []byte(`{"run_id":"r","seq":1}`), err: errors.New("connection reset")}, false},
		{"truncated body", &errAfterReader{data: []byte(`{"run_id":"r","seq":1}`), err: io.ErrUnexpectedEOF}, false},
		{"oversized body with valid prefix", bytes.NewReader(oversized), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			s := New(&out, 10)
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "http://sink.invalid/tb", tc.body))
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d", rec.Code)
			}
			// Metrics remain observable after a rejection.
			mrec := httptest.NewRecorder()
			s.ServeHTTP(mrec, httptest.NewRequest(http.MethodGet, "http://sink.invalid/metrics", nil))
			metrics := mrec.Body.String()
			s.Close()

			if out.Len() != 0 {
				t.Fatalf("incomplete body fabricated a record: %q", out.String())
			}
			if s.Received() != 0 || s.dups.Load() != 0 || s.Overflow() != 0 {
				t.Fatalf("received=%d dups=%d overflow=%d", s.Received(), s.dups.Load(), s.Overflow())
			}
			if s.IncompleteBody() != 1 || s.BodyReadErrors() != boolInt(!tc.wantOversized) || s.OversizedBody() != boolInt(tc.wantOversized) {
				t.Fatalf("incomplete=%d readErrors=%d oversized=%d", s.IncompleteBody(), s.BodyReadErrors(), s.OversizedBody())
			}
			if s.bad.Load() != 1 {
				t.Fatalf("unparsed legacy health count=%d, want 1", s.bad.Load())
			}
			for _, want := range []string{"sink_incomplete_body_total 1", "sink_body_read_error_total ", "sink_oversized_body_total "} {
				if !strings.Contains(metrics, want) {
					t.Fatalf("metrics missing %q:\n%s", want, metrics)
				}
			}
			if tc.wantOversized && !strings.Contains(metrics, "sink_oversized_body_total 1") {
				t.Fatalf("oversized metric not incremented:\n%s", metrics)
			}
		})
	}
}

func boolInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

func TestServerAcceptsBodyAtExactBound(t *testing.T) {
	payload := []byte(`{"run_id":"r","seq":1}`)
	body := append(append([]byte(nil), payload...), bytes.Repeat([]byte(" "), MaxBodyBytes-len(payload))...)
	if len(body) != MaxBodyBytes {
		t.Fatalf("body len=%d", len(body))
	}
	var out bytes.Buffer
	s := New(&out, 10)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "http://sink.invalid/tb", bytes.NewReader(body)))
	s.Close()
	if s.IncompleteBody() != 0 || s.Received() != 1 {
		t.Fatalf("incomplete=%d received=%d", s.IncompleteBody(), s.Received())
	}
	var got Record
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &got); err != nil {
		t.Fatal(err)
	}
	if !got.HasCompleteBodyReceipt() {
		t.Fatalf("record missing receipt semantics: %+v", got)
	}
}

func TestLegacySinkRecordStaysLegacy(t *testing.T) {
	var legacy Record
	if err := json.Unmarshal([]byte(`{"recv_ms":1700000000000.5,"platform":"tb","run_id":"r","seq":1,"dup":false}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.HasCompleteBodyReceipt() || legacy.ReceiptCaptureSemantics != "" || legacy.ReceiptCompleteBodyReadMS != 0 {
		t.Fatalf("legacy record was silently reinterpreted: %+v", legacy)
	}
	var current Record
	if err := json.Unmarshal([]byte(`{"recv_ms":1700000000000.5,"receipt_complete_body_read_ms":1700000000000.9,"receipt_capture_semantics":"`+receipt.CaptureSemantics+`","platform":"tb","run_id":"r","seq":1,"dup":false}`), &current); err != nil {
		t.Fatal(err)
	}
	if !current.HasCompleteBodyReceipt() {
		t.Fatalf("new record was not recognized: %+v", current)
	}
}

func TestServerConcurrentRequestsRecordConsistently(t *testing.T) {
	var out bytes.Buffer
	s := New(&out, 1000)
	srv := httptest.NewServer(s)
	const n = 32
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			res, err := http.Post(srv.URL+"/tb", "application/json", strings.NewReader(fmt.Sprintf(`{"run_id":"r","seq":%d}`, i)))
			if err != nil {
				t.Errorf("post %d: %v", i, err)
				return
			}
			_, _ = io.Copy(io.Discard, res.Body)
			res.Body.Close()
		}(i)
	}
	wg.Wait()
	srv.Close() // wait for every handler to finish before closing the record channel
	s.Close()

	seen := map[int64]bool{}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var r Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatal(err)
		}
		if !r.HasCompleteBodyReceipt() || r.ReceiptCaptureSemantics != receipt.CaptureSemantics {
			t.Fatalf("record missing complete-body receipt: %+v", r)
		}
		if seen[r.Seq] {
			t.Fatalf("duplicate seq %d in unique workload", r.Seq)
		}
		seen[r.Seq] = true
		if r.Dup {
			t.Fatalf("unexpected duplicate flag for seq %d", r.Seq)
		}
	}
	if len(seen) != n || s.Received() != n || s.dups.Load() != 0 || s.Overflow() != 0 {
		t.Fatalf("records=%d received=%d dups=%d overflow=%d", len(seen), s.Received(), s.dups.Load(), s.Overflow())
	}
}

func TestServerConcurrentDuplicateIdentityHasOneFirstObservation(t *testing.T) {
	var out bytes.Buffer
	s := New(&out, 1000)
	srv := httptest.NewServer(s)
	const n = 16
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			res, err := http.Post(srv.URL+"/tb", "application/json", strings.NewReader(`{"run_id":"r","seq":1}`))
			if err != nil {
				t.Errorf("post: %v", err)
				return
			}
			_, _ = io.Copy(io.Discard, res.Body)
			res.Body.Close()
		}()
	}
	wg.Wait()
	srv.Close() // wait for every handler to finish before closing the record channel
	s.Close()
	first, dups := 0, 0
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var r Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatal(err)
		}
		if r.Dup {
			dups++
		} else {
			first++
		}
	}
	if first != 1 || dups != n-1 || s.Received() != n || s.dups.Load() != int64(n-1) {
		t.Fatalf("first=%d dups=%d received=%d counter=%d", first, dups, s.Received(), s.dups.Load())
	}
}
