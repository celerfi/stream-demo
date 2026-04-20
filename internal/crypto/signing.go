package crypto

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// HashOutput computes SHA-256 of any output data string
// This is what providers commit to before delivering output
func HashOutput(output string) string {
	h := sha256.Sum256([]byte(output))
	return hex.EncodeToString(h[:])
}

// BuildStateUpdateMessage constructs the canonical message a payer signs
// for a SignedStateUpdate. 
// Format mirrors the spec: sessionID || sequenceNumber || committedAmount || proofCommitment || currency || timestamp
func BuildStateUpdateMessage(
	sessionID string,
	sequenceNumber int64,
	committedAmount float64,
	proofCommitment string,
	currency string,
	timestamp int64,
) []byte {
	raw := fmt.Sprintf("%s||%d||%.6f||%s||%s||%d",
		sessionID,
		sequenceNumber,
		committedAmount,
		proofCommitment,
		currency,
		timestamp,
	)
	return []byte(raw)
}

// BuildSessionOpenMessage constructs the message a payer signs
// when sending a SessionOpenRequest
func BuildSessionOpenMessage(
	sessionID string,
	payerPublicKey string,
	maxSpend float64,
	settlementChain string,
	timestamp int64,
) []byte {
	raw := fmt.Sprintf("%s||%s||%.6f||%s||%d",
		sessionID,
		payerPublicKey,
		maxSpend,
		settlementChain,
		timestamp,
	)
	return []byte(raw)
}

// VerifyOutputCommitment checks that the hash of delivered output
// matches the proof commitment the payer signed
func VerifyOutputCommitment(outputData string, proofCommitment string) bool {
	return HashOutput(outputData) == proofCommitment
}