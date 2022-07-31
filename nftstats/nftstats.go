package nftstats

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/mager/sweeper/config"
	"go.uber.org/zap"
)

type NFTStatsClient struct {
	apiKey     string
	httpClient *http.Client
	logger     *zap.SugaredLogger
}

func ProvideNFTStats(cfg config.Config, logger *zap.SugaredLogger) *NFTStatsClient {
	return &NFTStatsClient{
		apiKey: cfg.EtherscanAPIKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

var Options = ProvideNFTStats

type NFT struct {
	Image  string `firestore:"image" json:"image"`
	Name   string `firestore:"name" json:"name"`
	OSLink string `firestore:"osLink" json:"osLink"`
}

type Resp struct {
	Toplists struct {
		SevenD struct {
			Nft []Nft `json:"nft"`
		} `json:"7d"`
		Two4H struct {
			Nft []Nft `json:"nft"`
		} `json:"24h"`
		Three0D struct {
			Nft []Nft `json:"nft"`
		} `json:"30d"`
	} `json:"topLists"`
}

type Nft struct {
	Name        string `json:"name"`
	Imageurl    string `json:"imageUrl"`
	Opensealink string `json:"openSeaLink"`
}

func (e *NFTStatsClient) GetTopNFTs(
	slug string,
) ([]NFT, error) {
	u := fmt.Sprintf("https://api.nft-stats.com/collection_details/%s", slug)

	e.logger.Infow("NFT Stats API call", "url", u, "slug", slug)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		log.Fatal(err)
		return []NFT{}, nil
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		log.Fatal(err)
		return []NFT{}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return []NFT{}, nil
	}

	var nftStatsResp Resp
	err = json.NewDecoder(resp.Body).Decode(&nftStatsResp)
	if err != nil {
		e.logger.Errorw(
			"Error decoding NFT Stats response",
			"err", err,
		)
		return []NFT{}, nil
	}

	var nfts []NFT
	for _, nft := range nftStatsResp.Toplists.Three0D.Nft {
		nfts = append(nfts, NFT{
			Image:  nft.Imageurl,
			Name:   nft.Name,
			OSLink: nft.Opensealink,
		})
	}

	return nfts, nil
}
