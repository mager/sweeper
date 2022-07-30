package config

import (
	"log"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	OpenSeaAPIKey   string
	EtherscanAPIKey string
	ReservoirAPIKey string
	SweeperHost     string
}

func ProvideConfig() Config {
	var cfg Config
	err := envconfig.Process("floorreport", &cfg)
	if err != nil {
		log.Fatal(err.Error())
	}

	return cfg
}

var Options = ProvideConfig
