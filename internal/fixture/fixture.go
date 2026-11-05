// Package fixture is a controllable HTTP endpoint for qualifying the load generator: it answers immediately by
// default and has switchable, seq-keyed faults. Every request it receives produces one record (what arrived, what was
// answered, whether it was "stored"); records go through a bounded buffer whose overflow is counted and reported, so
// a lossy capture can never pass silently.
//
// Faults (seq = the "seq" field of a JSON body; requests without one count as seq -1 and get no seq-keyed fault):
//
//	FailEvery/FailStatus   seq % n == 0 -> answer FailStatus (e.g. 503); stored only if StoreFailed
//	SlowEvery/SlowDelay    seq % n == 0 -> stored on receipt, answered after SlowDelay (beyond the client timeout)
//	RampTo/RampOver        every answer delayed by RampTo x min(1, t/RampOver), t = time since the first request
//	Delay                  fixed delay for every answer (overridable per request with ?delay_ms=)
//	Mode207/Fail207Every   answer 207 with a one-item JSON batch body; item statusCode 500 if seq % n == 0, else 201
//	ForwardURL             re-post every stored message's body to this URL (a sink), like a platform's outbound
//	                       path; done by a bounded worker pool, and every failed or dropped forward is counted
package fixture

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct {
	Delay        time.Duration
	FailEvery    int64
	FailStatus   int
	StoreFailed  bool
	SlowEvery    int64
	SlowDelay    time.Duration
	RampTo       time.Duration
	RampOver     time.Duration
	Mode207      bool
	Fail207Every int64
	ForwardURL   string
	// DropSeqs are answered 200 but never stored: acknowledged-but-unavailable-at-history (mixed with an
	// equivalent duplicate, stored counts stay equal while identities differ).
	DropSeqs []int64
	// MalformedEvery strips the run_id from every n-th stored record: the row exists but its identity
	// does not, so it must remain unaccountable instead of silently counting as stored.
	MalformedEvery int64
}

// Record is one received request. Times are epoch milliseconds (float, sub-ms resolution) and monotonic ns.
type Record struct {
	RecvMs     float64 `json:"recv_ms"`
	RecvWallNs string  `json:"recv_wall_ns"` // lossless epoch ns (string)
	RecvMonoNs int64   `json:"recv_mono_ns"`
	RunID      string  `json:"run_id,omitempty"`
	Seq        int64   `json:"seq"`
	Device     string  `json:"device_id,omitempty"`
	SchedMs    float64 `json:"scheduled_send_ms,omitempty"`
	// Padding is the actual adapter-supplied padding text, captured when present. It is omitted for
	// legacy bodies, so existing records remain unchanged and are never relabelled as padded.
	Padding string            `json:"padding,omitempty"`
	Status  int               `json:"status"`
	Stored  bool              `json:"stored"`
	DelayMs float64           `json:"delay_ms"`
	Headers map[string]string `json:"h,omitempty"`
}

type body struct {
	RunID   string   `json:"run_id"`
	Seq     *int64   `json:"seq"`
	Device  string   `json:"device_id"`
	SchedMs *float64 `json:"scheduled_send_ms"`
	Padding string   `json:"padding"`
}

type Server struct {
	cfg      Config
	start    time.Time
	first    atomic.Int64 // mono ns of the first request (+1), for the ramp
	received atomic.Int64
	overflow atomic.Int64
	records  chan Record
	done     chan struct{}
	closeMu  sync.Once
	forward  chan []byte
	fwdFail  atomic.Int64 // forwards that failed or were dropped (queue full)
	fwdOK    atomic.Int64
}

