package etherscan

import (
	"github.com/mager/sweeper/config"
	etherscan "github.com/nanmu42/etherscan-api"
)

func ProvideEtherscan(cfg config.Config) *etherscan.Client {
	client := etherscan.New(etherscan.Mainnet, cfg.EtherscanAPIKey)
	return client
}

var Options = ProvideEtherscan
