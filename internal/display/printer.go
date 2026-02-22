package display

import (
	"fmt"
	"time"

	"polymarket-ws-listener/internal/api"
)

func TruncateAddr(addr string) string {
	if len(addr) <= 10 {
		return addr
	}
	return addr[:6] + "..." + addr[len(addr)-4:]
}

func TruncateHash(hash string) string {
	if len(hash) <= 12 {
		return hash
	}
	return hash[:10] + "..."
}

func PrintTrade(t api.Trade) {
	ts := time.Unix(t.Timestamp, 0).Format("15:04:05")
	trader := t.Pseudonym
	if trader == "" {
		trader = t.Name
	}
	if trader == "" {
		trader = TruncateAddr(t.ProxyWallet)
	}

	sideColor := "\033[32m" // green for BUY
	if t.Side == "SELL" {
		sideColor = "\033[31m" // red for SELL
	}
	reset := "\033[0m"

	fmt.Printf("[%s] %sTRADE%s  %s%-4s%s  $%.2f × %.4f  outcome=%-6s  trader=%s  wallet=%s  tx=%s\n",
		ts,
		"\033[1;33m", reset, // yellow TRADE label
		sideColor, t.Side, reset,
		t.Price, t.Size,
		t.Outcome,
		trader,
		TruncateAddr(t.ProxyWallet),
		TruncateHash(t.TransactionHash),
	)
}
