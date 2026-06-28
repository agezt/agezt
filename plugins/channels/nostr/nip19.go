// SPDX-License-Identifier: MIT

package nostr

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// NIP-19 bech32 entity decoding (npub / nsec → 32-byte hex). Only the simple
// 32-byte forms are supported (npub, nsec); TLV forms (nprofile, nevent) are not
// needed here. bech32 is implemented inline to avoid a dependency.

const bech32Charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

// DecodePubkey accepts a hex pubkey or an npub and returns the 32-byte x-only
// pubkey as hex. Exported so the daemon can normalize an author allowlist that
// may mix hex and npub forms.
func DecodePubkey(s string) (string, error) { return decodeNostrKey(s, "npub") }

// decodeNostrKey accepts a 64-char hex key or a bech32 npub/nsec with the given
// expected hrp ("npub" or "nsec") and returns the 32-byte value as hex.
func decodeNostrKey(s, wantHRP string) (string, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, wantHRP+"1") {
		hrp, data, err := bech32Decode(s)
		if err != nil {
			return "", err
		}
		if hrp != wantHRP {
			return "", fmt.Errorf("nostr: expected %s, got %s", wantHRP, hrp)
		}
		if len(data) != 32 {
			return "", fmt.Errorf("nostr: %s decodes to %d bytes, want 32", wantHRP, len(data))
		}
		return hex.EncodeToString(data), nil
	}
	// Assume hex; validate it's 32 bytes.
	raw, err := hex.DecodeString(s)
	if err != nil || len(raw) != 32 {
		return "", fmt.Errorf("nostr: key must be 64-char hex or %s1…", wantHRP)
	}
	return hex.EncodeToString(raw), nil
}

// bech32Decode decodes a bech32 string into its human-readable part and the
// 8-bit data payload (checksum verified, 5-bit groups regrouped to bytes).
func bech32Decode(s string) (string, []byte, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	pos := strings.LastIndexByte(s, '1')
	if pos < 1 || pos+7 > len(s) {
		return "", nil, fmt.Errorf("bech32: malformed string")
	}
	hrp := s[:pos]
	var values []byte
	for _, c := range s[pos+1:] {
		idx := strings.IndexRune(bech32Charset, c)
		if idx < 0 {
			return "", nil, fmt.Errorf("bech32: invalid character %q", c)
		}
		values = append(values, byte(idx))
	}
	if !bech32VerifyChecksum(hrp, values) {
		return "", nil, fmt.Errorf("bech32: bad checksum")
	}
	data, err := convertBits(values[:len(values)-6], 5, 8, false)
	if err != nil {
		return "", nil, err
	}
	return hrp, data, nil
}

func bech32Polymod(values []byte) uint32 {
	gen := []uint32{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3}
	chk := uint32(1)
	for _, v := range values {
		b := chk >> 25
		chk = (chk&0x1ffffff)<<5 ^ uint32(v)
		for i := 0; i < 5; i++ {
			if (b>>uint(i))&1 == 1 {
				chk ^= gen[i]
			}
		}
	}
	return chk
}

func bech32HRPExpand(hrp string) []byte {
	out := make([]byte, 0, len(hrp)*2+1)
	for _, c := range hrp {
		out = append(out, byte(c)>>5)
	}
	out = append(out, 0)
	for _, c := range hrp {
		out = append(out, byte(c)&31)
	}
	return out
}

func bech32VerifyChecksum(hrp string, data []byte) bool {
	return bech32Polymod(append(bech32HRPExpand(hrp), data...)) == 1
}

// convertBits regroups a byte slice from `from`-bit groups to `to`-bit groups.
func convertBits(data []byte, from, to uint, pad bool) ([]byte, error) {
	var acc uint32
	var bits uint
	var out []byte
	maxv := uint32(1)<<to - 1
	for _, b := range data {
		if uint32(b)>>from != 0 {
			return nil, fmt.Errorf("bech32: invalid data range")
		}
		acc = acc<<from | uint32(b)
		bits += from
		for bits >= to {
			bits -= to
			out = append(out, byte(acc>>bits&maxv))
		}
	}
	if pad {
		if bits > 0 {
			out = append(out, byte(acc<<(to-bits)&maxv))
		}
	} else if bits >= from || byte(acc<<(to-bits)&maxv) != 0 {
		return nil, fmt.Errorf("bech32: invalid padding")
	}
	return out, nil
}
