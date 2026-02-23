# Polymarket WebSocket Listener

A high-performance Go-based listener for real-time Polymarket data. It listens to both the Central Limit Order Book (CLOB) via WebSocket and on-chain trades on Polygon EVM.

Features include tracking trades by event slug, specific token IDs, wallet address, or even a user's Polymarket pseudonym.

## Prerequisites

- [Go](https://go.dev/doc/install) 1.24.0 or higher.

## Installation

Clone the repository and install dependencies:

```bash
git clone https://github.com/petersaba/polymarket-trade-listener.git
cd polymarket-trade-listener
go mod download
```

## Usage

You can run the application directly using `go run`:

```bash
go run cmd/polymarket-ws/main.go [flags]
```

### Flags

| Flag | Description |
|---|---|
| `--slug` | Polymarket event slug (e.g., `btc-updown-5m-1771719900`). Will fetch associated markets. |
| `--tokens` | Comma-separated list of token IDs to monitor. |
| `--account` | Filter live transactions by a specific Proxy Wallet address (`0x...`). |
| `--user` | Filter live transactions by a Polymarket Username/Pseudonym. |
| `--trades-only` | Only show executed trades with maker info (suppresses orderbook/price update noise). |
| `--show-ws` | Show WebSocket noise (book/price changes) even if `--account` or `--user` is set. |

### Examples

**1. Listen to a specific event by slug:**
```bash
go run cmd/polymarket-ws/main.go --slug btc-updown-5m-1771719900
```

**2. Track all trades for a specific Polymarket user:**
*(This will automatically resolve the username to their proxy wallet address)*
```bash
go run cmd/polymarket-ws/main.go --user polymarket_whale
```

**3. Monitor specific token IDs:**
```bash
go run cmd/polymarket-ws/main.go --tokens "12345678,87654321"
```

**4. Track an account and suppress orderbook noise:**
```bash
go run cmd/polymarket-ws/main.go --account 0x123abc... --trades-only
```

## How It Works

1. **Onchain Listener (EVM):** Listens to live Polygon blockchain events to capture definitive executed trades and maker information.
2. **WebSocket Listener (CLOB):** Connects to Polymarket's Gamma API via WebSocket to stream real-time orderbook updates, price ticks, and fast off-chain trades.
3. **API Resolution:** Before listening, it uses the regular Polymarket REST API to resolve things like usernames to proxy wallets and event slugs to their underlying condition/token IDs.
