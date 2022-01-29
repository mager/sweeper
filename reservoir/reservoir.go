package reservoir

import (
	"net/http"
	"time"

	"github.com/mager/sweeper/config"
	"go.uber.org/zap"
)

type ReservoirClient struct {
	httpClient *http.Client
	logger     *zap.SugaredLogger
}

// ProvideReservoir provides an HTTP client
func ProvideReservoir(cfg config.Config, logger *zap.SugaredLogger) *ReservoirClient {
	tr := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
	}

	return &ReservoirClient{
		httpClient: &http.Client{
			Transport: tr,
		},
		logger: logger,
	}
}

var Options = ProvideReservoir

func (r *ReservoirClient) GetAttributes() {}
