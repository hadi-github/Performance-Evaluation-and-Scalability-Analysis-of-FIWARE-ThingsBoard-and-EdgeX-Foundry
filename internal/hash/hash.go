// Package hash is the per-message randomness of the generator: every random decision for message seq is a pure
// function of (seed, seq), so it does not depend on which VU ran the iteration and can be reproduced offline.
// workloads/k6/lib/hash.js implements the same function bit for bit (checked by tests/integration).
package hash

// Salts separate independent decisions about the same seq.
const (
	SaltSelect      uint32 = 0x53454c31 // thinning: is tick seq sent?
	SaltTemperature uint32 = 0x54454d50
	SaltHumidity    uint32 = 0x48554d49
	SaltPressure    uint32 = 0x50524553
)

// Mix returns a 32-bit hash of (seed, seq, salt). For fixed seed and salt it is a bijection on seq (< 2^32):
// a multiply-xor of the key followed by the murmur3 32-bit finalizer.
func Mix(seed, seq, salt uint32) uint32 {
	x := ((seed ^ salt) * 0x9e3779b1) ^ seq
	x ^= x >> 16
	x *= 0x85ebca6b
	x ^= x >> 13
	x *= 0xc2b2ae35
	x ^= x >> 16
	return x
}

// Threshold is the selection threshold for send probability p: tick seq is sent iff Mix(seed, seq, SaltSelect) < t.
func Threshold(p float64) uint64 {
	if p >= 1 {
		return 1 << 32
	}
	return uint64(p * 4294967296)
}

// Selected reports whether tick seq is sent under thinning with threshold t.
func Selected(seed, seq uint32, t uint64) bool {
	return uint64(Mix(seed, seq, SaltSelect)) < t
}

// Values are the logical payload's three numeric fields for seq (same ranges as the reference JMeter plans).
func Values(seed, seq uint32) (temperature, humidity, pressure int) {
	return 15 + int(Mix(seed, seq, SaltTemperature)%31),
		30 + int(Mix(seed, seq, SaltHumidity)%51),
		950 + int(Mix(seed, seq, SaltPressure)%201)
}
