// Package sink receives each platform's outbound copy of a message and records the observation: the latency
// endpoint "reached the sink" and the basis of outstanding work (sent - reached sink, by unique ID).
//
// Any JSON body is accepted. Every object in it that has both "run_id" and "seq" is one observed message; NGSI-style
// attributes ({"value": ...}) are unwrapped. This covers the ThingsBoard rule-chain REST call (telemetry JSON),
// EdgeX app-service export (event JSON with readings[].objectValue) and Orion notifications (data[] entities).
// The URL path names the platform. Records go through a bounded buffer; overflow is counted, reported on /metrics
// and makes the process exit non-zero, so a lossy capture can never pass silently.
package sink

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Record is one observed message. RecvMs is the sink's wall clock (epoch ms, sub-ms resolution).
type Record struct {
	RecvMs   float64 `json:"recv_ms"`
	Platform string  `json:"platform"`
	RunID    string  `json:"run_id"`
	Seq      int64   `json:"seq"`
	Dup      bool    `json:"dup"` // this (run_id, seq) was observed before
}

type Server struct {
	start     time.Time
	records   chan Record
	done      chan struct{}
	closeOnce sync.Once
	mu        sync.Mutex
	seen      map[string]struct{}
	received  atomic.Int64 // observed messages
	dups      atomic.Int64
	overflow  atomic.Int64
	bad       atomic.Int64 // requests without any (run_id, seq)
	requests  atomic.Int64
}

func New(w io.Writer, buffer int) *Server {
	s := &Server{start: time.Now(), records: make(chan Record, buffer), done: make(chan struct{}), seen: map[string]struct{}{}}
	go func() {
		bw := bufio.NewWriterSize(w, 1<<20)
		enc := json.NewEncoder(bw)
		for r := range s.records {
			_ = enc.Encode(r)
		}
		_ = bw.Flush()
		close(s.done)
	}()
	return s
}

func (s *Server) Overflow() int64 { return s.overflow.Load() }
func (s *Server) Received() int64 { return s.received.Load() }

// Close flushes the records; call after the HTTP server has stopped.
func (s *Server) Close() {
	s.closeOnce.Do(func() { close(s.records) })
	<-s.done
}

func seqOf(v any) (int64, bool) {
	if m, ok := v.(map[string]any); ok { // NGSI attribute {"type": ..., "value": ...}
		v = m["value"]
	}
	switch x := v.(type) {
	case float64:
		return int64(x), x == float64(int64(x)) && x >= 0
	case string:
		n, err := strconv.ParseInt(x, 10, 64)
		return n, err == nil && n >= 0
	}
	return 0, false
}

func strOf(v any) (string, bool) {
	if m, ok := v.(map[string]any); ok {
		v = m["value"]
	}
	s, ok := v.(string)
	return s, ok && s != ""
}

// Extract returns every (run_id, seq) pair found in a JSON document.
func Extract(body []byte) ([][2]string, error) {
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	var out [][2]string
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if r, ok := strOf(x["run_id"]); ok {
				if q, ok := seqOf(x["seq"]); ok {
					out = append(out, [2]string{r, strconv.FormatInt(q, 10)})
				}
			}
			for _, c := range x {
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(doc)
	return out, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/metrics":
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP sink_requests_total HTTP requests received.\n# TYPE sink_requests_total counter\nsink_requests_total %d\n", s.requests.Load())
		fmt.Fprintf(w, "# HELP sink_received_total Messages observed (objects with run_id and seq), incl. duplicates.\n# TYPE sink_received_total counter\nsink_received_total %d\n", s.received.Load())
		fmt.Fprintf(w, "# HELP sink_duplicates_total Observations of an already observed (run_id, seq).\n# TYPE sink_duplicates_total counter\nsink_duplicates_total %d\n", s.dups.Load())
		fmt.Fprintf(w, "# HELP sink_unparsed_total Requests without any (run_id, seq).\n# TYPE sink_unparsed_total counter\nsink_unparsed_total %d\n", s.bad.Load())
		fmt.Fprintf(w, "# HELP sink_overflow_total Records lost to buffer overflow (must stay 0).\n# TYPE sink_overflow_total counter\nsink_overflow_total %d\n", s.overflow.Load())
		return
	}
	now := time.Now()
	s.requests.Add(1)
	body, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	w.WriteHeader(http.StatusOK) // answer first: the sink must not slow the platform's export path
	pairs, err := Extract(body)
	if err != nil || len(pairs) == 0 {
		s.bad.Add(1)
		return
	}
	platform := strings.Trim(r.URL.Path, "/")
	recvMs := float64(now.UnixNano()) / 1e6
	for _, p := range pairs {
		seq, _ := strconv.ParseInt(p[1], 10, 64)
		key := p[0] + "\x00" + p[1]
		s.mu.Lock()
		_, dup := s.seen[key]
		if !dup {
			s.seen[key] = struct{}{}
		}
		s.mu.Unlock()
		s.received.Add(1)
		if dup {
			s.dups.Add(1)
		}
		select {
		case s.records <- Record{RecvMs: recvMs, Platform: platform, RunID: p[0], Seq: seq, Dup: dup}:
		default:
			s.overflow.Add(1)
		}
	}
}
