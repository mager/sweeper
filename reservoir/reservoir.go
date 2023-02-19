package reservoir

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/mager/go-reservoir/reservoir"
	"github.com/mager/sweeper/config"
	"go.uber.org/zap"
)

type ReservoirClient struct {
	httpClient *http.Client
	logger     *zap.SugaredLogger
	baseURL    string
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
		logger:  logger,
		baseURL: "https://api.reservoir.tools",
	}
}

func ProvideReservoirClient(cfg config.Config) *reservoir.ReservoirClient {
	return reservoir.NewReservoirClient(cfg.ReservoirAPIKey)
}

type Attribute struct {
	Key            string    `json:"key"`
	Value          string    `json:"value"`
	FloorAskPrices []float64 `json:"floorAskPrices"`
	SampleImages   []string  `json:"sampleImages"`
}

type AttributesExploreResp struct {
	Attributes []Attribute `json:"attributes"`
}

var Options = ProvideReservoirClient

const (
	limit         = 500
	maxAttributes = 500
)

func (r *ReservoirClient) GetAttributesForContract(contract string, offset int) []Attribute {
	var attributes []Attribute

	u, err := url.Parse(fmt.Sprintf("%s/collections/%s/attributes/explore/v3", r.baseURL, contract))
	if err != nil {
		r.logger.Errorw("Error parsing URL", "error", err)
		return attributes
	}

	q := u.Query()
	q.Set("maxFloorAskPrices", "1")
	q.Set("limit", fmt.Sprint(limit))
	q.Set("offset", fmt.Sprintf("%d", offset))

	u.RawQuery = q.Encode()
	r.logger.Infow("Reservoir Explore Attributes API Call", "url", u.String(), "offset", offset, "contract", contract)

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		r.logger.Errorw("Error creating request", "error", err)
		return attributes
	}

	httpResp, err := r.httpClient.Do(req)
	if err != nil {
		r.logger.Errorw("Error making request", "error", err)
		return attributes
	}

	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		r.logger.Errorw("Error response from reservoir", "status", httpResp.StatusCode)
		return attributes
	}

	// Append attributes to list
	var resp AttributesExploreResp
	err = json.NewDecoder(httpResp.Body).Decode(&resp)
	if err != nil {
		r.logger.Errorw("Error decoding response", "error", err)
		return attributes
	}

	attributes = append(attributes, resp.Attributes...)

	return attributes
}

func (r *ReservoirClient) GetAllAttributesForContract(contract string) []Attribute {
	// For now just fetch 500 attributes for a collection
	return r.GetAttributesForContract(contract, 0)
}
