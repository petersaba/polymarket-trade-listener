package onchain

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"polymarket-ws-listener/internal/api"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	PolygonWSS         = "wss://polygon-bor-rpc.publicnode.com"
	CTFExchangeAddress = "0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E"
	NegRiskAddress     = "0xC5d563A36AE78145C45a50134d48A1215220f80a"
	OrderFilledABISnip = `[{
		"anonymous": false,
		"inputs": [
			{"indexed": true, "internalType": "bytes32", "name": "orderHash", "type": "bytes32"},
			{"indexed": true, "internalType": "address", "name": "maker", "type": "address"},
			{"indexed": true, "internalType": "address", "name": "taker", "type": "address"},
			{"indexed": false, "internalType": "uint256", "name": "makerAssetId", "type": "uint256"},
			{"indexed": false, "internalType": "uint256", "name": "takerAssetId", "type": "uint256"},
			{"indexed": false, "internalType": "uint256", "name": "makerAmountFilled", "type": "uint256"},
			{"indexed": false, "internalType": "uint256", "name": "takerAmountFilled", "type": "uint256"},
			{"indexed": false, "internalType": "uint256", "name": "fee", "type": "uint256"}
		],
		"name": "OrderFilled",
		"type": "event"
	}]`
)

type TradeEvent struct {
	OrderHash         [32]byte
	Maker             common.Address
	Taker             common.Address
	MakerAssetId      *big.Int
	TakerAssetId      *big.Int
	MakerAmountFilled *big.Int
	TakerAmountFilled *big.Int
	Fee               *big.Int
}

var (
	tokenMarketCache = make(map[string]string)
	cacheMutex       sync.RWMutex
)

func resolveTokenName(tokenId string, participantAddress string) string {
	cacheMutex.RLock()
	info, exists := tokenMarketCache[tokenId]
	cacheMutex.RUnlock()
	if exists {
		return info
	}

	title, outcome, err := api.FetchMarketByTokenId(tokenId, participantAddress)
	var infoStr string
	if err != nil {
		if len(tokenId) > 12 {
			infoStr = fmt.Sprintf("Token %s...", tokenId[:12])
		} else {
			infoStr = fmt.Sprintf("Token %s", tokenId)
		}
	} else {
		infoStr = fmt.Sprintf("%s - %s", title, outcome)
	}

	cacheMutex.Lock()
	tokenMarketCache[tokenId] = infoStr
	cacheMutex.Unlock()
	return infoStr
}

