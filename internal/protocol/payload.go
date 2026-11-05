package protocol

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	benchhash "iotbench/internal/hash"
)

const PayloadEncodingID = "logical-json-ascii-v1"

const PayloadPaddingSalt uint32 = 0x50414431

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)
var integerTokenPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

// LogicalPayload field declaration order is normative for logical-json-ascii-v1.
type LogicalPayload struct {
	RunID           string  `json:"run_id"`
	Seq             uint32  `json:"seq"`
	DeviceID        string  `json:"device_id"`
	ScheduledSendMS float64 `json:"scheduled_send_ms"`
	Temperature     int     `json:"temperature"`
	Humidity        int     `json:"humidity"`
	Pressure        int     `json:"pressure"`
	Padding         string  `json:"padding"`
}

func (p LogicalPayload) Validate() error {
	if !idPattern.MatchString(p.RunID) || !idPattern.MatchString(p.DeviceID) {
		return errors.New("payload run_id/device_id must be bounded ASCII identifiers")
	}
	if math.IsNaN(p.ScheduledSendMS) || math.IsInf(p.ScheduledSendMS, 0) || p.ScheduledSendMS < 0 || p.ScheduledSendMS > 9007199254740991 {
		return errors.New("payload scheduled_send_ms outside safe finite range")
	}
	if !atMostThreeDecimalPlaces(strconv.FormatFloat(p.ScheduledSendMS, 'e', -1, 64)) {
		return errors.New("payload scheduled_send_ms must have at most three fractional decimal digits; encoder never rounds")
	}
	if p.Temperature < benchhash.TemperatureMin || p.Temperature > benchhash.TemperatureMax ||
		p.Humidity < benchhash.HumidityMin || p.Humidity > benchhash.HumidityMax ||
		p.Pressure < benchhash.PressureMin || p.Pressure > benchhash.PressureMax {
		return errors.New("payload numeric values outside contract ranges")
	}
	if len(p.Padding) > PayloadMaximumBytes || strings.Trim(p.Padding, "0123456789abcdef") != "" {
		return errors.New("payload padding must be bounded lowercase ASCII hex")
	}
	return nil
}

const PayloadMaximumBytes = 1 << 20

func CanonicalBytes(p LogicalPayload) ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if p.ScheduledSendMS == 0 {
		p.ScheduledSendMS = 0
	} // normalize negative zero in both Go and JS
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	if len(b) > PayloadMaximumBytes {
		return nil, errors.New("encoded payload exceeds contract maximum")
	}
	return b, nil
}

func atMostThreeDecimalPlaces(s string) bool {
	return decimalScaleOK(s)
}

func rawDecimalAtMostThreePlaces(raw json.RawMessage) bool {
	return decimalScaleOK(strings.TrimSpace(string(raw)))
}

// decimalParts computes scale without exponent arithmetic overflow. Zero is recognized
// from coefficient digits, independent of even absurd exponent spellings.
func decimalParts(s string) (zero bool, scale int, ok bool) {
	parts := strings.SplitN(strings.ToLower(s), "e", 2)
	if len(parts) > 2 {
		return false, 0, false
	}
	m := strings.TrimPrefix(parts[0], "-")
	fraction := 0
	if dot := strings.IndexByte(m, '.'); dot >= 0 {
		fraction = len(m) - dot - 1
		m = m[:dot] + m[dot+1:]
	}
	if m == "" {
		return false, 0, false
	}
	digits := strings.TrimLeft(m, "0")
	exponent := 0
	if len(parts) == 2 {
		e := strings.TrimPrefix(parts[1], "+")
		if e == "" || e == "-" {
			return false, 0, false
		}
		n, err := strconv.ParseInt(e, 10, 32)
		if err != nil || n > 10000 || n < -10000 {
			return false, 0, false
		}
		exponent = int(n)
	}
	if digits == "" {
		return true, 0, true
	}
	trailing := len(digits) - len(strings.TrimRight(digits, "0"))
	// Exponent magnitude is bounded above; compare scale algebraically instead of
	// constraining it by coefficient length. Scientific notation for large integers
	// (for example 1.7e+12) is valid and has a nonpositive effective scale.
	base := fraction - trailing
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	if exponent > 0 && base < minInt+exponent {
		return false, minInt, true
	}
	if exponent < 0 && base > maxInt+exponent {
		return false, maxInt, true
	}
	return false, base - exponent, true
}
func decimalScaleOK(s string) bool {
	zero, scale, ok := decimalParts(s)
	return ok && (zero || scale <= 3)
}
func decimalTokenRepresentable(s string, value float64) bool {
	zero, _, ok := decimalParts(s)
	if !ok {
		return false
	}
	if zero {
		return value == 0
	}
	return value != 0 && !math.IsInf(value, 0) && !math.IsNaN(value)
}
func rawIntegerToken(raw json.RawMessage) bool {
	return integerTokenPattern.Match(bytes.TrimSpace(raw))
}

