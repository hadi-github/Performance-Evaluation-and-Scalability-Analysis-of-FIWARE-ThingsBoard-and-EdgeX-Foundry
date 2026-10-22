package sink

import (
	"bytes"
	"encoding/json"
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
