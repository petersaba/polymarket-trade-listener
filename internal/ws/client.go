package ws

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"polymarket-ws-listener/internal/display"

	"github.com/gorilla/websocket"
)

const WSEndpoint = "wss://ws-subscriptions-clob.polymarket.com/ws/market"

func ListenWS(assetIds []string, conditionIds []string, done chan struct{}) {
	if len(assetIds) == 0 {
		return
	}

	c, _, err := websocket.DefaultDialer.Dial(WSEndpoint, nil)
	if err != nil {
		log.Fatalf("WebSocket connection error: %v", err)
	}
	defer c.Close()

	// Build subscription message
	subMsg := map[string]interface{}{
		"assets_ids": assetIds,
		"type":       "market",
	}

	if err := c.WriteJSON(subMsg); err != nil {
		log.Fatalf("WebSocket write error: %v", err)
	}

	// Make sure to close connection when done channel is closed
	go func() {
		<-done
		c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}()

	// Heartbeat goroutine
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := c.WriteMessage(websocket.TextMessage, []byte(`{"type":"ping"}`)); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()

	// Read loop
	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			select {
			case <-done:
				// Expected close
				return
			default:
				log.Printf("WebSocket read error: %v", err)
				return
			}
		}
		processWSMessage(message)
	}
}

func processWSMessage(message []byte) {
	var raw map[string]interface{}
	if err := json.Unmarshal(message, &raw); err != nil {
		// Some messages may be arrays (e.g. batch), try that
		var arr []map[string]interface{}
		if err2 := json.Unmarshal(message, &arr); err2 == nil {
			for _, item := range arr {
				handleWSEvent(item)
			}
		}
		return
	}
	handleWSEvent(raw)
}

func handleWSEvent(raw map[string]interface{}) {
	msgType, _ := raw["event_type"].(string)
	if msgType == "" {
		msgType, _ = raw["type"].(string)
	}

	if msgType == "pong" {
		return
	}

	ts := time.Now().Format("15:04:05")

	switch msgType {
	case "book":
		assetId, _ := raw["asset_id"].(string)
		fmt.Printf("[%s] \033[36mBOOK\033[0m     asset=%s\n", ts, display.TruncateAddr(assetId))
	case "price_change":
		// price_changes is a nested array of per-asset updates
		changes, ok := raw["price_changes"].([]interface{})
		if !ok {
			return
		}
		for _, c := range changes {
			pc, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			assetId, _ := pc["asset_id"].(string)
			price, _ := pc["price"].(string)
			side, _ := pc["side"].(string)
			size, _ := pc["size"].(string)
			bestBid, _ := pc["best_bid"].(string)
			bestAsk, _ := pc["best_ask"].(string)
			fmt.Printf("[%s] \033[35mPRICE\033[0m    asset=%s  price=%s  size=%s  side=%-4s  bid=%s  ask=%s\n",
				ts, display.TruncateAddr(assetId), price, size, side, bestBid, bestAsk)
		}
	case "last_trade_price":
		assetId, _ := raw["asset_id"].(string)
		price, _ := raw["price"].(string)
		fmt.Printf("[%s] \033[33mLAST\033[0m     asset=%s  price=%s\n", ts, display.TruncateAddr(assetId), price)
	default:
		indented, _ := json.MarshalIndent(raw, "", "  ")
		fmt.Printf("[%s] \033[37m%s\033[0m\n%s\n", ts, msgType, string(indented))
	}
}
