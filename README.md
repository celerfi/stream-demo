# CelerFi Stream Protocol

**Live Provider:** https://stream-demo-8eze.onrender.com  
**Live Dashboard:** https://stream-demo-8eze.onrender.com  
**Manifest:** https://stream-demo-8eze.onrender.com/.well-known/stream.json

---

## Hackathon Submission

### What We Built

Stream is CelerFi's open payment protocol for the agentic economy. This submission is a working
demonstration of the core protocol mechanics — a fully autonomous research agent that opens a
payment session with a live AI compute provider, pays per token consumed using cryptographically
signed state updates, verifies every delivery via output commitment, and settles with a trustless
refund of unused collateral.

The agent does all of this without any human in the loop. No checkout flow. No subscription.
No per-request billing infrastructure. Payment flows continuously against real consumption,
verified cryptographically at every step.

---

### Important Context — Demo Scope

This submission is a lightweight demonstration built under time constraints for this hackathon.

**What is real and working in this demo:**
- Ed25519 keypair generation for payer and provider identities
- Signed SessionOpenRequest and SignedStateUpdate messages
- SHA-256 output commitment — payment is cryptographically bound to delivered content
- Monotonicity enforcement — the contract correctly rejects any attempt to reduce a committed amount
- Real Anthropic API calls — the provider calls claude-haiku-4-5 and reports actual token consumption
- Full session lifecycle — discovery, negotiation, collateral lock, streaming payment, settlement, refund
- Live deployed provider at the URL above — anyone can hit it right now

**What is mocked due to time constraints:**
- The settlement contract is in-memory rather than deployed on-chain. In the full implementation
  this is an Anchor program on Solana and a Solidity contract on Base.
- Collateral locking is simulated locally rather than via an on-chain transaction.
- TEE attestation mode is not implemented in this demo — only output commitment mode is live.
- The provider registry is not deployed — manifest discovery is direct URL rather than on-chain lookup.

**What this means:** the cryptographic guarantees, the protocol message flow, the session
lifecycle, and the payment mechanics are all faithful to the full specification. The settlement
layer is the piece that would be replaced by deployed smart contracts in production.

---

### The Full Vision

The complete Stream protocol specification and whitepaper are currently under active development
at CelerFi. The full implementation includes:

- Deployed settlement contracts on Solana (Anchor), Base (Solidity), and a couple of other chains.
- TEE attestation as a second verification mode for compute workloads
- Capability token delegation for agent-to-agent sub-task payments
- Multi-payer sessions for collaborative agent swarms
- CelerFi provider registry — on-chain manifest discovery
- SDKs in TypeScript, Python, Go, and Rust
- Portal integration — gasless transactions, multi-token settlement, fiat on-ramp [Future]

We intend to launch Stream alongside CelerFi's full infrastructure platform in the coming months.
If we qualify for the Loops House Shanghai residency, our goal is to arrive with the on-chain
contracts deployed, the registry live, and the full SDK ready — and use the residency to ship
the production protocol, onboard the first external providers, and demonstrate real agent-to-agent
payments running on CelerFi infrastructure in front of the builder community.

The residency is not where we would start. It is where we would launch.

---

### Why Stream Fits This Hackathon

**Challenge 01 — Autonomous dApp:** Stream is the payment primitive that makes autonomous
dApps possible. An agent that can pay for AI compute, oracle data, or execution infrastructure
without human approval is the foundation of every autonomous workflow. This demo shows exactly
that — an agent completing a research task and paying for it end to end without a single human
interaction.

**Challenge 02 — x402 and Infrastructure:** Stream is a direct architectural alternative to
x402 and MPP. Where x402 is request-response, Stream is continuous. Where MPP settles through
Stripe's proprietary infrastructure, Stream settles through open contracts on Solana and Base.
The protocol is fully open source and the canonical infrastructure is operated by CelerFi with
no proprietary settlement layer.

---

## What This Is

Stream is CelerFi's open payment protocol for the agentic economy. It solves a fundamental
problem with how AI agents pay for services today.

Existing protocols like x402 and Stripe's MPP were designed around request-response: one API
call, one payment, one result. That model breaks down when agents maintain long-running sessions,
consume resources continuously, and delegate subtasks to other agents in parallel.

Stream is built around a different primitive: the payment channel. Instead of a payment per
request, Stream opens a cryptographically secured session where payment flows continuously
against real consumption, with trustless settlement backed by signed state and output commitment
verification.

---

## Demo Walkthrough

**Step 1 — Session Discovery**  
The payer agent fetches the provider manifest from `/.well-known/stream.json`. This is the
Stream equivalent of a service advertisement — the provider publishes its pricing, supported
proof types, accepted settlement chains, and endpoint.

**Step 2 — Session Negotiation**  
The payer constructs a `SessionOpenRequest` with a capability request (50,000 compute tokens,
max 0.50 USDC), signs it with its Ed25519 keypair, and sends it to the provider. The provider
verifies the signature and accepts.

**Step 3 — Collateral Lock**  
The payer locks 0.50 USDC in the settlement contract and sends the transaction proof to the
provider. The session becomes active only after this proof is received. No collateral, no
active session.

