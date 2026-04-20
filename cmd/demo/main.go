package main

import (
	"fmt"
	"time"

	"github.com/celerfi/stream-demo/internal/contract"
	"github.com/celerfi/stream-demo/internal/payer"
	"github.com/celerfi/stream-demo/internal/provider"
	"github.com/joho/godotenv"
)

const (
	providerAddr = ":8080"
	providerURL  = "http://localhost:8080"
	spendLimit   = 0.50
	cycles       = 5
)

func main() {
	if err := godotenv.Load(); err != nil {
		fmt.Println("[DEMO] No .env file found, using system environment")
	}

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("     CelerFi Stream Protocol Demo")
	fmt.Println("========================================")
	fmt.Println()

	// Step 1: Boot the shared mock settlement contract
	fmt.Println("[DEMO] Initializing mock settlement contract...")
	settlementContract := contract.NewMockSettlementContract()

	// Step 2: Start the provider server in the background
	fmt.Println("[DEMO] Starting provider server...")
	providerNode, err := provider.NewProvider(settlementContract)
	if err != nil {
		panic(fmt.Sprintf("failed to create provider: %s", err))
	}

	go func() {
		if err := providerNode.Start(providerAddr); err != nil {
			panic(fmt.Sprintf("provider server error: %s", err))
		}
	}()

	// Give the server a moment to bind
	time.Sleep(300 * time.Millisecond)

	fmt.Println("[DEMO] Dashboard live at http://localhost:8080")
	fmt.Println("[DEMO] Open it in your browser now, then watch events appear")
	fmt.Println()

	// Step 3: Create the payer agent
	fmt.Println("[DEMO] Creating payer agent...")
	payerAgent, err := payer.NewPayer(settlementContract, providerURL, spendLimit)
	if err != nil {
		panic(fmt.Sprintf("failed to create payer: %s", err))
	}

	// Small pause so you can open the browser before the action starts
	time.Sleep(2 * time.Second)

	fmt.Println()

	// Step 4: Payer discovers the provider manifest
	printStep(1, "Provider Discovery")
	manifest, err := payerAgent.DiscoverManifest()
	if err != nil {
		panic(fmt.Sprintf("failed to discover manifest: %s", err))
	}

	pause()

	// Step 5: Payer opens a session and locks collateral
	printStep(2, "Session Negotiation + Collateral Lock")
	if err := payerAgent.OpenSession(manifest); err != nil {
		panic(fmt.Sprintf("failed to open session: %s", err))
	}

	pause()

	// Step 6: Run consumption cycles
	printStep(3, fmt.Sprintf("Streaming Payment — %d Consumption Cycles", cycles))
	fmt.Println()

	for i := 1; i <= cycles; i++ {
		fmt.Printf("  --- Cycle %d of %d ---\n", i, cycles)

		report, err := payerAgent.RequestConsumption(
			"Explain how streaming payments improve autonomous agent infrastructure in 2 sentences.",
		)
		if err != nil {
			panic(fmt.Sprintf("cycle %d: failed to get consumption report: %s", i, err))
		}

		fmt.Printf("  [PAYER] Report received — tokens: %d, cost so far: %.6f USDC\n",
			report.ComputeTokensUsed, report.CumulativeCost)

		if err := payerAgent.AcknowledgeReport(report); err != nil {
			panic(fmt.Sprintf("cycle %d: failed to acknowledge report: %s", i, err))
		}

		time.Sleep(300 * time.Millisecond)
		fmt.Println()
	}

	pause()

	// Step 7: Demonstrate monotonicity enforcement
	printStep(4, "Monotonicity Enforcement Check")
	fmt.Println("[DEMO] Attempting to verify a state update below the committed amount...")

	err = settlementContract.VerifyStateUpdate(
		payerAgent.SessionID(),
		999,
		0.000001,
	)
	if err != nil {
		fmt.Printf("[DEMO] Correctly rejected: %s\n", err)
	} else {
		fmt.Println("[DEMO] ERROR: should have been rejected but wasn't")
	}

	pause()

	// Step 8: Close the session and settle
	printStep(5, "Session Close + Settlement")
	_, err = payerAgent.CloseSession()
	if err != nil {
		panic(fmt.Sprintf("failed to close session: %s", err))
	}

	// Print final summary
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("         SESSION COMPLETE")
	fmt.Println("========================================")
	fmt.Printf("  Session ID    : %s\n", payerAgent.SessionID()[:8])
	fmt.Printf("  Total spent   : %.6f USDC\n", payerAgent.CommittedAmount())
	fmt.Printf("  Updates sent  : %d\n", cycles)
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("[DEMO] Stream protocol demo complete.")
	fmt.Println("[DEMO] Dashboard still live at http://localhost:8080")
	fmt.Println()

	// Keep server alive so you can inspect the dashboard after the demo
	select {}
}

// printStep prints a clearly labelled demo step header
func printStep(n int, title string) {
	fmt.Println()
	fmt.Printf("----------------------------------------\n")
	fmt.Printf("  Step %d: %s\n", n, title)
	fmt.Printf("----------------------------------------\n")
}

// pause adds a small delay between steps so the output is readable
func pause() {
	time.Sleep(400 * time.Millisecond)
}