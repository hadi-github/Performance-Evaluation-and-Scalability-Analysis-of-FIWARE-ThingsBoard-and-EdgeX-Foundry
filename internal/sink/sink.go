// Package sink receives each platform's outbound copy of a message and records the observation: the latency
// endpoint "reached the sink" and the basis of outstanding work (sent - reached sink, by unique ID).
//
// Any JSON body is accepted. Every object in it that has both "run_id" and "seq" is one observed message; NGSI-style
// attributes ({"value": ...}) are unwrapped. This covers the ThingsBoard rule-chain REST call (telemetry JSON),
// EdgeX app-service export (event JSON with readings[].objectValue) and Orion notifications (data[] entities).
// The URL path names the platform. Records go through a bounded buffer; overflow is counted, reported on /metrics
// and makes the process exit non-zero, so a lossy capture can never pass silently.
//
// New records carry receipt_capture_semantics=complete-body-read-before-parse-v1 and a
// receipt_complete_body_read_ms timestamp taken only after the bounded complete body read succeeds
// and before parsing. The legacy recv_ms field is unchanged. A failed, truncated or oversized body
// emits no receipt record; it increments sink_incomplete_body_total (with read-error/oversized
// breakdown) while leaving overflow and received-message accounting intact.
package sink

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"iotbench/internal/receipt"
)

// MaxBodyBytes bounds the receiver's per-request body memory. A body that exceeds this bound is
// rejected without a receipt record; it is never silently truncated and parsed.
const MaxBodyBytes = 4 << 20

var errBodyTooLarge = errors.New("sink: request body exceeds the bounded maximum")

// Record is one observed message.
//
// RecvMs is the legacy request-arrival wall clock (epoch ms, sub-ms resolution), taken before the
// body is read. It is retained unchanged and is never reinterpreted as a complete-body receipt.
//
// ReceiptCompleteBodyReadMS is the versioned complete-body receipt time: it is taken immediately
// after a successful, bounded read of the entire request body and before any JSON parsing or
// identity extraction. It uses the same shared-load-realtime wall-clock epoch-millisecond domain as
// RecvMs, but no monotonic or cross-host clock qualification is claimed.
//
// ReceiptCaptureSemantics carries the explicit contract identifier. Legacy records omit both new
// fields, so a reader can distinguish them instead of inferring the new semantics by default.
//
// One HTTP body may contain several identities (a batch). Every record extracted from that body
// shares the same request-level complete-body receipt time; it is not a per-message parse time or
// a per-message platform delivery time.
type Record struct {
	RecvMs                    float64 `json:"recv_ms"`
	ReceiptCompleteBodyReadMS float64 `json:"receipt_complete_body_read_ms,omitempty"`
	ReceiptCaptureSemantics   string  `json:"receipt_capture_semantics,omitempty"`
	Platform                  string  `json:"platform"`
	RunID                     string  `json:"run_id"`
	Seq                       int64   `json:"seq"`
	Dup                       bool    `json:"dup"` // this (run_id, seq) was observed before
}

// HasCompleteBodyReceipt reports whether this record was written with the versioned complete-body
// semantics. Legacy records return false and must stay interpreted as legacy.
func (r Record) HasCompleteBodyReceipt() bool {
	return r.ReceiptCaptureSemantics == receipt.CaptureSemantics && r.ReceiptCompleteBodyReadMS > 0
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

	// Incomplete-body diagnostics. These count requests that produced no receipt record because
	// the complete body could not be read; they are separate from overflow and unparsed identities.
	incompleteBody atomic.Int64
	bodyReadErrors atomic.Int64
	oversizedBody  atomic.Int64

	// now and extract are injectable for deterministic offline tests. They are immutable after New
	// in production and are only set before the server accepts requests in tests.
	now     func() time.Time
	extract func([]byte) ([][2]string, error)
}

func New(w io.Writer, buffer int) *Server {
	s := &Server{start: time.Now(), records: make(chan Record, buffer), done: make(chan struct{}), seen: map[string]struct{}{}, now: time.Now, extract: Extract}
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

// IncompleteBody counts requests whose complete body could not be read (read error, truncation or
// oversized body). No receipt record is emitted for them.
func (s *Server) IncompleteBody() int64 { return s.incompleteBody.Load() }

// BodyReadErrors counts incomplete-body requests caused by a read failure or a body shorter than
// its declared length.
func (s *Server) BodyReadErrors() int64 { return s.bodyReadErrors.Load() }

// OversizedBody counts incomplete-body requests rejected because the body exceeded MaxBodyBytes.
func (s *Server) OversizedBody() int64 { return s.oversizedBody.Load() }

func (s *Server) clock() time.Time {
	if s.now == nil {
		return time.Now()
	}
	return s.now()
}

func (s *Server) parse(body []byte) ([][2]string, error) {
	if s.extract == nil {
		return Extract(body)
	}
	return s.extract(body)
}

// readCompleteBody returns the full body only when it was read successfully within the bound. A
// reader error, a short/truncated body or a body larger than MaxBodyBytes returns an error and no
// bytes, so callers cannot fabricate a complete-body receipt from a partial read.
func readCompleteBody(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, MaxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > MaxBodyBytes {
		return nil, errBodyTooLarge
	}
	return body, nil
}

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
		fmt.Fprintf(w, "# HELP sink_incomplete_body_total Requests whose complete body could not be read (read error, truncation or oversized body); no receipt record was emitted.\n# TYPE sink_incomplete_body_total counter\nsink_incomplete_body_total %d\n", s.incompleteBody.Load())
		fmt.Fprintf(w, "# HELP sink_body_read_error_total Incomplete-body requests whose body read failed or ended before its declared length.\n# TYPE sink_body_read_error_total counter\nsink_body_read_error_total %d\n", s.bodyReadErrors.Load())
		fmt.Fprintf(w, "# HELP sink_oversized_body_total Incomplete-body requests rejected for exceeding the bounded maximum body size.\n# TYPE sink_oversized_body_total counter\nsink_oversized_body_total %d\n", s.oversizedBody.Load())
		return
	}
	now := s.clock()
	s.requests.Add(1)
	body, readErr := readCompleteBody(r.Body)
	if readErr != nil {
		s.incompleteBody.Add(1)
		if errors.Is(readErr, errBodyTooLarge) {
			s.oversizedBody.Add(1)
		} else {
			s.bodyReadErrors.Add(1)
		}
		s.bad.Add(1) // also preserves the legacy "request without any identity" health count
		w.WriteHeader(http.StatusOK)
		return
	}
	// The contract timestamp is taken here: after the full body read succeeded, before parsing.
	receiptAt := s.clock()
	w.WriteHeader(http.StatusOK) // answer first: the sink must not slow the platform's export path
	pairs, err := s.parse(body)
	if err != nil || len(pairs) == 0 {
		s.bad.Add(1)
		return
	}
	platform := strings.Trim(r.URL.Path, "/")
	recvMs := float64(now.UnixNano()) / 1e6
	receiptMs := float64(receiptAt.UnixNano()) / 1e6
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
		case s.records <- Record{RecvMs: recvMs, ReceiptCompleteBodyReadMS: receiptMs, ReceiptCaptureSemantics: receipt.CaptureSemantics, Platform: platform, RunID: p[0], Seq: seq, Dup: dup}:
		default:
			s.overflow.Add(1)
		}
	}
}