**Step 4 — Streaming Payment Cycles**  
The agent sends a research question to the provider. The provider calls the Anthropic API,
receives a response, computes SHA-256(output), and returns a `ConsumptionReport` with the
output hash and real token count. The payer verifies that SHA-256(delivered output) matches
the reported hash, then signs a `SignedStateUpdate` committing the exact USDC cost to the
provider. This repeats across 5 cycles, with the committed amount accumulating monotonically.

**Step 5 — Monotonicity Enforcement**  
The demo explicitly attempts to submit a state update with a committed amount lower than the
current committed total. The settlement contract correctly rejects it with a monotonicity
violation error. This is the core trustless guarantee — a payer cannot roll back a committed
payment once signed.

**Step 6 — Settlement and Close**  
The payer sends a `SessionCloseRequest`. The provider calls `Settle()` on the contract,
submitting the final state update and output data. The contract verifies SHA-256(output) matches
the proof commitment in the signed state. Funds are released to the provider and unused
collateral is refunded to the payer.

---

## Architecture

```
Payer Agent
    │
    ├── 1. Discover manifest (/.well-known/stream.json)
    ├── 2. Send SessionOpenRequest (Ed25519 signed)
    ├── 3. Lock collateral (Settlement Contract)
    ├── 4. Send CollateralLockProof
    │
    │   ── Active Session ──────────────────────────────────
    ├── 5. Request consumption (prompt → provider)
    ├── 6. Provider calls Anthropic, returns ConsumptionReport + outputHash
    ├── 7. Payer verifies SHA-256(output) == outputHash
    ├── 8. Payer signs SignedStateUpdate, sends to provider
    ├── 9. Provider verifies signature + contract monotonicity check
    │   ── Repeat ───────────────────────────────────────────
    │
    ├── 10. Send SessionCloseRequest
    ├── 11. Provider calls Settle() — verifies output commitment
    └── 12. Unused collateral refunded to payer
```

---

## Protocol Messages

| Message | Direction | Purpose |
|---|---|---|
| `SessionOpenRequest` | Payer → Provider | Initiate session, signed by payer keypair |
| `SessionOpenResponse` | Provider → Payer | Accept or reject with reason |
| `CollateralLockProof` | Payer → Provider | Prove funds are locked |
| `ConsumptionReport` | Provider → Payer | Report tokens used + output hash |
| `SignedStateUpdate` | Payer → Provider | Cryptographic payment commitment |
| `SessionCloseRequest` | Either → Either | Trigger settlement |

---

## Security Properties

**Output commitment** — every ConsumptionReport includes SHA-256(output). The payer's
SignedStateUpdate commits to that hash. Settlement fails if the provider submits output
that does not match.

**Monotonicity enforcement** — the settlement contract tracks committed amounts. Any state
update with a lower committed amount than the current is rejected. Stale state attacks
are impossible.

**Ed25519 signatures** — every SessionOpenRequest and SignedStateUpdate is signed by the
payer's keypair. The provider verifies every signature before accepting.

**Collateral lock** — funds are locked before the session goes active. The provider cannot
be paid more than the locked collateral regardless of what state updates say.

---

## Running Locally

```bash
git clone https://github.com/YOUR_USERNAME/stream-demo
cd stream-demo
cp .env.example .env
# add your ANTHROPIC_API_KEY to .env
go run cmd/demo/main.go
```

Open http://localhost:8080 to watch the live session dashboard.

---

## Project Structure

```
stream-demo/
├── cmd/
│   └── demo/
│       └── main.go           # Demo entrypoint
├── internal/
│   ├── protocol/
│   │   └── types.go          # All protocol message types
│   ├── crypto/
│   │   ├── keys.go           # Ed25519 keypair generation and signing
│   │   └── signing.go        # Message construction and output commitment
│   ├── contract/
│   │   └── settlement.go     # Mock settlement contract with monotonicity enforcement
│   ├── provider/
│   │   └── provider.go       # HTTP provider server + Anthropic integration + live UI
│   └── payer/
│       └── payer.go          # Autonomous research agent
├── .env.example
├── go.mod
└── README.md
```

---

## Stack

- **Go** — protocol implementation, provider server, payer agent
- **Ed25519** — message signing (Go standard library)
- **SHA-256** — output commitment hashing (Go standard library)
- **claude-haiku-4-5** — real AI compute powering the provider
- **Render** — live deployment

---

## About CelerFi

CelerFi builds unified multi-chain blockchain infrastructure for Web3 developers — RPC,
indexing, payments, security, and DeFi yield from a single platform.

We started CelerFi after experiencing infrastructure fragmentation firsthand while trying to
build a multichain DeFi trading platform. Every chain meant a different RPC provider,
inconsistent APIs, unpredictable pricing, and months of integration work before we could ship
anything. We pivoted to build the infrastructure layer we wished existed.

Stream is the payment protocol that ties the entire CelerFi stack together for the agentic
economy. This hackathon gave us the forcing function to build the first working demonstration.
We want to use the residency to ship the production version.

**celerfi.network**