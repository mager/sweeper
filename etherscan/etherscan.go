package etherscan

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/mager/sweeper/config"
	etherscan "github.com/nanmu42/etherscan-api"
	"go.uber.org/zap"
)

type EtherscanClient struct {
	Client     *etherscan.Client
	apiKey     string
	httpClient *http.Client
	logger     *zap.SugaredLogger
}

func ProvideEtherscan(cfg config.Config, logger *zap.SugaredLogger) *EtherscanClient {
	client := etherscan.New(etherscan.Mainnet, cfg.EtherscanAPIKey)

	return &EtherscanClient{
		Client: client,
		apiKey: cfg.EtherscanAPIKey,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		logger: logger,
	}
}

var Options = ProvideEtherscan

type EtherscanResp struct {
	Result []EtherscanTrx `json:"result"`
}

type EtherscanTrx struct {
	From      string `json:"from"`
	To        string `json:"to"`
	TokenID   string `json:"tokenID"`
	Timestamp string `json:"timeStamp"`
}

func (e *EtherscanClient) GetNFTTransactionsForContract(
	contract string,
	page int,
	offset int,
) ([]EtherscanTrx, error) {
	u, err := url.Parse("https://api.etherscan.io/api")
	if err != nil {
		log.Fatal(err)
		return []EtherscanTrx{}, nil
	}

	q := u.Query()
	q.Set("apikey", e.apiKey)
	q.Set("contractaddress", contract)
	q.Set("module", "account")
	q.Set("action", "tokennfttx")
	q.Set("sort", "asc")
	q.Set("page", fmt.Sprintf("%d", page))
	q.Set("offset", fmt.Sprintf("%d", offset))
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		log.Fatal(err)
		return []EtherscanTrx{}, nil
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		log.Fatal(err)
		return []EtherscanTrx{}, nil
	}
	defer resp.Body.Close()

	var etherscanResp EtherscanResp
	err = json.NewDecoder(resp.Body).Decode(&etherscanResp)
	if err != nil {
		log.Fatal(err)
		return []EtherscanTrx{}, nil
	}

	return etherscanResp.Result, nil
}

func (e *EtherscanClient) GetAllNFTTransactionsForContract(
	contract string,
	page int,
	offset int,
) ([]EtherscanTrx, error) {
	return e.GetNFTTransactionsForContract(contract, 0, 1000)
}
