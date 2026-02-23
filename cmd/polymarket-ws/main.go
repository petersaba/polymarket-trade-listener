package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"

	"polymarket-ws-listener/internal/api"
	"polymarket-ws-listener/internal/onchain"
	"polymarket-ws-listener/internal/ws"
)

func main() {
	slug := flag.String("slug", "", "Polymarket event slug (e.g., btc-updown-5m-1771719900)")
	tokens := flag.String("tokens", "", "Comma-separated list of token IDs")
	tradesOnly := flag.Bool("trades-only", false, "Only show trades with maker info (skip orderbook/price events)")
	account := flag.String("account", "", "Filter live transactions by a specific proxy wallet address (0x...)")
	username := flag.String("user", "", "Filter live transactions by a Polymarket username/pseudonym")
	showWS := flag.Bool("show-ws", false, "Show WebSocket noise (book/price changes) even if --account/--user is set")
	flag.Parse()

	if *username != "" {
		wallet, err := api.ResolveUserWallet(*username)
		if err != nil {
			log.Fatalf("Failed to resolve user '%s': %v", *username, err)
		}
		fmt.Printf("Resolved username '%s' to wallet: %s\n", *username, wallet)
		*account = wallet
	}

	var assetIds []string
	var conditionIds []string
	knownTokens := make(map[string]string)

	if *tokens != "" {
		assetIds = strings.Split(*tokens, ",")
	} else if *slug != "" {
		fmt.Printf("Looking up event: %s...\n", *slug)
		event, err := api.FetchEventBySlug(*slug)
		if err != nil {
			log.Fatalf("Failed to fetch event: %v", err)
		}
		fmt.Printf("Event: %s\n", event.Title)
		for _, m := range event.Markets {
			var marketIds []string
			if err := json.Unmarshal([]byte(m.ClobTokenIds), &marketIds); err != nil {
				continue
			}
			assetIds = append(assetIds, marketIds...)
			conditionIds = append(conditionIds, m.ConditionId)

			var outcomes []string
			json.Unmarshal([]byte(m.Outcomes), &outcomes)

			for i, id := range marketIds {
				if i < len(outcomes) {
					knownTokens[id] = fmt.Sprintf("%s - %s", m.Question, outcomes[i])
				}
			}

			fmt.Printf("  Market: %s | Outcomes: %v\n", m.Question, outcomes)
		}
	} else if *slug == "" && *tokens == "" {
		fmt.Println("No specific token or slug requested. Pre-loading Active Markets (up to 15,000) for global name resolution...")
		markets, err := api.FetchTopMarkets(15000)
		if err != nil {
			log.Printf("WARN: Failed to fetch top markets: %v\n", err)
		} else {
			for _, m := range markets {
				var marketIds []string
				if err := json.Unmarshal([]byte(m.ClobTokenIds), &marketIds); err != nil {
					continue
				}
				var outcomes []string
				json.Unmarshal([]byte(m.Outcomes), &outcomes)

				for i, id := range marketIds {
					if i < len(outcomes) {
						knownTokens[id] = fmt.Sprintf("%s - %s", m.Question, outcomes[i])
					}
				}
			}
			fmt.Printf("Successfully loaded names for %d active top tokens.\n", len(knownTokens))
		}

		fmt.Println("Pre-loading 500 Newest Markets (for fast-resolving markets like btc-updown)...")
		newMarkets, err := api.FetchNewestMarkets(500)
		if err != nil {
			log.Printf("WARN: Failed to fetch newest markets: %v\n", err)
		} else {
			for _, m := range newMarkets {
				var marketIds []string
				if err := json.Unmarshal([]byte(m.ClobTokenIds), &marketIds); err != nil {
					continue
				}
				var outcomes []string
				json.Unmarshal([]byte(m.Outcomes), &outcomes)

				for i, id := range marketIds {
					if i < len(outcomes) {
						knownTokens[id] = fmt.Sprintf("%s - %s", m.Question, outcomes[i])
					}
				}
			}
			fmt.Printf("Successfully loaded new token cache, total tracked: %d.\n", len(knownTokens))
		}
	} else if *account == "" {
		fmt.Println("Usage:")
		fmt.Println("  go run cmd/polymarket-ws/main.go --slug <event-slug>")
		fmt.Println("  go run cmd/polymarket-ws/main.go --tokens <id1,id2>")
		fmt.Println("  go run cmd/polymarket-ws/main.go --account <wallet-address>")
		fmt.Println("  go run cmd/polymarket-ws/main.go --user <username>")
		fmt.Println("\nFlags:")
		fmt.Println("  --trades-only   Only show trades with maker info")
		fmt.Println("  --account       Filter trades by specific proxy wallet address (e.g. 0x123...)")
		fmt.Println("  --user          Filter trades by Polymarket Username/Pseudonym")
		fmt.Println("  --show-ws       Keep showing orderbook/price WS events when --account is used")
		os.Exit(1)
	}

	globalAccountMode := *account != "" && len(assetIds) == 0

	if !globalAccountMode && len(assetIds) == 0 {
		log.Fatal("No token IDs found for this event")
	}

	// If an account is specified, default to hiding noisy WS updates unless explicitly requested
	suppressWS := *tradesOnly || (*account != "" && !*showWS) || globalAccountMode

	if *account != "" {
		*account = strings.ToLower(*account)
		if globalAccountMode {
			fmt.Printf("\nTracking ALL global trades for account: %s\n", *account)
		} else {
			fmt.Printf("\nFiltering for account: %s", *account)
			if suppressWS {
				fmt.Print(" (WebSocket price noise suppressed)")
			}
			fmt.Println()
		}
	} else {
		fmt.Printf("\nListening on %d token(s)...\n", len(assetIds))
	}
	fmt.Println(strings.Repeat("=", 60))

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	done := make(chan struct{})

	// ── Trade listener (Polygon EVM) ──
	go onchain.ListenOnchain(assetIds, *account, done, knownTokens)

	if suppressWS {
		// Just wait for interrupt, trades are streamed from the blockchain above
		<-interrupt
		close(done)
		log.Println("Shutting down...")
		return
	}

	// ── WebSocket Listener (CLOB) ──
	go ws.ListenWS(assetIds, conditionIds, done)

	<-interrupt
	close(done)
	log.Println("Interrupt received, shutting down...")
}
