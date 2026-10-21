// Per-message randomness: a pure function of (seed, seq), bit-identical to internal/hash/hash.go.
// All arithmetic is unsigned 32-bit (Math.imul + >>> 0); seq must be < 2^32 (benchctl enforces it).
export const SALT_SELECT = 0x53454c31;
export const SALT_TEMPERATURE = 0x54454d50;
export const SALT_HUMIDITY = 0x48554d49;
export const SALT_PRESSURE = 0x50524553;

export function mix(seed, seq, salt) {
  let x = (Math.imul((seed ^ salt) >>> 0, 0x9e3779b1) ^ (seq >>> 0)) >>> 0;
  x = (x ^ (x >>> 16)) >>> 0;
  x = Math.imul(x, 0x85ebca6b) >>> 0;
  x = (x ^ (x >>> 13)) >>> 0;
  x = Math.imul(x, 0xc2b2ae35) >>> 0;
  return (x ^ (x >>> 16)) >>> 0;
}

// Selection threshold for send probability p (tick seq is sent iff mix(seed, seq, SALT_SELECT) < threshold).
export function threshold(p) {
  return p >= 1 ? 4294967296 : Math.floor(p * 4294967296);
}

export function selected(seed, seq, t) {
  return mix(seed, seq, SALT_SELECT) < t;
}

export function values(seed, seq) {
  return {
    temperature: 15 + (mix(seed, seq, SALT_TEMPERATURE) % 31),
    humidity: 30 + (mix(seed, seq, SALT_HUMIDITY) % 51),
    pressure: 950 + (mix(seed, seq, SALT_PRESSURE) % 201),
  };
}
