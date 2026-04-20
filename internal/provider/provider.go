package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/celerfi/stream-demo/internal/contract"
	"github.com/celerfi/stream-demo/internal/crypto"
	"github.com/celerfi/stream-demo/internal/protocol"
)

// ActiveSession tracks a session the provider is currently serving
type ActiveSession struct {
	SessionID       string
	PayerPublicKey  string
	MaxSpend        float64
	Status          protocol.SessionStatus
	SequenceNumber  int64
	TotalConsumed   float64
	LastReport      *protocol.ConsumptionReport
	OpenedAt        time.Time
}

// SessionEvent is broadcast to the web UI for live display
type SessionEvent struct {
	Type      string      `json:"type"`
	Timestamp string      `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// Provider is the Stream protocol provider node
type Provider struct {
	mu            sync.RWMutex
	keypair       *crypto.Keypair
	contract      *contract.MockSettlementContract
	sessions      map[string]*ActiveSession
	pricePerToken float64
	anthropic     *anthropic.Client
	events        []SessionEvent
	eventsMu      sync.RWMutex
}

// NewProvider creates a new provider with a fresh keypair
func NewProvider(c *contract.MockSettlementContract) (*Provider, error) {
	kp, err := crypto.GenerateKeypair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate provider keypair: %w", err)
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	fmt.Printf("[PROVIDER] Public key: %s\n", kp.PublicKeyHex())

	return &Provider{
		keypair:       kp,
		contract:      c,
		sessions:      make(map[string]*ActiveSession),
		pricePerToken: 0.000010,
		anthropic:     &client,
		events:        []SessionEvent{},
	}, nil
}

// logEvent records an event for the live web UI
func (p *Provider) logEvent(eventType string, data interface{}) {
	p.eventsMu.Lock()
	defer p.eventsMu.Unlock()
	p.events = append(p.events, SessionEvent{
		Type:      eventType,
		Timestamp: time.Now().Format("15:04:05.000"),
		Data:      data,
	})
}

// Manifest returns the provider's public manifest
func (p *Provider) Manifest() protocol.ProviderManifest {
	host := os.Getenv("PROVIDER_URL")
	if host == "" {
		host = "http://localhost:8080"
	}
	return protocol.ProviderManifest{
		ProviderPublicKey: p.keypair.PublicKeyHex(),
		Name:              "CelerFi Stream Demo Provider",
		Description:       "AI compute provider accepting Stream protocol sessions",
		PricePerToken:     p.pricePerToken,
		Currency:          "USDC",
		SupportedProofs:   []protocol.ProofType{protocol.ProofOutputCommitment},
		SettlementChains:  []string{"solana", "base"},
		StreamEndpoint:    host,
		Version:           "1.0",
	}
}

// handleManifest serves the provider manifest
func (p *Provider) handleManifest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p.Manifest())
}

// handleSessionOpen processes a SessionOpenRequest from the payer
func (p *Provider) handleSessionOpen(w http.ResponseWriter, r *http.Request) {
	var req protocol.SessionOpenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	fmt.Printf("[PROVIDER] Session open request: %s\n", req.SessionID[:8])

	msg := crypto.BuildSessionOpenMessage(
		req.SessionID,
		req.PayerPublicKey,
		req.MaxSpend,
		req.SettlementChain,
		req.Timestamp,
	)

	valid, err := crypto.Verify(req.PayerPublicKey, msg, req.PayerSignature)
	if err != nil || !valid {
		p.logEvent("session_rejected", map[string]string{
			"session_id": req.SessionID[:8],
			"reason":     "invalid signature",
		})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(protocol.SessionOpenResponse{
			ProtocolVersion: "1.0",
			MessageType:     "SessionOpenResponse",
			SessionID:       req.SessionID,
			Accepted:        false,
			RejectionReason: "invalid payer signature",
			Timestamp:       time.Now().UnixMilli(),
		})
		return
	}

	p.mu.Lock()
	p.sessions[req.SessionID] = &ActiveSession{
		SessionID:      req.SessionID,
		PayerPublicKey: req.PayerPublicKey,
		MaxSpend:       req.MaxSpend,
		Status:         protocol.StatusCollateralLock,
		SequenceNumber: 0,
		TotalConsumed:  0,
		OpenedAt:       time.Now(),
	}
	p.mu.Unlock()

	p.logEvent("session_opened", map[string]interface{}{
		"session_id": req.SessionID[:8],
		"max_spend":  req.MaxSpend,
	})

	fmt.Printf("[PROVIDER] Session %s accepted\n", req.SessionID[:8])

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(protocol.SessionOpenResponse{
		ProtocolVersion:   "1.0",
		MessageType:       "SessionOpenResponse",
		SessionID:         req.SessionID,
		Accepted:          true,
		AgreedProofType:   protocol.ProofOutputCommitment,
		AgreedMaxSpend:    req.MaxSpend,
		ProviderPublicKey: p.keypair.PublicKeyHex(),
		Timestamp:         time.Now().UnixMilli(),
	})
}

// handleCollateralProof receives proof that the payer locked collateral
func (p *Provider) handleCollateralProof(w http.ResponseWriter, r *http.Request) {
	var proof protocol.CollateralLockProof
	if err := json.NewDecoder(r.Body).Decode(&proof); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	p.mu.Lock()
	session, exists := p.sessions[proof.SessionID]
	if !exists {
		p.mu.Unlock()
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	session.Status = protocol.StatusActive
	p.mu.Unlock()

	p.logEvent("collateral_locked", map[string]interface{}{
		"session_id": proof.SessionID[:8],
		"amount":     proof.LockedAmount,
	})

	fmt.Printf("[PROVIDER] Session %s is now ACTIVE\n", proof.SessionID[:8])
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "active"})
}

// handleConsume calls real Anthropic API and returns a ConsumptionReport
func (p *Provider) handleConsume(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	prompt := r.URL.Query().Get("prompt")
	if sessionID == "" {
		http.Error(w, "session_id required", http.StatusBadRequest)
		return
	}
	if prompt == "" {
		prompt = "Explain how streaming payments improve autonomous agent infrastructure in 2 sentences."
	}

	p.mu.Lock()
	session, exists := p.sessions[sessionID]
	if !exists {
		p.mu.Unlock()
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if session.Status != protocol.StatusActive {
		p.mu.Unlock()
		http.Error(w, "session is not active", http.StatusBadRequest)
		return
	}
	p.mu.Unlock()

	// Call real Anthropic API
	fmt.Printf("[PROVIDER] Calling Anthropic for session %s...\n", sessionID[:8])

	message, err := p.anthropic.Messages.New(context.Background(),
		anthropic.MessageNewParams{
			Model:     anthropic.ModelClaudeHaiku4_5_20251001,
			MaxTokens: 300,
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
			},
		},
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("anthropic api error: %s", err), http.StatusInternalServerError)
		return
	}

	outputData := message.Content[0].Text
	tokensUsed := message.Usage.InputTokens + message.Usage.OutputTokens
	cost := float64(tokensUsed) * p.pricePerToken

	p.mu.Lock()
	session.TotalConsumed += cost
	session.SequenceNumber++
	seqNum := session.SequenceNumber
	totalCost := session.TotalConsumed
	p.mu.Unlock()

	outputHash := crypto.HashOutput(outputData)

	reportMsg := fmt.Sprintf("%s||%d||%.6f||%s",
		sessionID, seqNum, totalCost, outputHash)

	report := &protocol.ConsumptionReport{
		ProtocolVersion:   "1.0",
		MessageType:       "ConsumptionReport",
		SessionID:         sessionID,
		SequenceNumber:    seqNum,
		ComputeTokensUsed: tokensUsed,
		CumulativeCost:    totalCost,
		OutputHash:        outputHash,
		OutputData:        outputData,
		Timestamp:         time.Now().UnixMilli(),
		ProviderSignature: p.keypair.Sign([]byte(reportMsg)),
	}

	p.mu.Lock()
	session.LastReport = report
	p.mu.Unlock()

	p.logEvent("consumption_report", map[string]interface{}{
		"session_id":   sessionID[:8],
		"sequence":     seqNum,
		"tokens_used":  tokensUsed,
		"cost":         totalCost,
		"output_preview": truncate(outputData, 80),
	})

	fmt.Printf("[PROVIDER] Report #%d — tokens: %d, cost: %.6f USDC\n",
		seqNum, tokensUsed, totalCost)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

// handleStateUpdate receives a SignedStateUpdate from the payer
func (p *Provider) handleStateUpdate(w http.ResponseWriter, r *http.Request) {
	var update protocol.SignedStateUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	p.mu.RLock()
	session, exists := p.sessions[update.SessionID]
	p.mu.RUnlock()

	if !exists {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	msg := crypto.BuildStateUpdateMessage(
		update.SessionID,
		update.SequenceNumber,
		update.CommittedAmount,
		update.ProofCommitment,
		update.Currency,
		update.Timestamp,
	)

	valid, err := crypto.Verify(session.PayerPublicKey, msg, update.PayerSignature)
	if err != nil || !valid {
		http.Error(w, "invalid payer signature on state update", http.StatusBadRequest)
		return
	}

	if err := p.contract.VerifyStateUpdate(
		update.SessionID,
		update.SequenceNumber,
		update.CommittedAmount,
	); err != nil {
		http.Error(w, fmt.Sprintf("contract verification failed: %s", err), http.StatusBadRequest)
		return
	}

	p.logEvent("state_update", map[string]interface{}{
		"session_id":       update.SessionID[:8],
		"sequence":         update.SequenceNumber,
		"committed_amount": update.CommittedAmount,
	})

	fmt.Printf("[PROVIDER] StateUpdate #%d accepted — committed: %.6f USDC\n",
		update.SequenceNumber, update.CommittedAmount)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}

// handleSessionClose processes a close request and triggers settlement
func (p *Provider) handleSessionClose(w http.ResponseWriter, r *http.Request) {
	var req protocol.SessionCloseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	p.mu.Lock()
	session, exists := p.sessions[req.SessionID]
	if !exists {
		p.mu.Unlock()
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	session.Status = protocol.StatusSettling
	lastReport := session.LastReport
	p.mu.Unlock()

	if lastReport == nil {
		http.Error(w, "no consumption reports found", http.StatusBadRequest)
		return
	}

	result, err := p.contract.Settle(
		req.SessionID,
		lastReport.SequenceNumber,
		lastReport.CumulativeCost,
		lastReport.OutputData,
		lastReport.OutputHash,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("settlement failed: %s", err), http.StatusInternalServerError)
		return
	}

	closeResult, err := p.contract.Close(req.SessionID)
	if err != nil {
		http.Error(w, fmt.Sprintf("close failed: %s", err), http.StatusInternalServerError)
		return
	}

	p.mu.Lock()
	session.Status = protocol.StatusClosed
	p.mu.Unlock()

	p.logEvent("session_closed", map[string]interface{}{
		"session_id":     req.SessionID[:8],
		"settled_amount": result.SettledAmount,
		"refund_amount":  closeResult.RefundAmount,
	})

	fmt.Printf("[PROVIDER] Session %s closed — settled: %.6f USDC\n",
		req.SessionID[:8], result.SettledAmount)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "closed",
		"settled_amount": result.SettledAmount,
		"refund_amount":  closeResult.RefundAmount,
		"timestamp":      closeResult.Timestamp,
	})
}

// handleEvents serves the live event log for the web UI
func (p *Provider) handleEvents(w http.ResponseWriter, r *http.Request) {
	p.eventsMu.RLock()
	defer p.eventsMu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(p.events)
}

// handleHealth is a simple health check endpoint
func (p *Provider) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"version": "1.0",
		"name":    "CelerFi Stream Demo Provider",
	})
}



// handleUI serves the live session dashboard
func (p *Provider) handleUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>CelerFi Stream — Live Session Monitor</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { background: #0a0a0f; color: #e2e8f0; font-family: 'Courier New', monospace; padding: 24px; }
  h1 { color: #7c3aed; font-size: 1.4rem; margin-bottom: 4px; letter-spacing: 0.05em; }
  .subtitle { color: #64748b; font-size: 0.8rem; margin-bottom: 32px; }
  .grid { display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 16px; margin-bottom: 32px; }
  .card { background: #111118; border: 1px solid #1e1e2e; border-radius: 8px; padding: 20px; }
  .card-label { color: #64748b; font-size: 0.7rem; text-transform: uppercase; letter-spacing: 0.1em; margin-bottom: 8px; }
  .card-value { color: #a78bfa; font-size: 1.8rem; font-weight: bold; }
  .card-unit { color: #64748b; font-size: 0.75rem; margin-top: 4px; }
  .section-title { color: #94a3b8; font-size: 0.75rem; text-transform: uppercase; letter-spacing: 0.1em; margin-bottom: 12px; }
  .event-log { background: #111118; border: 1px solid #1e1e2e; border-radius: 8px; padding: 16px; height: 420px; overflow-y: auto; }
  .event { display: flex; gap: 12px; padding: 8px 0; border-bottom: 1px solid #1e1e2e; font-size: 0.78rem; }
  .event:last-child { border-bottom: none; }
  .event-time { color: #475569; min-width: 90px; }
  .event-type { min-width: 160px; font-weight: bold; }
  .event-type.session_opened { color: #34d399; }
  .event-type.collateral_locked { color: #60a5fa; }
  .event-type.consumption_report { color: #a78bfa; }
  .event-type.state_update { color: #f59e0b; }
  .event-type.session_closed { color: #f87171; }
  .event-type.session_rejected { color: #ef4444; }
  .event-data { color: #94a3b8; word-break: break-all; }
  .dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; background: #34d399; margin-right: 8px; animation: pulse 2s infinite; }
  @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.3; } }
  .status-bar { display: flex; align-items: center; margin-bottom: 24px; font-size: 0.8rem; color: #64748b; }
  .empty { color: #334155; font-size: 0.8rem; padding: 20px 0; text-align: center; }
</style>
</head>
<body>

<h1>⚡ CelerFi Stream Protocol</h1>
<p class="subtitle">Live Session Monitor — Autonomous Agent Payment Infrastructure</p>

<div class="status-bar">
  <span class="dot"></span>
  <span id="status">Listening for sessions...</span>
  <span style="margin-left: auto;">Provider: CelerFi Stream Demo &nbsp;|&nbsp; Chain: Solana + Base &nbsp;|&nbsp; Currency: USDC</span>
</div>

<div class="grid">
  <div class="card">
    <div class="card-label">Total Sessions</div>
    <div class="card-value" id="totalSessions">0</div>
    <div class="card-unit">all time</div>
  </div>
  <div class="card">
    <div class="card-label">Total Settled</div>
    <div class="card-value" id="totalSettled">0.000000</div>
    <div class="card-unit">USDC</div>
  </div>
  <div class="card">
    <div class="card-label">State Updates</div>
    <div class="card-value" id="totalUpdates">0</div>
    <div class="card-unit">signed by agents</div>
  </div>
</div>

<div class="section-title">Live Event Stream</div>
<div class="event-log" id="eventLog">
  <div class="empty" id="emptyMsg">Waiting for agent activity...</div>
</div>

<script>
  let lastCount = 0;
  let totalSessions = 0;
  let totalSettled = 0;
  let totalUpdates = 0;

  const typeLabels = {
    session_opened:     'SESSION OPENED',
    collateral_locked:  'COLLATERAL LOCKED',
    consumption_report: 'CONSUMPTION REPORT',
    state_update:       'STATE UPDATE',
    session_closed:     'SESSION CLOSED',
    session_rejected:   'SESSION REJECTED',
  };

  function formatData(type, data) {
    if (!data) return '';
    if (type === 'session_opened')
      return 'session=' + data.session_id + '  max_spend=' + data.max_spend + ' USDC';
    if (type === 'collateral_locked')
      return 'session=' + data.session_id + '  locked=' + (data.amount||0).toFixed(4) + ' USDC';
    if (type === 'consumption_report')
      return 'session=' + data.session_id + '  seq=' + data.sequence + '  tokens=' + data.tokens_used + '  cost=' + (data.cost||0).toFixed(6) + ' USDC  "' + (data.output_preview||'') + '"';
    if (type === 'state_update')
      return 'session=' + data.session_id + '  seq=' + data.sequence + '  committed=' + (data.committed_amount||0).toFixed(6) + ' USDC';
    if (type === 'session_closed')
      return 'session=' + data.session_id + '  settled=' + (data.settled_amount||0).toFixed(6) + ' USDC  refund=' + (data.refund_amount||0).toFixed(6) + ' USDC';
    return JSON.stringify(data);
  }

  async function poll() {
    try {
      const res = await fetch('/events');
      const events = await res.json();

      if (events.length !== lastCount) {
        const log = document.getElementById('eventLog');
        const empty = document.getElementById('emptyMsg');
        if (empty) empty.remove();

        // Add only new events
        for (let i = lastCount; i < events.length; i++) {
          const e = events[i];

          // Update counters
          if (e.type === 'session_opened') totalSessions++;
          if (e.type === 'state_update') totalUpdates++;
          if (e.type === 'session_closed' && e.data)
            totalSettled += e.data.settled_amount || 0;

          const row = document.createElement('div');
          row.className = 'event';
          row.innerHTML =
            '<span class="event-time">' + e.timestamp + '</span>' +
            '<span class="event-type ' + e.type + '">' + (typeLabels[e.type] || e.type) + '</span>' +
            '<span class="event-data">' + formatData(e.type, e.data) + '</span>';
          log.appendChild(row);
        }

        log.scrollTop = log.scrollHeight;
        lastCount = events.length;

        document.getElementById('totalSessions').textContent = totalSessions;
        document.getElementById('totalSettled').textContent = totalSettled.toFixed(6);
        document.getElementById('totalUpdates').textContent = totalUpdates;
        document.getElementById('status').textContent = 'Active — ' + events.length + ' events recorded';
      }
    } catch(e) {}

    setTimeout(poll, 800);
  }

  poll();
</script>
</body>
</html>`)
}









// Start registers all routes and starts the HTTP server
func (p *Provider) Start(addr string) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/.well-known/stream.json", p.handleManifest)
	mux.HandleFunc("/session/open", p.handleSessionOpen)
	mux.HandleFunc("/session/collateral", p.handleCollateralProof)
	mux.HandleFunc("/session/consume", p.handleConsume)
	mux.HandleFunc("/session/update", p.handleStateUpdate)
	mux.HandleFunc("/session/close", p.handleSessionClose)
	mux.HandleFunc("/events", p.handleEvents)
	mux.HandleFunc("/health", p.handleHealth)
	mux.HandleFunc("/", p.handleUI)

	fmt.Printf("[PROVIDER] Server starting on %s\n", addr)
	fmt.Printf("[PROVIDER] Dashboard: http://localhost%s\n", addr)
	return http.ListenAndServe(addr, mux)
}

// truncate shortens a string for display
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}