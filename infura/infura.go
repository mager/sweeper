package infura

import (
	"fmt"
	"log"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/kelseyhightower/envconfig"
	"github.com/mager/sweeper/config"
)

// ProvideInfura provides an infura client
func ProvideInfura() *ethclient.Client {
	var cfg config.Config
	err := envconfig.Process("floorreport", &cfg)
	if err != nil {
		log.Fatal(err.Error())
	}
	client, _ := ethclient.Dial(fmt.Sprintf("https://mainnet.infura.io/v3/%s", cfg.InfuraKey))
	return client
}

var Options = ProvideInfura
