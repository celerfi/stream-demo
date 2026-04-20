package payer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/celerfi/stream-demo/internal/contract"
	"github.com/celerfi/stream-demo/internal/crypto"
	"github.com/celerfi/stream-demo/internal/protocol"
)

// ResearchResult holds the final output of a research session
type ResearchResult struct {
	SessionID       string
	Question        string
	Answer          string
	TokensUsed      int64
	TotalCost       float64
	RefundAmount    float64
	UpdatesSent     int64
	Duration        time.Duration
}

// Payer is the Stream protocol payer agent
type Payer struct {
	keypair         *crypto.Keypair
	contract        *contract.MockSettlementContract
	providerURL     string
	sessionID       string
	sequenceNumber  int64
	committedAmount float64
	spendLimit      float64
	httpClient      *http.Client
}

// NewPayer creates a new payer agent with a fresh keypair
func NewPayer(
	c *contract.MockSettlementContract,
	providerURL string,
	spendLimit float64,
) (*Payer, error) {
	kp, err := crypto.GenerateKeypair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate payer keypair: %w", err)
	}

	fmt.Printf("[PAYER] Public key: %s\n", kp.PublicKeyHex())

	return &Payer{
		keypair:     kp,
		contract:    c,
		providerURL: providerURL,
		spendLimit:  spendLimit,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Research is the main entry point — runs a full autonomous research session
// for a given question and returns the result
func (p *Payer) Research(question string) (*ResearchResult, error) {
	start := time.Now()

	fmt.Printf("\n[AGENT] Question received: %q\n", question)
	fmt.Println("[AGENT] Starting autonomous research session...")

	// Discover provider
	manifest, err := p.DiscoverManifest()
	if err != nil {
		return nil, fmt.Errorf("discovery failed: %w", err)
	}

	// Open session and lock collateral
	if err := p.OpenSession(manifest); err != nil {
		return nil, fmt.Errorf("session open failed: %w", err)
	}

	// Request compute — pass the real question to the provider
	report, err := p.RequestConsumption(question)
	if err != nil {
		return nil, fmt.Errorf("consumption request failed: %w", err)
	}

	answer := report.OutputData
	tokensUsed := report.ComputeTokensUsed

	// Acknowledge the report with a signed state update
	if err := p.AcknowledgeReport(report); err != nil {
		return nil, fmt.Errorf("acknowledgement failed: %w", err)
	}

	// Close the session and settle
	refund, err := p.CloseSession()
	if err != nil {
		return nil, fmt.Errorf("session close failed: %w", err)
	}

	return &ResearchResult{
		SessionID:    p.sessionID,
		Question:     question,
		Answer:       answer,
		TokensUsed:   tokensUsed,
		TotalCost:    p.committedAmount,
		RefundAmount: refund,
		UpdatesSent:  p.sequenceNumber,
		Duration:     time.Since(start),
	}, nil
}

// DiscoverManifest fetches the provider manifest
func (p *Payer) DiscoverManifest() (*protocol.ProviderManifest, error) {
	fmt.Println("[AGENT] Discovering provider manifest...")

	resp, err := p.httpClient.Get(p.providerURL + "/.well-known/stream.json")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer resp.Body.Close()

	var manifest protocol.ProviderManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}

	fmt.Printf("[AGENT] Provider: %s @ %.8f USDC/token\n",
		manifest.Name, manifest.PricePerToken)

	return &manifest, nil
}

// OpenSession negotiates and opens a session with the provider
func (p *Payer) OpenSession(manifest *protocol.ProviderManifest) error {
	sessionID := generateSessionID()
	p.sessionID = sessionID

	fmt.Printf("[AGENT] Opening session %s...\n", sessionID[:8])

	auth := protocol.SpendingAuthorization{
		ParentPublicKey: p.keypair.PublicKeyHex(),
		AgentPublicKey:  p.keypair.PublicKeyHex(),
		MaxSpend:        p.spendLimit,
		Currency:        "USDC",
		Expiry:          time.Now().Add(1 * time.Hour).Unix(),
		Nonce:           generateNonce(),
	}

	timestamp := time.Now().UnixMilli()
	msg := crypto.BuildSessionOpenMessage(
		sessionID,
		p.keypair.PublicKeyHex(),
		p.spendLimit,
		"solana",
		timestamp,
	)
	sig := p.keypair.Sign(msg)

	req := protocol.SessionOpenRequest{
		ProtocolVersion:      "1.0",
		MessageType:          "SessionOpenRequest",
		SessionID:            sessionID,
		SessionType:          protocol.SessionTypeCapability,
		PayerPublicKey:       p.keypair.PublicKeyHex(),
		SpendingAuthorization: auth,
		CapabilityRequest: protocol.CapabilityRequest{
			ComputeTokens: 50000,
			APICalls:      10,
			MaxSpendUSDC:  p.spendLimit,
			RequiredProof: protocol.ProofOutputCommitment,
		},
		RequiredProofType:  protocol.ProofOutputCommitment,
		SettlementChain:    "solana",
		SettlementCurrency: "USDC",
		MaxSpend:           p.spendLimit,
		Timestamp:          timestamp,
		PayerSignature:     sig,
	}

	body, _ := json.Marshal(req)
	resp, err := p.httpClient.Post(
		p.providerURL+"/session/open",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("failed to send session open request: %w", err)
	}
	defer resp.Body.Close()

	var openResp protocol.SessionOpenResponse
	if err := json.NewDecoder(resp.Body).Decode(&openResp); err != nil {
		return fmt.Errorf("failed to decode session open response: %w", err)
	}

	if !openResp.Accepted {
		return fmt.Errorf("session rejected: %s", openResp.RejectionReason)
	}

	fmt.Printf("[AGENT] Session accepted by provider\n")

	// Lock collateral on mock contract
	txSig, err := p.contract.LockCollateral(
		sessionID,
		p.spendLimit,
		p.keypair.PublicKeyHex(),
		manifest.ProviderPublicKey,
	)
	if err != nil {
		return fmt.Errorf("failed to lock collateral: %w", err)
	}

	// Send collateral proof to provider
	proof := protocol.CollateralLockProof{
		ProtocolVersion: "1.0",
		MessageType:     "CollateralLockProof",
		SessionID:       sessionID,
		LockedAmount:    p.spendLimit,
		TxSignature:     txSig,
		Timestamp:       time.Now().UnixMilli(),
	}

	proofBody, _ := json.Marshal(proof)
	proofResp, err := p.httpClient.Post(
		p.providerURL+"/session/collateral",
		"application/json",
		bytes.NewReader(proofBody),
	)
	if err != nil {
		return fmt.Errorf("failed to send collateral proof: %w", err)
	}
	defer proofResp.Body.Close()

	fmt.Printf("[AGENT] Collateral locked — %.4f USDC reserved\n", p.spendLimit)
	return nil
}

