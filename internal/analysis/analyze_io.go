package analysis

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
)

type reader struct {
	result *Result
	base   string
}

func (r reader) read(role, path string) ([]byte, error) {
	if path != "" && !filepath.IsAbs(path) {
		path = filepath.Join(r.base, path)
	}
	b, err := os.ReadFile(path)
	in := Input{Role: role, Path: path, Status: Available, Bytes: len(b)}
	if err != nil {
		in.Status = Unavailable
		in.Error = err.Error()
	} else {
		h := sha256.Sum256(b)
		in.SHA256 = hex.EncodeToString(h[:])
	}
	r.result.Inputs = append(r.result.Inputs, in)
	return b, err
}

func decode(b []byte, v any, strict bool) error {
	d := json.NewDecoder(bytes.NewReader(b))
	if strict {
		d.DisallowUnknownFields()
	}
	if err := d.Decode(v); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("trailing JSON or corrupt input")
	}
	return nil
}

func rows[T any](b []byte) ([]T, error) {
	var out []T
	for i, line := range bytes.Split(b, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var v T
		if err := decode(line, &v, false); err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		out = append(out, v)
	}
	return out, nil
}

func finite(v float64) bool  { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func integer(v float64) bool { return finite(v) && v >= 0 && math.Trunc(v) == v }

func absolute(path string) string {
	p, e := filepath.Abs(path)
	if e != nil {
		return path
	}
	return p
}
