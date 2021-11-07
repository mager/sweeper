package opensea

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/kr/pretty"
	"github.com/mager/sweeper/config"
)

// OpenSeaCollection represents an OpenSea collection and also the response from
// the v1/collections endpoint
type OpenSeaCollection struct {
	Name                  string                         `json:"name"`
	FloorPrice            float64                        `json:"floor_price"`
	PrimaryAssetContracts []OpenSeaPrimaryAssetContracts `json:"primary_asset_contracts"`
	OpenSeaStats          OpenSeaStats                   `json:"stats"`
	ImageURL              string                         `json:"image_url"`
	Slug                  string                         `json:"slug"`
	OwnedAssetCount       int                            `json:"owned_asset_count"`
}

// OpenSeaCollectionStat represents an OpenSea collection stat object
type OpenSeaCollectionStat struct {
	FloorPrice float64 `json:"floor_price"`
	TotalSales float64 `json:"total_sales"`
}

// OpenSeaCollectionStatResp represents the response from the v1/collection/{slug}/stats endpoint
type OpenSeaCollectionStatResp struct {
	Stats OpenSeaCollectionStat `json:"stats"`
}

// OpenSeaPrimaryAssetContracts represents a contract in OpenSea
type OpenSeaPrimaryAssetContracts struct {
	Name            string `json:"name"`
	ContractAddress string `json:"address"`
}

// OpenSeaStats represents a contract's stats in OpenSea
type OpenSeaStats struct {
	FloorPrice   float64 `json:"floor_price"`
	OneDayChange float64 `json:"one_day_change"`
}

// OpenSeaGetAssetsResp represents the response from OpenSea's v1/assets endpoint
type OpenSeaGetAssetsResp struct {
	Assets []OpenSeaAsset `json:"assets"`
}

// OpenSeaAsset represents an asset on OpenSea
type OpenSeaAsset struct {
	Name              string                 `json:"name"`
	AssetContract     OpenSeaAssetContract   `json:"asset_contract"`
	TokenID           string                 `json:"token_id"`
	ImageThumbnailURL string                 `json:"image_thumbnail_url"`
	Traits            []OpenSeaAssetTrait    `json:"traits"`
	Collection        OpenSeaAssetCollection `json:"collection"`
	Owner             OpenSeaOwner           `json:"owner"`
}

type OpenSeaOwner struct {
	User          OpenSeaUser `json:"user"`
	ProfileImgURL string      `json:"profile_img_url"`
}

type OpenSeaUser struct {
	Username string `json:"username"`
}

// OpenSeaAssetCollection represents a collection inside an OpenSea asset
type OpenSeaAssetCollection struct {
	Slug string `json:"slug"`
}

type OpenSeaAssetTrait struct {
	TraitType string      `json:"trait_type"`
	Value     interface{} `json:"value"`
}

// OpenSeaAssetContract represents a contract with the OpenSea asset
type OpenSeaAssetContract struct {
	Address string `json:"address"`
}

type OpenSeaClient struct {
	httpClient *http.Client
	apiKey     string
}

// ProvideOpenSea provides an HTTP client
func ProvideOpenSea() OpenSeaClient {
	tr := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
	}

	var cfg config.Config
	err := envconfig.Process("floorreport", &cfg)
	if err != nil {
		log.Fatal(err.Error())
	}

	return OpenSeaClient{
		httpClient: &http.Client{
			Transport: tr,
		},
		apiKey: cfg.OpenSeaAPIKey,
	}
}

var Options = ProvideOpenSea

// GetCollectionsForAddress returns the collections for an address
func (o *OpenSeaClient) GetCollectionsForAddress(address string) ([]OpenSeaCollection, error) {
	u, err := url.Parse("https://api.opensea.io/api/v1/collections?&offset=0&limit=50")
	if err != nil {
		log.Fatal(err)
		return []OpenSeaCollection{}, nil
	}
	q := u.Query()
	q.Set("asset_owner", address)
	u.RawQuery = q.Encode()

	// Fetch collections
	req, err := http.NewRequest("GET", u.String(), nil)
	req.Header.Set("X-API-KEY", o.apiKey)
	if err != nil {
		log.Fatal(err)
		return []OpenSeaCollection{}, nil
	}

	resp, err := o.httpClient.Do(req)
	if err != nil {
		log.Fatal(err)
		return []OpenSeaCollection{}, nil
	}
	defer resp.Body.Close()

	var openSeaCollections []OpenSeaCollection
	err = json.NewDecoder(resp.Body).Decode(&openSeaCollections)
	if err != nil {
		log.Fatal(err)
		return []OpenSeaCollection{}, nil
	}

	return openSeaCollections, nil
}

