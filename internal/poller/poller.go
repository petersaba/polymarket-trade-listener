package poller

import (
	"log"
	"time"

	"polymarket-ws-listener/internal/api"
	"polymarket-ws-listener/internal/display"
)

const TradesPollDelay = 2 * time.Second

func PollTrades(assetIds []string, filterAccount string, done chan struct{}) {
	seen := make(map[string]bool)           // track seen tx hashes to avoid duplicates
	lastTimestamp := time.Now().Unix() - 30 // start from 30 seconds ago

	for {
		select {
		case <-done:
			return
		default:
		}

		if len(assetIds) == 0 && filterAccount != "" {
			// Global account tracking mode
			trades, err := api.FetchRecentTradesGlobal(filterAccount, lastTimestamp)
			if err != nil {
				log.Printf("Global trade poll error: %v", err)
			} else {
				for _, t := range trades {
					if seen[t.TransactionHash] {
						continue
					}
					seen[t.TransactionHash] = true

					if t.Timestamp > lastTimestamp {
						lastTimestamp = t.Timestamp
					}

					display.PrintTrade(t)
				}
			}
		} else {
			// Specific asset tracking mode
			for _, assetId := range assetIds {
				trades, err := api.FetchRecentTrades(assetId, filterAccount, lastTimestamp)
				if err != nil {
					log.Printf("Trade poll error: %v", err)
					continue
				}

				for _, t := range trades {
					if seen[t.TransactionHash] {
						continue
					}
					seen[t.TransactionHash] = true

					if t.Timestamp > lastTimestamp {
						lastTimestamp = t.Timestamp
					}

					display.PrintTrade(t)
				}
			}
		}

		time.Sleep(TradesPollDelay)
	}
}
