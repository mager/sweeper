package opensea

import (
	"errors"
	"time"

	"github.com/mager/go-opensea/opensea"
	"github.com/mager/sweeper/config"
	"go.uber.org/zap"
)

var (
	OpenSeaRateLimit     = 250 * time.Millisecond
	OpenSeaNotFoundError = "collection_not_found"
)

func NewOpenSeaNotFoundError() error {
	return errors.New(OpenSeaNotFoundError)
}

// ProvideOpenSea provides an HTTP client
func ProvideOpenSea(cfg config.Config, logger *zap.SugaredLogger) *opensea.OpenSeaClient {
	return opensea.NewOpenSeaClient(cfg.OpenSeaAPIKey)
}

var Options = ProvideOpenSea
