package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// SessionLock represents locked collateral for a session
type SessionLock struct {
	SessionID         string
	PayerPublicKey    string
	ProviderPublicKey string
	LockedAmount      float64
	SettledAmount     float64
	LastSequence      int64
	Status            string
	CreatedAt         time.Time
}

// SettlementResult is returned after a successful settlement
type SettlementResult struct {
	SessionID     string
	SettledAmount float64
	RefundAmount  float64
	Timestamp     time.Time
}

// MockSettlementContract simulates on-chain settlement logic in memory
type MockSettlementContract struct {
	mu       sync.RWMutex
	sessions map[string]*SessionLock
}

// NewMockSettlementContract creates a new in-memory settlement contract
func NewMockSettlementContract() *MockSettlementContract {
	return &MockSettlementContract{
		sessions: make(map[string]*SessionLock),
	}
}

// LockCollateral locks funds for a session
func (c *MockSettlementContract) LockCollateral(
	sessionID string,
	amount float64,
	payerPubKey string,
	providerPubKey string,
) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.sessions[sessionID]; exists {
		return "", fmt.Errorf("session %s already has collateral locked", sessionID)
	}

	if amount <= 0 {
		return "", fmt.Errorf("collateral amount must be greater than zero")
	}

	c.sessions[sessionID] = &SessionLock{
		SessionID:         sessionID,
		PayerPublicKey:    payerPubKey,
		ProviderPublicKey: providerPubKey,
		LockedAmount:      amount,
		SettledAmount:     0,
		LastSequence:      0,
		Status:            "active",
		CreatedAt:         time.Now(),
	}

	txSig := fmt.Sprintf("mock_tx_%s_%d", sessionID[:8], time.Now().UnixNano())
	fmt.Printf("[CONTRACT] Collateral locked: %.6f USDC for session %s\n", amount, sessionID[:8])
	return txSig, nil
}

// VerifyStateUpdate checks a state update is valid without settling
func (c *MockSettlementContract) VerifyStateUpdate(
	sessionID string,
	sequenceNumber int64,
	committedAmount float64,
) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	lock, exists := c.sessions[sessionID]
	if !exists {
		return fmt.Errorf("no collateral found for session %s", sessionID)
	}

	if lock.Status != "active" {
		return fmt.Errorf("session %s is not active (status: %s)", sessionID, lock.Status)
	}

	if committedAmount < lock.SettledAmount {
		return fmt.Errorf(
			"monotonicity violation: committed %.6f is less than already settled %.6f",
			committedAmount, lock.SettledAmount,
		)
	}

	if committedAmount > lock.LockedAmount {
		return fmt.Errorf(
			"committed amount %.6f exceeds locked collateral %.6f",
			committedAmount, lock.LockedAmount,
		)
	}

	if committedAmount <= lock.SettledAmount && lock.SettledAmount > 0 {
		return fmt.Errorf(
			"sequence number %d is not greater than last seen %d",
			sequenceNumber, lock.LastSequence,
		)
	}

	return nil
}

// Settle releases funds to the provider after verifying output commitment
func (c *MockSettlementContract) Settle(
	sessionID string,
	sequenceNumber int64,
	committedAmount float64,
	outputData string,
	proofCommitment string,
) (*SettlementResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	lock, exists := c.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("no collateral found for session %s", sessionID)
	}

	computed := hashOutput(outputData)
	if computed != proofCommitment {
		return nil, fmt.Errorf(
			"output commitment mismatch: got %s expected %s",
			computed, proofCommitment,
		)
	}

	if committedAmount < lock.SettledAmount {
		return nil, fmt.Errorf(
			"monotonicity violation: %.6f < already settled %.6f",
			committedAmount, lock.SettledAmount,
		)
	}

	if committedAmount > lock.LockedAmount {
		return nil, fmt.Errorf(
			"settlement amount %.6f exceeds locked collateral %.6f",
			committedAmount, lock.LockedAmount,
		)
	}

	lock.SettledAmount = committedAmount
	lock.LastSequence = sequenceNumber
	refund := lock.LockedAmount - committedAmount

	fmt.Printf("[CONTRACT] Settled %.6f USDC to provider, %.6f USDC refund pending\n",
		committedAmount, refund)

	return &SettlementResult{
		SessionID:     sessionID,
		SettledAmount: committedAmount,
		RefundAmount:  refund,
		Timestamp:     time.Now(),
	}, nil
}

// Close finalizes the session and returns remaining collateral to payer
func (c *MockSettlementContract) Close(sessionID string) (*SettlementResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	lock, exists := c.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("no collateral found for session %s", sessionID)
	}

	refund := lock.LockedAmount - lock.SettledAmount
	lock.Status = "closed"

	fmt.Printf("[CONTRACT] Session %s closed. Total settled: %.6f USDC, Refunded: %.6f USDC\n",
		sessionID[:8], lock.SettledAmount, refund)

	return &SettlementResult{
		SessionID:     sessionID,
		SettledAmount: lock.SettledAmount,
		RefundAmount:  refund,
		Timestamp:     time.Now(),
	}, nil
}

// GetSession returns the current state of a session lock
func (c *MockSettlementContract) GetSession(sessionID string) (*SessionLock, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	lock, exists := c.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}
	return lock, nil
}

// hashOutput is the internal SHA-256 helper for output commitment verification
func hashOutput(output string) string {
	h := sha256.Sum256([]byte(output))
	return hex.EncodeToString(h[:])
}

