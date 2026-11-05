// Package receipt defines the versioned receiver-side receipt-capture semantics shared by the
// sink and the offline analysis. It is deliberately separate from the frozen protocol definition:
// adding this metadata does not change the protocol definition hash, the D44 profile policy, or any
// scientific metric definition.
//
// The contract covers only what the receiver can observe locally. A timestamp produced here is a
// shared-load-realtime wall-clock epoch millisecond value. It does not qualify a monotonic clock,
// prove cross-host clock alignment, or establish end-to-end platform delivery.
package receipt

const (
	// CaptureSemantics is the explicit versioned receiver contract. It states that the timestamp
	// is taken only after the complete request body has been read successfully and before the sink
	// parses or canonicalizes that body. A legacy record that omits this marker is never silently
	// reinterpreted under this contract.
	CaptureSemantics = "complete-body-read-before-parse-v1"

	// Endpoint names the measured endpoint: the receiver observed the complete response body.
	Endpoint = "response_complete_body_read"

	// SinkTimestampField is the sink record field that carries the complete-body receipt time.
	SinkTimestampField = "receipt_complete_body_read_ms"

	// SchemaVersion is the current protocol receipt-capture document schema. Schema 2 requires
	// CaptureSemantics; schema 1 is retained read-only as a legacy capture and cannot claim the new
	// primary delivery metrics.
	SchemaVersion = 2

	// LegacySchemaVersion is the retained pre-M1d capture schema. It remains readable, but its
	// records lack the complete-body-before-parse attestation.
	LegacySchemaVersion = 1

	// TimestampUnit documents the unit of receipt timestamps. Values are epoch milliseconds as
	// float64 with sub-millisecond resolution, in the same wall-clock domain as the legacy recv_ms.
	TimestampUnit = "epoch milliseconds (float64, sub-millisecond resolution)"

	// ClockDomain documents the local clock relationship. It is not a qualification claim.
	ClockDomain = "shared_load_realtime_wall_clock"
)
