// sink serves internal/sink: it records every message a platform exports (outbound observation). It exits
// non-zero if any record was lost to buffer overflow.
//
//	sink -addr 0.0.0.0:18090 -out sink.ndjson
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

	"iotbench/internal/sink"
)

func main() {
	addr := flag.String("addr", "0.0.0.0:18090", "listen address")
	out := flag.String("out", "sink.ndjson", "NDJSON record file")
	buf := flag.Int("buffer", 1<<20, "record buffer capacity (records)")
	flag.Parse()

	f, err := os.OpenFile(*out, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Fatal(err)
	}
	s := sink.New(f, *buf)
	srv := &http.Server{Addr: *addr, Handler: s, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	log.Printf("sink on %s, appending to %s", *addr, *out)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	shutdownErr := srv.Shutdown(ctx)
	if shutdownErr != nil {
		log.Printf("HTTP shutdown incomplete: %v; forcing connection close", shutdownErr)
		_ = srv.Close()
	}
	s.Close()
	_ = f.Close()
	log.Printf("final counters received=%d duplicates=%d unparsed=%d overflow=%d incomplete_body=%d body_read_error=%d oversized_body=%d shutdown_clean=%t",
		s.Received(), s.Duplicates(), s.Unparsed(), s.Overflow(), s.IncompleteBody(), s.BodyReadErrors(), s.OversizedBody(), shutdownErr == nil)
	if shutdownErr != nil {
		os.Exit(4)
	}
	if s.Overflow() > 0 {
		os.Exit(3)
	}
}
