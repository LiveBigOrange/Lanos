package transport

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// SASModulus is the modulus used to reduce the handshake hash to a 4-digit
// code. PRD §3.3 / docs/PROTOCOL.md §3.3.
const SASModulus = 10000

// SASCode computes the 4-digit Short Authentication String from a Noise
// handshake hash (ChannelBinding):
//
//	code = int.from_bytes(SHA256(handshake_hash), "big") mod 10000
//	code_str = "%04d" % code
//
// Both sides of a completed handshake derive the same code; users visually
// compare the two displays to detect a man-in-the-middle. The outer SHA256
// is applied per the protocol spec (the ChannelBinding is itself already a
// hash, but the spec hashes once more before the mod).
func SASCode(handshakeHash []byte) string {
	sum := sha256.Sum256(handshakeHash)
	v := binary.BigEndian.Uint64(sum[:8]) % SASModulus
	return fmt.Sprintf("%04d", v)
}
