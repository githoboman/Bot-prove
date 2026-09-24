package hasher

import (
	"crypto/sha256"
	"encoding/hex"
)

func Hash(data []byte) [32]byte {
	return sha256.Sum256(data)
}

func HexHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func CommitHash(input, output, model []byte) string {
	combined := make([]byte, 0, len(input)+len(output)+len(model))
	combined = append(combined, input...)
	combined = append(combined, output...)
	combined = append(combined, model...)
	return HexHash(combined)
}

func VerifyCommit(commit string, input, output, model []byte) bool {
	return CommitHash(input, output, model) == commit
}
