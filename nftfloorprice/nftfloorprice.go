package nftfloorprice

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/mager/sweeper/config"
	"go.uber.org/zap"
)

type NFTFloorPriceClient struct {
	httpClient *http.Client
	logger     *zap.SugaredLogger
}

func ProvideNFTFloorPrice(cfg config.Config, logger *zap.SugaredLogger) *NFTFloorPriceClient {
	return &NFTFloorPriceClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

var Options = ProvideNFTFloorPrice

type Resp struct {
	ProjectData ProjectData `json:"projectData"`
}

type ProjectData struct {
	FloorPriceETH float64 `json:"floorPriceETH"`
}

func (e *NFTFloorPriceClient) GetFloorPriceFromCollection(
	slug string,
) (float64, error) {
	u := fmt.Sprintf("https://api-bff.nftpricefloor.com/nft/%s", slug)
	floor := 0.0
	e.logger.Infow("NFT Floor Price API call", "url", u, "slug", slug)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		log.Fatal(err)
		return floor, nil
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		log.Fatal(err)
		return floor, nil
	}
	defer resp.Body.Close()

	var nftFloorPriceResp Resp
	err = json.NewDecoder(resp.Body).Decode(&nftFloorPriceResp)
	if err != nil {
		e.logger.Errorw(
			"Error decoding NFT Floor Price response",
			"err", err,
		)
		return floor, nil
	}

	// Convert string to float64
	return nftFloorPriceResp.ProjectData.FloorPriceETH, nil
}