// New starts the record writer on w (NDJSON). Call Close after the HTTP server has stopped.
func New(cfg Config, w io.Writer, buffer int) *Server {
	s := &Server{cfg: cfg, start: time.Now(), records: make(chan Record, buffer), done: make(chan struct{})}
	if cfg.ForwardURL != "" {
		s.forward = make(chan []byte, 100000)
		client := &http.Client{Timeout: 10 * time.Second}
		for i := 0; i < 64; i++ {
			go func() {
				for b := range s.forward {
					res, err := client.Post(cfg.ForwardURL, "application/json", bytes.NewReader(b))
					if err != nil || res.StatusCode != http.StatusOK {
						s.fwdFail.Add(1)
					} else {
						s.fwdOK.Add(1)
					}
					if res != nil {
						_, _ = io.Copy(io.Discard, res.Body)
						res.Body.Close()
					}
				}
			}()
		}
	}
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

func (s *Server) Received() int64 { return s.received.Load() }
func (s *Server) Overflow() int64 { return s.overflow.Load() }

// Close flushes the records. It must be called after no handler can run any more.
func (s *Server) Close() {
	s.closeMu.Do(func() { close(s.records) })
	<-s.done
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/stats" {
		fmt.Fprintf(w, "{\"received\":%d,\"overflow\":%d,\"forwarded\":%d,\"forward_failed\":%d}\n",
			s.received.Load(), s.overflow.Load(), s.fwdOK.Load(), s.fwdFail.Load())
		return
	}
	recv := time.Now()
	mono := int64(recv.Sub(s.start))
	s.first.CompareAndSwap(0, mono+1)
	s.received.Add(1)

	rec := Record{RecvMs: float64(recv.UnixNano()) / 1e6, RecvWallNs: strconv.FormatInt(recv.UnixNano(), 10),
		RecvMonoNs: mono, Seq: -1}
	// Bound the capture at 2 MiB: the canonical payload maximum is 1 MiB, so a valid logical body
	// plus JSON framing fits. The fixture never parses a silently truncated valid payload.
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	var b body
	if len(raw) > 0 && raw[0] == '{' && json.Unmarshal(raw, &b) == nil {
		rec.RunID, rec.Device, rec.Padding = b.RunID, b.Device, b.Padding
		if b.Seq != nil {
			rec.Seq = *b.Seq
		}
		if b.SchedMs != nil {
			rec.SchedMs = *b.SchedMs
		}
	}
	for k, v := range r.Header { // feasibility probes send their fields as X-Bench-* headers
		if lk := strings.ToLower(k); strings.HasPrefix(lk, "x-bench-") {
			if rec.Headers == nil {
				rec.Headers = map[string]string{}
			}
			rec.Headers[strings.TrimPrefix(lk, "x-bench-")] = v[0]
		}
	}

	c := s.cfg
	delay := c.Delay
	if v := r.URL.Query().Get("delay_ms"); v != "" {
		if ms, err := strconv.ParseFloat(v, 64); err == nil {
			delay = time.Duration(ms * float64(time.Millisecond))
		}
	}
	if c.RampTo > 0 && c.RampOver > 0 {
		f := float64(mono-(s.first.Load()-1)) / float64(c.RampOver)
		delay += time.Duration(float64(c.RampTo) * min(1, f))
	}
	keyed := func(n int64) bool { return n > 0 && rec.Seq >= 0 && rec.Seq%n == 0 }
	dropped := map[int64]struct{}{}
	for _, q := range s.cfg.DropSeqs {
		dropped[q] = struct{}{}
	}

	status, stored := http.StatusOK, true
	switch {
	case keyed(c.FailEvery):
		status, stored = c.FailStatus, c.StoreFailed
	case keyed(c.SlowEvery):
		delay = c.SlowDelay
	}
	// Deliberate injection AFTER the status path: the answer is a normal 200, the message just never
	// reaches the history point (acknowledged but absent), and every n-th stored row loses its identity.
	if _, missing := dropped[rec.Seq]; missing {
		stored = false
	}
	if keyed(c.MalformedEvery) {
		rec.RunID = ""
	}
	rec.Status, rec.Stored, rec.DelayMs = status, stored, float64(delay)/float64(time.Millisecond)
	// Stored on receipt (before the answer), so a request the client gave up on is still persisted.
	s.emit(rec)
	if stored && s.forward != nil && rec.Seq >= 0 {
		select {
		case s.forward <- raw:
		default:
			s.fwdFail.Add(1)
		}
	}

	if delay > 0 {
		time.Sleep(delay)
	}
	if c.Mode207 && status == http.StatusOK {
		item := 201
		if keyed(c.Fail207Every) {
			item = 500
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprintf(w, `[{"apiVersion":"v3","statusCode":%d}]`, item)
		return
	}
	w.WriteHeader(status)
}

func (s *Server) emit(r Record) {
	select {
	case s.records <- r:
	default:
		s.overflow.Add(1)
	}
}

// ReadRecords loads an NDJSON record file.
func ReadRecords(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var r Record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, r)
	}
	return out, sc.Err()
}
