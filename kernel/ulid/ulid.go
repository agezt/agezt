// SPDX-License-Identifier: MIT

// Package ulid implements ULID generation per DECISIONS B2 — 128-bit
// lexicographically sortable identifiers with a 48-bit millisecond timestamp
// prefix and 80-bit cryptographic randomness, encoded as 26 Crockford base32
// characters.
//
// Spec reference: https://github.com/ulid/spec
//
// Stdlib only. The kernel owns ID assignment (DECISIONS B2: "Kernel assigns
// IDs; plugins never do"); this package is the kernel's source.
package ulid

import (
	"crypto/rand"
	"fmt"
	"io"
	"sync"
	"time"
)

// Size is the byte length of a binary ULID.
const Size = 16

// EncodedSize is the character length of an encoded ULID.
const EncodedSize = 26

// crockfordAlphabet is Crockford base32 (no I, L, O, U).
const crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// Generator produces ULIDs. It is safe for concurrent use. A zero-value
// Generator uses crypto/rand and time.Now.
type Generator struct {
	mu      sync.Mutex
	nowFunc func() time.Time
	randSrc io.Reader
}

// Default is the package-wide generator, backed by time.Now and crypto/rand.
var Default = &Generator{
	nowFunc: time.Now,
	randSrc: rand.Reader,
}

// New returns a freshly minted ULID using the default generator.
func New() string {
	return Default.New()
}

// New returns a freshly minted ULID.
func (g *Generator) New() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	nowFn := g.nowFunc
	if nowFn == nil {
		nowFn = time.Now
	}
	randSrc := g.randSrc
	if randSrc == nil {
		randSrc = rand.Reader
	}

	var b [Size]byte
	ms := uint64(nowFn().UnixMilli())
	// big-endian 48-bit timestamp into first 6 bytes
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	// 80 bits of randomness
	if _, err := io.ReadFull(randSrc, b[6:]); err != nil {
		// crypto/rand failing is fatal; the kernel cannot proceed without
		// unique IDs. Panic so the supervisor sees it.
		panic(fmt.Errorf("ulid: rand: %w", err))
	}
	return encode(b)
}

// encode converts 16 bytes to 26 Crockford base32 characters. The encoding
// treats the 128 bits as one big integer, padded with two leading zero bits
// at the top to reach 130 bits = 26 × 5.
func encode(b [Size]byte) string {
	var out [EncodedSize]byte

	// Bytes 0-5 (48-bit timestamp) → chars 0-9.
	out[0] = crockfordAlphabet[(b[0]&224)>>5]
	out[1] = crockfordAlphabet[b[0]&31]
	out[2] = crockfordAlphabet[(b[1]&248)>>3]
	out[3] = crockfordAlphabet[((b[1]&7)<<2)|((b[2]&192)>>6)]
	out[4] = crockfordAlphabet[(b[2]&62)>>1]
	out[5] = crockfordAlphabet[((b[2]&1)<<4)|((b[3]&240)>>4)]
	out[6] = crockfordAlphabet[((b[3]&15)<<1)|((b[4]&128)>>7)]
	out[7] = crockfordAlphabet[(b[4]&124)>>2]
	out[8] = crockfordAlphabet[((b[4]&3)<<3)|((b[5]&224)>>5)]
	out[9] = crockfordAlphabet[b[5]&31]

	// Bytes 6-15 (80-bit random) → chars 10-25.
	out[10] = crockfordAlphabet[(b[6]&248)>>3]
	out[11] = crockfordAlphabet[((b[6]&7)<<2)|((b[7]&192)>>6)]
	out[12] = crockfordAlphabet[(b[7]&62)>>1]
	out[13] = crockfordAlphabet[((b[7]&1)<<4)|((b[8]&240)>>4)]
	out[14] = crockfordAlphabet[((b[8]&15)<<1)|((b[9]&128)>>7)]
	out[15] = crockfordAlphabet[(b[9]&124)>>2]
	out[16] = crockfordAlphabet[((b[9]&3)<<3)|((b[10]&224)>>5)]
	out[17] = crockfordAlphabet[b[10]&31]
	out[18] = crockfordAlphabet[(b[11]&248)>>3]
	out[19] = crockfordAlphabet[((b[11]&7)<<2)|((b[12]&192)>>6)]
	out[20] = crockfordAlphabet[(b[12]&62)>>1]
	out[21] = crockfordAlphabet[((b[12]&1)<<4)|((b[13]&240)>>4)]
	out[22] = crockfordAlphabet[((b[13]&15)<<1)|((b[14]&128)>>7)]
	out[23] = crockfordAlphabet[(b[14]&124)>>2]
	out[24] = crockfordAlphabet[((b[14]&3)<<3)|((b[15]&224)>>5)]
	out[25] = crockfordAlphabet[b[15]&31]

	return string(out[:])
}
