package coinstats

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"time"
)

type Coin struct {
	ID    string  `json:"id"`
	Price float64 `json:"price"`
}

type Coins []Coin

type CoinsResp struct {
	Coins Coins `json:"coins"`
}

type CoinstatsClient struct {
	httpClient *http.Client
}

// ProvideCoinstats provides an HTTP client
func ProvideCoinstats() CoinstatsClient {
	tr := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
	}

	return CoinstatsClient{
		httpClient: &http.Client{Transport: tr},
	}
}

var Options = ProvideCoinstats

// GetCoins returns the coin prices
func (c *CoinstatsClient) GetCoins() (CoinsResp, error) {
	var coinsResp CoinsResp
	u, err := url.Parse("https://api.coinstats.app/public/v1/coins?skip=0&limit=5&currency=USD")
	if err != nil {
		log.Fatal(err)
		return coinsResp, nil
	}

	// Fetch collections
	resp, err := c.httpClient.Get(u.String())
	if err != nil {
		log.Fatal(err)
		return coinsResp, nil
	}
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(&coinsResp)
	if err != nil {
		log.Fatal(err)
		return coinsResp, nil
	}

	return coinsResp, nil
}

func (c *CoinstatsClient) GetETHPrice() float64 {
	coinsResp, err := c.GetCoins()
	if err != nil {
		return 0.0
	}

	for _, coin := range coinsResp.Coins {
		if coin.ID == "ethereum" {
			return coin.Price
		}
	}

	return 0.0
}
