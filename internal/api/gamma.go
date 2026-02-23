package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const GammaAPIBase = "https://gamma-api.polymarket.com"

type GammaEvent struct {
	Title   string        `json:"title"`
	Markets []GammaMarket `json:"markets"`
}

type GammaMarket struct {
	ConditionId  string `json:"conditionId"`
	Question     string `json:"question"`
	Outcomes     string `json:"outcomes"`
	ClobTokenIds string `json:"clobTokenIds"`
}

type SearchResponse struct {
	Profiles []Profile `json:"profiles"`
}

type Profile struct {
	Pseudonym   string `json:"pseudonym"`
	Name        string `json:"name"`
	ProxyWallet string `json:"proxyWallet"`
}

func FetchEventBySlug(slug string) (*GammaEvent, error) {
	u := fmt.Sprintf("%s/events/slug/%s", GammaAPIBase, slug)
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

	var event GammaEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, err
	}
	return &event, nil
}

func FetchTopMarkets(limit int) ([]GammaMarket, error) {
	var allMarkets []GammaMarket
	chunkSize := 500
	if limit < chunkSize {
		chunkSize = limit
	}

	numWorkers := limit / chunkSize
	if limit%chunkSize != 0 {
		numWorkers++
	}

	var wg sync.WaitGroup
	var mutex sync.Mutex
	var firstErr error

	for offset := 0; offset < limit; offset += chunkSize {
		wg.Add(1)
		go func(o int) {
			defer wg.Done()
			u := fmt.Sprintf("%s/markets?limit=%d&offset=%d&active=true&closed=false", GammaAPIBase, chunkSize, o)
			resp, err := http.Get(u)
			if err != nil {
				mutex.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mutex.Unlock()
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				mutex.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("status %d", resp.StatusCode)
				}
				mutex.Unlock()
				return
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				mutex.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mutex.Unlock()
				return
			}

			var chunk []GammaMarket
			if err := json.Unmarshal(body, &chunk); err != nil {
				mutex.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mutex.Unlock()
				return
			}

			mutex.Lock()
			allMarkets = append(allMarkets, chunk...)
			mutex.Unlock()
		}(offset)
	}

	wg.Wait()
	if firstErr != nil {
		return allMarkets, firstErr
	}
	return allMarkets, nil
}

func FetchNewestMarkets(limit int) ([]GammaMarket, error) {
	var allMarkets []GammaMarket
	chunkSize := 500
	if limit < chunkSize {
		chunkSize = limit
	}

	for offset := 0; offset < limit; offset += chunkSize {
		u := fmt.Sprintf("%s/markets?limit=%d&offset=%d&active=true&closed=false&order=createdAt&ascending=false", GammaAPIBase, chunkSize, offset)
		resp, err := http.Get(u)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("status %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		var chunk []GammaMarket
		if err := json.Unmarshal(body, &chunk); err != nil {
			return nil, err
		}

		allMarkets = append(allMarkets, chunk...)

		if len(chunk) < chunkSize {
			break
		}
	}

	return allMarkets, nil
}

// We search a user's recent trades to find what this obscure Token ID represents.
func FetchMarketByTokenId(tokenId string, makerAddress string) (string, string, error) {
	u := fmt.Sprintf("https://data-api.polymarket.com/trades?user=%s&limit=20", url.QueryEscape(makerAddress))
	resp, err := http.Get(u)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	// The Data API returns an array of trades.
	// E.g. [{"title": "Bitcoin Up or down...", "outcome": "Yes", "asset_id": "3267..."}]
	var trades []struct {
		Title   string `json:"title"`
		Outcome string `json:"outcome"`
		Asset   string `json:"asset_id"` // Sometimes asset, asset_id, depending on API revision.
		Asset2  string `json:"asset"`
		Asset3  string `json:"token_id"`
	}

	if err := json.Unmarshal(body, &trades); err != nil {
		return "", "", err
	}

	for _, t := range trades {
		id := t.Asset
		if id == "" {
			id = t.Asset2
		}
		if id == "" {
			id = t.Asset3
		}
		if id == tokenId {
			return t.Title, t.Outcome, nil
		}
	}

	return "", "", fmt.Errorf("token %s not found in recent trades for %s", tokenId, makerAddress)
}

func ResolveUserWallet(username string) (string, error) {
	u := fmt.Sprintf("%s/search-v2?q=%s&search_profiles=true", GammaAPIBase, url.QueryEscape(username))
	resp, err := http.Get(u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var searchResp SearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return "", err
	}

	searchUsername := strings.ToLower(username)
	for _, profile := range searchResp.Profiles {
		if strings.Contains(strings.ToLower(profile.Pseudonym), searchUsername) ||
			strings.Contains(strings.ToLower(profile.Name), searchUsername) {
			if profile.ProxyWallet == "" {
				return "", fmt.Errorf("found user but no proxy wallet address available")
			}
			return profile.ProxyWallet, nil
		}
	}

	return "", fmt.Errorf("user not found")
}
