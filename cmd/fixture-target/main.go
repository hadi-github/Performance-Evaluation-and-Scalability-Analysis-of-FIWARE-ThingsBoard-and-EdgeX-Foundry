// fixture-target is a controllable HTTP endpoint for qualifying the load generator (docs/k6-feasibility.md).
//
// Every POST is answered after a configurable delay (flag -delay, or per request with ?delay_ms=), and one NDJSON
// record is written per request: receipt time (wall-clock epoch ns as a string, plus monotonic ns since start) and
// every X-Bench-* request header. Records go through a bounded buffer; if it ever overflows, the overflow is counted,
// reported on /stats and at exit, and the process exits non-zero, so a lossy capture can never pass silently.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

type record struct {
	RecvWallNs string            `json:"recv_wall_ns"`
	RecvMonoNs int64             `json:"recv_mono_ns"`
	DoneMonoNs int64             `json:"done_mono_ns"`
	DelayMs    float64           `json:"delay_ms"`
	Status     int               `json:"status"`
	Headers    map[string]string `json:"h"`
}

func main() {
	addr := flag.String("addr", "127.0.0.1:18080", "listen address")
	delay := flag.Duration("delay", 0, "response delay (overridable per request with ?delay_ms=)")
	out := flag.String("out", "target-events.ndjson", "NDJSON output file")
	buf := flag.Int("buffer", 1<<20, "record buffer capacity (records)")
	flag.Parse()

	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	records := make(chan record, *buf)
	var received, overflow atomic.Int64
	start := time.Now()

	writerDone := make(chan struct{})
	go func() {
		w := bufio.NewWriterSize(f, 1<<20)
		enc := json.NewEncoder(w)
		for r := range records {
			if err := enc.Encode(r); err != nil {
				log.Fatal(err)
			}
		}
		if err := w.Flush(); err != nil {
			log.Fatal(err)
		}
		f.Close()
		close(writerDone)
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/stats", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "{\"received\":%d,\"overflow\":%d}\n", received.Load(), overflow.Load())
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		recv := time.Now()
		received.Add(1)
		d := *delay
		if v := r.URL.Query().Get("delay_ms"); v != "" {
			if ms, err := strconv.ParseFloat(v, 64); err == nil {
				d = time.Duration(ms * float64(time.Millisecond))
			}
		}
		h := map[string]string{}
		for k, v := range r.Header {
			if lk := strings.ToLower(k); strings.HasPrefix(lk, "x-bench-") {
				h[strings.TrimPrefix(lk, "x-bench-")] = v[0]
			}
		}
		if d > 0 {
			time.Sleep(d)
		}
		w.WriteHeader(http.StatusOK)
		rec := record{
			RecvWallNs: strconv.FormatInt(recv.UnixNano(), 10),
			RecvMonoNs: int64(recv.Sub(start)),
			DoneMonoNs: int64(time.Since(start)),
			DelayMs:    float64(d) / float64(time.Millisecond),
			Status:     http.StatusOK,
			Headers:    h,
		}
		select {
		case records <- rec:
		default:
			overflow.Add(1)
		}
	})

	srv := &http.Server{Addr: *addr, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	log.Printf("fixture-target on %s, delay %s, writing %s", *addr, *delay, *out)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	close(records)
	<-writerDone
	log.Printf("received %d, overflow %d", received.Load(), overflow.Load())
	if overflow.Load() > 0 {
		os.Exit(3)
	}
}
