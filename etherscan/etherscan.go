package etherscan

import (
	"github.com/mager/sweeper/config"
	etherscan "github.com/nanmu42/etherscan-api"
)

type EtherscanClient struct {
	Client *etherscan.Client
}

func ProvideEtherscan(cfg config.Config) *EtherscanClient {
	client := etherscan.New(etherscan.Mainnet, cfg.EtherscanAPIKey)

	return &EtherscanClient{
		Client: client,
	}
}

var Options = ProvideEtherscan
