package etherscan

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
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
	BlockNumber string `json:"blockNumber"`
	Hash        string `json:"hash"`
	From        string `json:"from"`
	To          string `json:"to"`
	TokenID     string `json:"tokenID"`
	Timestamp   string `json:"timeStamp"`
}

func (e *EtherscanClient) GetNFTTransactionsForContract(
	contract string,
	offset int,
	startBlock int64,
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
	q.Set("offset", fmt.Sprintf("%d", offset))
	q.Set("endblock", "999999999")
	if startBlock > 0 {
		q.Set("startblock", fmt.Sprintf("%d", startBlock))
	}
	u.RawQuery = q.Encode()

	e.logger.Infow("Etherscan API call", "url", u.String())
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

	// Etherscan's rate limit is 5/sec
	time.Sleep(250 * time.Millisecond)

	return etherscanResp.Result, nil
}

func (e *EtherscanClient) GetLatestTransactionsForContract(
	contract string,
	startBlock int64,
) ([]EtherscanTrx, error) {
	var (
		transactions []EtherscanTrx
		offset       = 0
		latestBlock  = startBlock
	)

	for {
		trxs, err := e.GetNFTTransactionsForContract(contract, offset, latestBlock)
		if err != nil {
			return []EtherscanTrx{}, err
		}

		if len(trxs) == 0 || len(trxs) == 1 {
			e.logger.Infow("Breaking out", "len", len(trxs))
			break
		}

		transactions = append(transactions, trxs...)

		// Get blockNumber from last trx
		lastTrx := trxs[len(trxs)-1]
		i, err := strconv.ParseInt(lastTrx.BlockNumber, 10, 64)
		if err != nil {
			log.Fatal(err)
			return []EtherscanTrx{}, nil
		}
		latestBlock = i
		e.logger.Infof("Latest block: %d", latestBlock)
	}

	return transactions, nil
}
