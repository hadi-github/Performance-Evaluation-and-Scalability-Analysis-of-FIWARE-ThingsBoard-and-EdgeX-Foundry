package arrivals

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"math"
	"os"
	"sort"
	"strconv"
	"time"
)

// SendTimes reads k6's full JSON output (k6.json.gz of a -full-json run) and computes the send lag from k6's own
// sample times: dispatch = http_req_duration sample time (end of response) - http_req_duration, against the
// scheduled time carried in the sample metadata. Returns lags in ms.
func SendTimes(path string) ([]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	var lags []float64
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var l struct {
			Type   string `json:"type"`
			Metric string `json:"metric"`
			Data   struct {
				Time     time.Time         `json:"time"`
				Value    float64           `json:"value"`
				Tags     map[string]string `json:"tags"`
				Metadata map[string]string `json:"metadata"`
			} `json:"data"`
		}
		if json.Unmarshal(sc.Bytes(), &l) != nil || l.Type != "Point" || l.Metric != "http_req_duration" || l.Data.Tags["name"] != "send" {
			continue
		}
		sched, err := strconv.ParseFloat(l.Data.Metadata["sched"], 64)
		if err != nil {
			continue
		}
		dispatch := float64(l.Data.Time.UnixNano())/1e6 - l.Data.Value
		lags = append(lags, dispatch-sched)
	}
	return lags, sc.Err()
}

// Quantile of sorted values (nearest rank).
func Quantile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	i := int(math.Ceil(p*float64(len(sorted)))) - 1
	if i < 0 {
		i = 0
	}
	return sorted[i]
}

func SortedCopy(v []float64) []float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return s
}