func ListenOnchain(assetIds []string, filterAccount string, done chan struct{}, knownTokens map[string]string) {
	cacheMutex.Lock()
	for k, v := range knownTokens {
		tokenMarketCache[k] = v
	}
	cacheMutex.Unlock()

	if len(assetIds) == 0 && filterAccount == "" {
		return
	}

	client, err := ethclient.Dial(PolygonWSS)
	if err != nil {
		log.Fatalf("Failed to connect to Polygon WSS: %v", err)
	}
	defer client.Close()

	parsedABI, err := abi.JSON(strings.NewReader(OrderFilledABISnip))
	if err != nil {
		log.Fatalf("Failed to parse ABI: %v", err)
	}

	contractAddresses := []common.Address{
		common.HexToAddress(CTFExchangeAddress),
	}

	query := ethereum.FilterQuery{
		Addresses: contractAddresses,
	}

	logs := make(chan types.Log)
	sub, err := client.SubscribeFilterLogs(context.Background(), query, logs)
	if err != nil {
		log.Fatalf("Failed to subscribe to logs: %v", err)
	}
	defer sub.Unsubscribe()

	// Pre-process asset IDs for quick lookup
	monitoredAssets := make(map[string]bool)
	for _, a := range assetIds {
		var b *big.Int
		var ok bool
		a = strings.TrimSpace(a)
		if strings.HasPrefix(strings.ToLower(a), "0x") {
			b, ok = new(big.Int).SetString(a[2:], 16)
		} else {
			b, ok = new(big.Int).SetString(a, 10)
		}
		if ok {
			monitoredAssets[b.String()] = true
		} else {
			log.Printf("WARN: Failed to parse asset ID: %s", a)
		}
	}

	filterAccLower := strings.ToLower(filterAccount)

	for {
		select {
		case err := <-sub.Err():
			log.Fatalf("Subscription error: %v", err)
		case <-done:
			return
		case vLog := <-logs:
			// OrderFilled is the only event in our ABI
			if vLog.Topics[0] != parsedABI.Events["OrderFilled"].ID {
				continue
			}

			var event TradeEvent
			// Decode un-indexed args
			err := parsedABI.UnpackIntoInterface(&event, "OrderFilled", vLog.Data)
			if err != nil {
				continue
			}

			// Assign indexed args manually from topics
			event.OrderHash = vLog.Topics[1]
			event.Maker = common.BytesToAddress(vLog.Topics[2].Bytes())

			// Some implementations of OrderFilled have taker as indexed, some don't.
			// Let's check how many topics there are.
			if len(vLog.Topics) > 3 {
				event.Taker = common.BytesToAddress(vLog.Topics[3].Bytes())
			}

			// Filter logic
			makerIdStr := event.MakerAssetId.String()
			takerIdStr := event.TakerAssetId.String()

			matchedAsset := false
			if len(monitoredAssets) > 0 {
				if monitoredAssets[makerIdStr] || monitoredAssets[takerIdStr] {
					matchedAsset = true
				}
			} else {
				// Global mode
				matchedAsset = true
			}

			if !matchedAsset {
				continue
			}

			if filterAccLower != "" && filterAccLower != "global" {
				if strings.ToLower(event.Maker.Hex()) != filterAccLower && strings.ToLower(event.Taker.Hex()) != filterAccLower {
					continue
				}
			}

			// Format human-readable
			sizeUSDC := float64(0)
			sizeShares := float64(0)
			sideStr := "Unknown"

			// Fetch market human-readable names
			makerMarketInfo := ""
			takerMarketInfo := ""
			tradedTokenId := ""

			if event.MakerAssetId.Cmp(big.NewInt(0)) == 0 {
				// Maker provided USDC, so Maker was buying YES/NO shares (Taker was selling shares)
				tradedTokenId = event.TakerAssetId.String()
				sizeUSDC = float64(event.MakerAmountFilled.Int64()) / 1e6
				if event.TakerAmountFilled.Cmp(big.NewInt(0)) > 0 {
					sizeShares = float64(event.TakerAmountFilled.Int64()) / 1e6
					price := sizeUSDC / sizeShares
					makerMarketInfo = resolveTokenName(tradedTokenId, event.Maker.Hex())
					sideStr = fmt.Sprintf("Maker BOUGHT %.2f shares of [%s] @ $%.3f", sizeShares, makerMarketInfo, price)
				}
			} else if event.TakerAssetId.Cmp(big.NewInt(0)) == 0 {
				// Taker provided USDC, so Maker was selling YES/NO shares
				tradedTokenId = event.MakerAssetId.String()
				sizeUSDC = float64(event.TakerAmountFilled.Int64()) / 1e6
				if event.MakerAmountFilled.Cmp(big.NewInt(0)) > 0 {
					sizeShares = float64(event.MakerAmountFilled.Int64()) / 1e6
					price := sizeUSDC / sizeShares
					takerMarketInfo = resolveTokenName(tradedTokenId, event.Maker.Hex())
					sideStr = fmt.Sprintf("Maker SOLD %.2f shares of [%s] @ $%.3f", sizeShares, takerMarketInfo, price)
				}
			}

			logTime := time.Now().Format("15:04:05")
			fmt.Printf("[%s] ONCHAIN TRADE | Maker: %s | Taker: %s\n", logTime, event.Maker.Hex(), event.Taker.Hex())
			fmt.Printf("          => %s\n", sideStr)
			fmt.Printf("          => Order Hash: %x\n", event.OrderHash)
			fmt.Printf("          => Tx Hash: %s\n", vLog.TxHash.Hex())
			fmt.Println(strings.Repeat("-", 80))
		}
	}
}
