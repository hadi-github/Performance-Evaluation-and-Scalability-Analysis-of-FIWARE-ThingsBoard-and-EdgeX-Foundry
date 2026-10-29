package sink

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	if len(recs) != 3 || recs[2].Seq != 1 || !recs[2].Dup || recs[0].Dup || recs[0].Platform != "tb" || recs[0].RecvMs <= 0 {
		t.Fatalf("records %+v", recs)
	}
	if s.dups.Load() != 1 || s.bad.Load() != 1 || s.Overflow() != 0 {
		t.Fatalf("counters dups=%d bad=%d overflow=%d", s.dups.Load(), s.bad.Load(), s.Overflow())
	}
}