// GetCollectionStatsForSlug returns the stats for a collection
func (o *OpenSeaClient) GetCollectionStatsForSlug(slug string) (OpenSeaCollectionStat, error) {
	u, err := url.Parse(fmt.Sprintf("https://api.opensea.io/api/v1/collection/%s/stats", slug))
	if err != nil {
		log.Fatal(err)
		return OpenSeaCollectionStat{}, nil
	}
	q := u.Query()
	u.RawQuery = q.Encode()
	pretty.Print(u.String())

	// Fetch stats
	req, err := http.NewRequest("GET", u.String(), nil)
	req.Header.Set("X-API-KEY", o.apiKey)
	if err != nil {
		log.Fatal(err)
		return OpenSeaCollectionStat{}, nil
	}

	resp, err := o.httpClient.Do(req)
	if err != nil {
		log.Fatal(err)
		return OpenSeaCollectionStat{}, nil
	}
	defer resp.Body.Close()

	var stat OpenSeaCollectionStatResp
	err = json.NewDecoder(resp.Body).Decode(&stat)
	if err != nil {
		log.Fatal(err)
		return OpenSeaCollectionStat{}, nil
	}

	return stat.Stats, nil
}

// GetCollectionsForAddress returns the assets for an address
func (o *OpenSeaClient) GetAssetsForAddress(address string, offset int) ([]OpenSeaAsset, error) {
	u, err := url.Parse(fmt.Sprintf("https://api.opensea.io/api/v1/assets?&offset=%d&limit=50", offset))
	if err != nil {
		log.Fatal(err)
		return []OpenSeaAsset{}, nil
	}
	q := u.Query()
	q.Set("owner", address)
	o.httpClient.Get("")
	u.RawQuery = q.Encode()

	// Fetch assets
	req, err := http.NewRequest("GET", u.String(), nil)
	req.Header.Set("X-API-KEY", o.apiKey)
	if err != nil {
		log.Fatal(err)
		return []OpenSeaAsset{}, nil
	}
	resp, err := o.httpClient.Do(req)
	if err != nil {
		log.Fatal(err)
		return []OpenSeaAsset{}, nil
	}
	defer resp.Body.Close()

	var openSeaGetAssetsResp OpenSeaGetAssetsResp
	err = json.NewDecoder(resp.Body).Decode(&openSeaGetAssetsResp)
	if err != nil {
		log.Fatal(err)
		return []OpenSeaAsset{}, nil
	}

	return openSeaGetAssetsResp.Assets, nil
}

// GetAllAssetsForAddress returns the assets for an address
func (o *OpenSeaClient) GetAllAssetsForAddress(address string) ([]OpenSeaAsset, error) {
	var assets []OpenSeaAsset
	// TODO: Fetch more than 250 and clean this up a lot
	first50, err := o.GetAssetsForAddress(address, 0)
	if err != nil {
		return assets, err
	}
	assets = append(assets, first50...)
	if len(assets) == 50 {
		second50, err := o.GetAssetsForAddress(address, 50)
		if err != nil {
			return assets, err
		}
		assets = append(assets, second50...)
	}
	if len(assets) == 100 {
		third50, err := o.GetAssetsForAddress(address, 100)
		if err != nil {
			return assets, err
		}
		assets = append(assets, third50...)
	}
	if len(assets) == 150 {
		fourth50, err := o.GetAssetsForAddress(address, 150)
		if err != nil {
			return assets, err
		}
		assets = append(assets, fourth50...)
	}
	if len(assets) == 200 {
		fifth50, err := o.GetAssetsForAddress(address, 200)
		if err != nil {
			return assets, err
		}
		assets = append(assets, fifth50...)
	}
	return assets, nil
}
