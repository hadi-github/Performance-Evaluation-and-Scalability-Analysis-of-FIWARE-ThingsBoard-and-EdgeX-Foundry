// feasibility-analyze summarises one k6 feasibility case (docs/methodology/k6-feasibility.md): it joins the fixture target's
// records with k6's JSON output and reports what the iterations' native identifiers say about the planned schedule.
//
//	feasibility-analyze -case results/feasibility/A0-baseline -rate 200 [-probe car|slot]
package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

type targetRec struct {
	RecvWallNs string            `json:"recv_wall_ns"`
	Headers    map[string]string `json:"h"`
}

type k6Line struct {
	Type   string `json:"type"`
	Metric string `json:"metric"`
	Data   struct {
		Time  time.Time `json:"time"`
		Value float64   `json:"value"`
	} `json:"data"`
}

func pct(v []float64, p float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	i := int(math.Ceil(p/100*float64(len(s)))) - 1
	if i < 0 {
		i = 0
	}
	return s[i]
}

func dist(name string, v []float64) {
	fmt.Printf("  %-44s n=%-6d p50=%9.2f p99=%9.2f max=%9.2f ms\n", name, len(v), pct(v, 50), pct(v, 99), pct(v, 100))
}

func atoi(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n }

func main() {
	dir := flag.String("case", "", "case directory")
	rate := flag.Float64("rate", 0, "planned arrival rate (req/s)")
	probe := flag.String("probe", "car", "car: constant-arrival-rate probe; slot: slot-schedule probe")
	planned := flag.Int64("planned", 0, "number of planned arrivals (ids 0..planned-1); reports the ones never received")
	flag.Parse()
	if *dir == "" || *rate <= 0 {
		flag.Usage()
		os.Exit(2)
	}

	// ---- target records
	f, err := os.Open(filepath.Join(*dir, "target.ndjson"))
	if err != nil {
		log.Fatal(err)
	}
	var recs []targetRec
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var r targetRec
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			log.Fatal(err)
		}
		recs = append(recs, r)
	}
	f.Close()

	// ---- k6 JSON output: iterations, dropped iterations
	g, err := os.Open(filepath.Join(*dir, "k6.json.gz"))
	if err != nil {
		log.Fatal(err)
	}
	zr, err := gzip.NewReader(g)
	if err != nil {
		log.Fatal(err)
	}
	var iterations, dropped float64
	var droppedTimes []time.Time
	ks := bufio.NewScanner(zr)
	ks.Buffer(make([]byte, 1<<20), 1<<20)
	for ks.Scan() {
		var l k6Line
		if json.Unmarshal(ks.Bytes(), &l) != nil || l.Type != "Point" {
			continue
		}
		switch l.Metric {
		case "iterations":
			iterations += l.Data.Value
		case "dropped_iterations":
			dropped += l.Data.Value
			droppedTimes = append(droppedTimes, l.Data.Time)
		}
	}
	g.Close()

	period := 1000.0 / *rate // ms
	fmt.Printf("case %s (planned %.0f req/s, period %.3f ms)\n", filepath.Base(*dir), *rate, period)
	fmt.Printf("  requests received by target: %d   k6 iterations: %.0f   k6 dropped_iterations: %.0f\n",
		len(recs), iterations, dropped)
	if len(recs) == 0 {
		return
	}

	key := "iter"
	if *probe == "slot" {
		key = "slot"
	}
	// ---- identifier coverage: is the index gap-free? does it hit planned slots?
	ids := map[int64]int{}
	var minID, maxID int64 = math.MaxInt64, -1
	for _, r := range recs {
		id := atoi(r.Headers[key])
		ids[id]++
		minID = min(minID, id)
		maxID = max(maxID, id)
	}
	dups := 0
	for _, c := range ids {
		if c > 1 {
			dups += c - 1
		}
	}
	gaps := int(maxID-minID+1) - len(ids)
	fmt.Printf("  %s: min=%d max=%d unique=%d gaps=%d duplicates=%d\n", key, minID, maxID, len(ids), gaps, dups)
	if *planned > 0 {
		missing := int64(0)
		for i := int64(0); i < *planned; i++ {
			if ids[i] == 0 {
				missing++
			}
		}
		fmt.Printf("  planned %d: received %d, never received %d, outside the plan %d\n",
			*planned, int64(len(ids))-outside(ids, *planned), missing, outside(ids, *planned))
	}

	// ---- times relative to the scenario/schedule start (ms)
	t0 := atoi(recs[0].Headers["scenario-start-ms"])
	if *probe == "slot" {
		t0 = atoi(recs[0].Headers["t0-ms"])
	}
	var startLag, preLag, recvLag, dispatchToRecv []float64
	perSecond := map[int64]int{}
	for _, r := range recs {
		id := float64(atoi(r.Headers[key]))
		planned := float64(t0) + id*period // planned time implied by the identifier
		if *probe == "slot" {
			planned = float64(atoi(r.Headers["sched-ms"]))
		}
		start := float64(atoi(r.Headers["start-ms"]))
		pre := float64(atoi(r.Headers["pre-ms"]))
		recv := float64(atoi(r.RecvWallNs)) / 1e6
		startLag = append(startLag, start-planned)
		preLag = append(preLag, pre-planned)
		recvLag = append(recvLag, recv-planned)
		dispatchToRecv = append(dispatchToRecv, recv-pre)
		perSecond[int64((recv-float64(t0))/1000)]++
	}
	fmt.Printf("  lag vs the planned time implied by %q:\n", key)
	dist("iteration start - planned", startLag)
	dist("pre-dispatch Date.now() - planned", preLag)
	dist("target receipt - planned", recvLag)
	dist("target receipt - pre-dispatch (cross-check)", dispatchToRecv)

	secs := make([]int64, 0, len(perSecond))
	for s := range perSecond {
		secs = append(secs, s)
	}
	sort.Slice(secs, func(i, j int) bool { return secs[i] < secs[j] })
	fmt.Printf("  arrivals at target per second:")
	for _, s := range secs {
		fmt.Printf(" %d", perSecond[s])
	}
	fmt.Println()
	if len(droppedTimes) > 0 {
		fmt.Printf("  dropped_iterations samples: %d (first %s, last %s)\n", len(droppedTimes),
			droppedTimes[0].Format("15:04:05.000"), droppedTimes[len(droppedTimes)-1].Format("15:04:05.000"))
	}
}

func outside(ids map[int64]int, planned int64) int64 {
	n := int64(0)
	for id := range ids {
		if id < 0 || id >= planned {
			n++
		}
	}
	return n
}
