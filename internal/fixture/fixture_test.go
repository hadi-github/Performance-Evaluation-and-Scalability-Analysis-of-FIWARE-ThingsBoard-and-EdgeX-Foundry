package fixture

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFixtureCapturesActualPaddingAndKeepsLegacyRecordsUnchanged(t *testing.T) {
	var out bytes.Buffer
	server := New(Config{}, &out, 16)
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()

	padded := `{"run_id":"SYNTHETIC-m1e-fixture","seq":1,"device_id":"bench-SYNTHETIC-m1e-fixture-00001",` +
		`"scheduled_send_ms":1700000001000,"temperature":20,"humidity":40,"pressure":1000,"padding":"` +
		strings.Repeat("ab", 2048) + `"}`
	res, err := http.Post(httpServer.URL+"/ingest", "application/json", strings.NewReader(padded))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	legacy := `{"run_id":"SYNTHETIC-m1e-fixture","seq":2,"device_id":"bench-SYNTHETIC-m1e-fixture-00002",` +
		`"scheduled_send_ms":1700000002000,"temperature":20,"humidity":40,"pressure":1000}`
	res, err = http.Post(httpServer.URL+"/ingest", "application/json", strings.NewReader(legacy))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	server.Close()

	records := []Record{}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var r Record
		if err = json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatal(err)
		}
		records = append(records, r)
	}
	if len(records) != 2 {
		t.Fatalf("records=%d want 2", len(records))
	}
	if records[0].Padding != strings.Repeat("ab", 2048) {
		t.Fatalf("actual padding not captured: len=%d", len(records[0].Padding))
	}
	if records[0].RunID != "SYNTHETIC-m1e-fixture" || records[0].Seq != 1 || records[0].Device != "bench-SYNTHETIC-m1e-fixture-00001" {
		t.Fatalf("padded identity changed: %+v", records[0])
	}
	if records[1].Padding != "" {
		t.Fatalf("legacy record gained padding: %q", records[1].Padding)
	}
	raw, err := json.Marshal(records[1])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(`"padding"`)) {
		t.Fatal("legacy record JSON was relabelled with a padding field")
	}
}