// RequestConsumption sends the question to the provider and gets a report back
func (p *Payer) RequestConsumption(question string) (*protocol.ConsumptionReport, error) {
	fmt.Println("[AGENT] Sending question to provider for AI compute...")

	url := fmt.Sprintf("%s/session/consume?session_id=%s&prompt=%s",
		p.providerURL,
		p.sessionID,
		encodeQuery(question),
	)

	resp, err := p.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to request consumption: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("provider error: %s", string(body))
	}

	var report protocol.ConsumptionReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		return nil, fmt.Errorf("failed to decode consumption report: %w", err)
	}

	fmt.Printf("[AGENT] Response received — %d tokens used\n", report.ComputeTokensUsed)
	return &report, nil
}

// AcknowledgeReport verifies output commitment and sends a SignedStateUpdate
func (p *Payer) AcknowledgeReport(report *protocol.ConsumptionReport) error {
	// Verify output commitment
	if !crypto.VerifyOutputCommitment(report.OutputData, report.OutputHash) {
		return fmt.Errorf("output commitment verification FAILED on report #%d — refusing to pay",
			report.SequenceNumber)
	}

	fmt.Printf("[AGENT] Output commitment verified — hash matches delivered content\n")

	// Monotonicity check
	if report.CumulativeCost < p.committedAmount {
		return fmt.Errorf(
			"monotonicity violation: new cost %.6f < committed %.6f",
			report.CumulativeCost, p.committedAmount,
		)
	}

	p.committedAmount = report.CumulativeCost
	p.sequenceNumber = report.SequenceNumber

	// Sign the state update
	timestamp := time.Now().UnixMilli()
	msg := crypto.BuildStateUpdateMessage(
		p.sessionID,
		p.sequenceNumber,
		p.committedAmount,
		report.OutputHash,
		"USDC",
		timestamp,
	)
	sig := p.keypair.Sign(msg)

	update := protocol.SignedStateUpdate{
		ProtocolVersion: "1.0",
		MessageType:     "SignedStateUpdate",
		SessionID:       p.sessionID,
		SequenceNumber:  p.sequenceNumber,
		CommittedAmount: p.committedAmount,
		ProofCommitment: report.OutputHash,
		Currency:        "USDC",
		Timestamp:       timestamp,
		PayerSignature:  sig,
	}

	body, _ := json.Marshal(update)
	resp, err := p.httpClient.Post(
		p.providerURL+"/session/update",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("failed to send state update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("state update rejected: %s", string(b))
	}

	fmt.Printf("[AGENT] SignedStateUpdate sent — committed %.6f USDC to provider\n",
		p.committedAmount)

	return nil
}

// CloseSession sends a close request and returns the refund amount
func (p *Payer) CloseSession() (float64, error) {
	fmt.Printf("[AGENT] Closing session %s...\n", p.sessionID[:8])

	req := protocol.SessionCloseRequest{
		ProtocolVersion: "1.0",
		MessageType:     "SessionCloseRequest",
		SessionID:       p.sessionID,
		InitiatedBy:     "payer",
		Timestamp:       time.Now().UnixMilli(),
	}

	body, _ := json.Marshal(req)
	resp, err := p.httpClient.Post(
		p.providerURL+"/session/close",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to send close request: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to decode close response: %w", err)
	}

	refund, _ := result["refund_amount"].(float64)
	return refund, nil
}

// SessionID returns the active session ID
func (p *Payer) SessionID() string {
	return p.sessionID
}

// CommittedAmount returns the total committed so far
func (p *Payer) CommittedAmount() float64 {
	return p.committedAmount
}

// generateSessionID creates a unique session ID
func generateSessionID() string {
	b := make([]byte, 32)
	for i := range b {
		b[i] = byte(time.Now().UnixNano()>>uint(i%8)) ^ byte(i*7+13)
	}
	return fmt.Sprintf("%x", b)
}

// generateNonce creates a random nonce
func generateNonce() string {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(time.Now().UnixNano()>>uint(i%8)) ^ byte(i*3+7)
	}
	return fmt.Sprintf("%x", b)
}

// encodeQuery does minimal URL encoding for the prompt parameter
func encodeQuery(s string) string {
	encoded := ""
	for _, c := range s {
		switch {
		case (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~':
			encoded += string(c)
		default:
			encoded += fmt.Sprintf("%%%02X", c)
		}
	}
	return encoded
}