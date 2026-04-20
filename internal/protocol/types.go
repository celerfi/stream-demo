package protocol

// SessionType defines whether a session is capability-based or time-based
type SessionType string

const (
	SessionTypeCapability SessionType = "capability"
	SessionTypeTime       SessionType = "time"
)

// ProofType defines the verification mode a provider uses
type ProofType string

const (
	ProofOutputCommitment ProofType = "OutputCommitment"
	ProofTEEAttestation   ProofType = "TEEAttestation"
)

// SessionStatus represents the lifecycle state of a session
type SessionStatus string

const (
	StatusNegotiating    SessionStatus = "Negotiating"
	StatusCollateralLock SessionStatus = "CollateralLock"
	StatusActive         SessionStatus = "Active"
	StatusSettling       SessionStatus = "Settling"
	StatusClosed         SessionStatus = "Closed"
	StatusFailed         SessionStatus = "Failed"
)

// CapabilityRequest defines what resources the payer wants to consume
type CapabilityRequest struct {
	ComputeTokens   int64   `json:"compute_tokens"`
	APICalls        int64   `json:"api_calls"`
	MaxSpendUSDC    float64 `json:"max_spend_usdc"`
	RequiredProof   ProofType `json:"required_proof_type"`
}

// SpendingAuthorization is a signed token from a parent account to an agent
type SpendingAuthorization struct {
	ParentPublicKey string  `json:"parent_public_key"`
	AgentPublicKey  string  `json:"agent_public_key"`
	MaxSpend        float64 `json:"max_spend"`
	Currency        string  `json:"currency"`
	Expiry          int64   `json:"expiry"`
	Nonce           string  `json:"nonce"`
	ParentSignature string  `json:"parent_signature"`
}

// SessionOpenRequest is sent from payer to provider to initiate a session
type SessionOpenRequest struct {
	ProtocolVersion      string                `json:"protocol_version"`
	MessageType          string                `json:"message_type"`
	SessionID            string                `json:"session_id"`
	SessionType          SessionType           `json:"session_type"`
	PayerPublicKey       string                `json:"payer_public_key"`
	SpendingAuthorization SpendingAuthorization `json:"spending_authorization"`
	CapabilityRequest    CapabilityRequest     `json:"capability_request"`
	RequiredProofType    ProofType             `json:"required_proof_type"`
	SettlementChain      string                `json:"settlement_chain"`
	SettlementCurrency   string                `json:"settlement_currency"`
	MaxSpend             float64               `json:"max_spend"`
	Timestamp            int64                 `json:"timestamp"`
	PayerSignature       string                `json:"payer_signature"`
}

// SessionOpenResponse is sent from provider back to payer
type SessionOpenResponse struct {
	ProtocolVersion   string        `json:"protocol_version"`
	MessageType       string        `json:"message_type"`
	SessionID         string        `json:"session_id"`
	Accepted          bool          `json:"accepted"`
	RejectionReason   string        `json:"rejection_reason,omitempty"`
	AgreedProofType   ProofType     `json:"agreed_proof_type"`
	AgreedMaxSpend    float64       `json:"agreed_max_spend"`
	ProviderPublicKey string        `json:"provider_public_key"`
	Timestamp         int64         `json:"timestamp"`
}

// CollateralLockProof is sent from payer to provider after locking collateral
type CollateralLockProof struct {
	ProtocolVersion string  `json:"protocol_version"`
	MessageType     string  `json:"message_type"`
	SessionID       string  `json:"session_id"`
	LockedAmount    float64 `json:"locked_amount"`
	TxSignature     string  `json:"tx_signature"`
	Timestamp       int64   `json:"timestamp"`
}

// ConsumptionReport is sent from provider to payer reporting work done
type ConsumptionReport struct {
	ProtocolVersion     string  `json:"protocol_version"`
	MessageType         string  `json:"message_type"`
	SessionID           string  `json:"session_id"`
	SequenceNumber      int64   `json:"sequence_number"`
	ComputeTokensUsed   int64   `json:"compute_tokens_used"`
	CumulativeCost      float64 `json:"cumulative_cost"`
	OutputHash          string  `json:"output_hash"`
	OutputData          string  `json:"output_data"`
	Timestamp           int64   `json:"timestamp"`
	ProviderSignature   string  `json:"provider_signature"`
}

// SignedStateUpdate is sent from payer to provider acknowledging consumption
type SignedStateUpdate struct {
	ProtocolVersion string  `json:"protocol_version"`
	MessageType     string  `json:"message_type"`
	SessionID       string  `json:"session_id"`
	SequenceNumber  int64   `json:"sequence_number"`
	CommittedAmount float64 `json:"committed_amount"`
	ProofCommitment string  `json:"proof_commitment"`
	Currency        string  `json:"currency"`
	Timestamp       int64   `json:"timestamp"`
	PayerSignature  string  `json:"payer_signature"`
}

// SessionCloseRequest is sent by either party to end the session
type SessionCloseRequest struct {
	ProtocolVersion string `json:"protocol_version"`
	MessageType     string `json:"message_type"`
	SessionID       string `json:"session_id"`
	InitiatedBy     string `json:"initiated_by"`
	Timestamp       int64  `json:"timestamp"`
}

// ProviderManifest is the provider's public description of their service
type ProviderManifest struct {
	ProviderPublicKey  string    `json:"provider_public_key"`
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	PricePerToken      float64   `json:"price_per_token"`
	Currency           string    `json:"currency"`
	SupportedProofs    []ProofType `json:"supported_proofs"`
	SettlementChains   []string  `json:"settlement_chains"`
	StreamEndpoint     string    `json:"stream_endpoint"`
	Version            string    `json:"version"`
}