// fixture-target serves internal/fixture: a controllable endpoint with switchable, seq-keyed faults for
// qualifying the load generator (docs/k6-feasibility.md, docs/generator.md). It exits non-zero if any record was
// lost to buffer overflow.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"iotbench/internal/fixture"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18080", "listen address")
	out := flag.String("out", "target-events.ndjson", "NDJSON record file")
	buf := flag.Int("buffer", 1<<20, "record buffer capacity (records)")
	var c fixture.Config
	flag.DurationVar(&c.Delay, "delay", 0, "fixed answer delay (per request: ?delay_ms=)")
	flag.Int64Var(&c.FailEvery, "fail-every", 0, "answer -fail-status for seq % n == 0")
	flag.IntVar(&c.FailStatus, "fail-status", 503, "status for -fail-every")
	flag.BoolVar(&c.StoreFailed, "store-failed", false, "still store messages answered with -fail-status")
	flag.Int64Var(&c.SlowEvery, "slow-every", 0, "store seq % n == 0 on receipt, answer after -slow-delay")
	flag.DurationVar(&c.SlowDelay, "slow-delay", 3*time.Second, "answer delay for -slow-every")
	flag.DurationVar(&c.RampTo, "ramp-to", 0, "extra answer delay reached after -ramp-over")
	flag.DurationVar(&c.RampOver, "ramp-over", 0, "ramp duration, from the first request")
	flag.BoolVar(&c.Mode207, "mode-207", false, "answer 207 with a one-item batch body (EdgeX style)")
	flag.Int64Var(&c.Fail207Every, "fail-207-every", 0, "in 207 mode, item statusCode 500 for seq % n == 0")
	flag.StringVar(&c.ForwardURL, "forward", "", "re-post every stored message to this URL (a sink)")
	flag.Parse()

	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	s := fixture.New(c, f, *buf)
	srv := &http.Server{Addr: *addr, Handler: s}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	log.Printf("fixture-target on %s (%+v), writing %s", *addr, c, *out)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	s.Close()
	_ = f.Close()
	log.Printf("received %d, overflow %d", s.Received(), s.Overflow())
	if s.Overflow() > 0 {
		os.Exit(3)
	}
}