// BuildPayload uses only the existing per-sequence numeric values supplied by the caller. Target size is explicit;
// no implicit default or numerical-field rounding is performed.
func BuildPayload(p LogicalPayload, seed uint32, targetBytes int) ([]byte, error) {
	if targetBytes < 0 || targetBytes > PayloadMaximumBytes {
		return nil, errors.New("payload target size outside explicit contract bounds")
	}
	p.Padding = ""
	base, err := CanonicalBytes(p)
	if err != nil {
		return nil, err
	}
	if targetBytes < len(base) || ((targetBytes-len(base))%2) != 0 {
		return nil, fmt.Errorf("target size %d cannot be represented by ASCII-hex padding; minimum is %d", targetBytes, len(base))
	}
	chars := targetBytes - len(base)
	var pad strings.Builder
	pad.Grow(chars)
	for block := uint32(0); pad.Len() < chars; block++ {
		v := benchhash.Mix(seed, p.Seq, PayloadPaddingSalt^block)
		var b [4]byte
		b[0] = byte(v >> 24)
		b[1] = byte(v >> 16)
		b[2] = byte(v >> 8)
		b[3] = byte(v)
		pad.WriteString(hex.EncodeToString(b[:]))
	}
	p.Padding = pad.String()[:chars]
	out, err := CanonicalBytes(p)
	if err != nil {
		return nil, err
	}
	if len(out) != targetBytes {
		return nil, errors.New("internal payload sizing mismatch")
	}
	return out, nil
}

// BuildPayloadOffset applies a proposed padding offset to the unpadded baseline; offset zero is the baseline.
func BuildPayloadOffset(p LogicalPayload, seed uint32, offsetBytes int) ([]byte, error) {
	if offsetBytes < 0 || offsetBytes > PayloadMaximumBytes {
		return nil, errors.New("payload offset outside explicit contract bounds")
	}
	p.Padding = ""
	base, err := CanonicalBytes(p)
	if err != nil {
		return nil, err
	}
	if len(base)+offsetBytes > PayloadMaximumBytes {
		return nil, errors.New("payload target exceeds contract maximum")
	}
	return BuildPayload(p, seed, len(base)+offsetBytes)
}

// MaximumPayloadOffset returns the largest even offset whose target (baseline + offset) remains
// within PayloadMaximumBytes. It is the same bound and parity rule BuildPayloadOffset enforces.
func MaximumPayloadOffset(baselineBytes int) (int, error) {
	if baselineBytes < 0 || baselineBytes > PayloadMaximumBytes {
		return 0, fmt.Errorf("payload baseline %d outside contract bounds [0,%d]", baselineBytes, PayloadMaximumBytes)
	}
	return (PayloadMaximumBytes - baselineBytes) &^ 1, nil
}

// payloadTimestampSamples cover the longest canonical scheduled_send_ms representations permitted by
// the logical contract (16 integer digits; and 13-16 integer digits with up to three decimals). The
// 18-byte form is covered at 14, 15 and 16 integer digits. Plan validation uses their maximum encoded
// length, never a separately maintained byte constant.
var payloadTimestampSamples = []float64{
	9007199254740991,
	1000000000000000.6, 100000000000000.12, 10000000000000.123,
	1700000000000.123, 999999999999.999,
}

