package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const DataAPIBase = "https://data-api.polymarket.com"

type Trade struct {
	TransactionHash string  `json:"transaction_hash"`
	Maker           string  `json:"maker"`
	Taker           string  `json:"taker"`
	ProxyWallet     string  `json:"proxyWallet"`
	Pseudonym       string  `json:"pseudonym"`
	Name            string  `json:"name"`
	AssetId         string  `json:"asset_id"`
	Side            string  `json:"side"`
	Price           float64 `json:"price"`
	Size            float64 `json:"size"`
	Timestamp       int64   `json:"timestamp"`
	Outcome         string  `json:"outcome"`
}

func FetchRecentTrades(assetId string, filterAccount string, sinceTimestamp int64) ([]Trade, error) {
	params := url.Values{}
	params.Set("asset_id", assetId)
	if filterAccount != "" {
		params.Set("maker", filterAccount)
	}
	params.Set("limit", "20")

	return doTradesFetch(params, sinceTimestamp)
}

func FetchRecentTradesGlobal(makerAccount string, sinceTimestamp int64) ([]Trade, error) {
	params := url.Values{}
	params.Set("maker", makerAccount)
	params.Set("limit", "50")

	return doTradesFetch(params, sinceTimestamp)
}

func doTradesFetch(params url.Values, sinceTimestamp int64) ([]Trade, error) {
	u := fmt.Sprintf("%s/trades?%s", DataAPIBase, params.Encode())
	resp, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var trades []Trade
	if err := json.Unmarshal(body, &trades); err != nil {
		return nil, err
	}

	// Filter to only trades after our last seen timestamp
	var recent []Trade
	for _, t := range trades {
		if t.Timestamp >= sinceTimestamp {
			recent = append(recent, t)
		}
	}
	return recent, nil
}
