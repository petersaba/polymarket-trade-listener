package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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