// PayloadBaselineUpperBound returns the largest unpadded canonical baseline length for the supplied
// run_id, the actual longest device_id the plan can generate, seq <= maxSeq, and any contract-valid
// scheduled_send_ms. It delegates to CanonicalBytes, so plan validation and the encoder share field
// order, identifier bounds and timestamp formatting. Numeric fields use the contract maxima: a
// four-digit pressure (1150) is one byte longer than the three-digit minimum (950) and must not be
// undercounted by the plan bound.
func PayloadBaselineUpperBound(runID, maxDeviceID string, maxSeq uint32) (int, error) {
	upper := 0
	for _, timestamp := range payloadTimestampSamples {
		p := LogicalPayload{RunID: runID, Seq: maxSeq, DeviceID: maxDeviceID,
			ScheduledSendMS: timestamp, Temperature: benchhash.TemperatureMax,
			Humidity: benchhash.HumidityMax, Pressure: benchhash.PressureMax}
		b, err := CanonicalBytes(p)
		if err != nil {
			return 0, fmt.Errorf("payload baseline for run_id/device_id: %w", err)
		}
		if len(b) > upper {
			upper = len(b)
		}
	}
	return upper, nil
}

// ValidateStored checks bytes supplied from an actual store/export, not a locally reconstructed surrogate.
func ValidateStored(expected LogicalPayload, stored []byte) error {
	if err := expected.Validate(); err != nil {
		return err
	}
	if !utf8.Valid(stored) {
		return errors.New("stored payload JSON is not valid UTF-8")
	}
	if err := rejectDuplicateJSONKeys(stored); err != nil {
		return fmt.Errorf("stored payload JSON invalid: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(stored, &fields); err != nil {
		return fmt.Errorf("stored payload JSON invalid: %w", err)
	}
	keys := []string{"run_id", "seq", "device_id", "scheduled_send_ms", "temperature", "humidity", "pressure", "padding"}
	if len(fields) != len(keys) {
		return errors.New("stored payload fields are incomplete or unknown")
	}
	for _, key := range keys {
		raw, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("stored payload field %q is missing or null", key)
		}
	}
	for _, key := range []string{"seq", "temperature", "humidity", "pressure"} {
		if !rawIntegerToken(fields[key]) {
			return fmt.Errorf("stored payload field %q must be a canonical integer JSON token", key)
		}
	}
	if !rawDecimalAtMostThreePlaces(fields["scheduled_send_ms"]) {
		return errors.New("stored scheduled_send_ms has more than three effective decimal places")
	}
	dec := json.NewDecoder(bytes.NewReader(stored))
	dec.DisallowUnknownFields()
	var got LogicalPayload
	if err := dec.Decode(&got); err != nil {
		return fmt.Errorf("stored payload JSON invalid: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("stored payload has trailing data")
	}
	if err := got.Validate(); err != nil {
		return fmt.Errorf("stored payload fields invalid: %w", err)
	}
	if !decimalTokenRepresentable(string(bytes.TrimSpace(fields["scheduled_send_ms"])), got.ScheduledSendMS) {
		return errors.New("stored timestamp is nonzero but underflowed or is not exactly representable")
	}
	canonicalValue := got.ScheduledSendMS
	if canonicalValue == 0 {
		canonicalValue = 0
	}
	canonicalTimestamp, err := json.Marshal(canonicalValue)
	if err != nil {
		return fmt.Errorf("encode stored timestamp: %w", err)
	}
	rawTimestamp := bytes.TrimSpace(fields["scheduled_send_ms"])
	zeroToken, _, tokenOK := decimalParts(string(rawTimestamp))
	if !tokenOK || (zeroToken && got.ScheduledSendMS != 0) || (!zeroToken && got.ScheduledSendMS == 0) || (!bytes.Equal(rawTimestamp, canonicalTimestamp) && !(zeroToken && got.ScheduledSendMS == 0)) {
		return errors.New("stored timestamp is not in canonical exact decimal form")
	}
	wantBytes, err := CanonicalBytes(expected)
	if err != nil {
		return fmt.Errorf("encode expected payload: %w", err)
	}
	gotBytes, err := CanonicalBytes(got)
	if err != nil {
		return fmt.Errorf("encode stored payload: %w", err)
	}
	if !bytes.Equal(wantBytes, gotBytes) {
		return errors.New("stored payload fields differ from expected logical payload")
	}
	return nil
}

// FormatScheduledMillis exports a numeric timestamp without rounding. Kept here for explicit golden vectors.
func FormatScheduledMillis(v float64) (string, error) {
	p := LogicalPayload{RunID: "r", DeviceID: "d", ScheduledSendMS: v, Temperature: 15, Humidity: 30, Pressure: 950}
	if err := p.Validate(); err != nil {
		return "", err
	}
	if v == 0 {
		v = 0
	}
	return strconv.FormatFloat(v, 'f', -1, 64), nil
}
