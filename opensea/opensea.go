package opensea

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/mager/sweeper/config"
	"go.uber.org/zap"
)

// OpenSeaV1CollectionResp represents an OpenSea collection and also the response from
// the v1/collection/{slug} endpoint
type OpenSeaCollectionResp struct {
	Collection OpenSeaCollection `json:"collection"`
}

// OpenSeaCollection is the inner collection object
type OpenSeaCollection struct {
	Name     string                `json:"name"`
	Slug     string                `json:"slug"`
	ImageURL string                `json:"image_url"`
	Stats    OpenSeaCollectionStat `json:"stats"`
}

// OpenSeaCollectionCollection represents an OpenSea collection and also the response from
// the v1/collections endpoint
type OpenSeaCollectionCollection struct {
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
	OneDayVolume    float64 `json:"one_day_volume"`
	SevenDayVolume  float64 `json:"seven_day_volume"`
	ThirtyDayVolume float64 `json:"thirty_day_volume"`
	TotalVolume     float64 `json:"total_volume"`
	NumOwners       int     `json:"num_owners"`
	TotalSupply     float64 `json:"total_supply"`
	MarketCap       float64 `json:"market_cap"`
	FloorPrice      float64 `json:"floor_price"`
	TotalSales      float64 `json:"total_sales"`
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
	logger     *zap.SugaredLogger
}

// ProvideOpenSea provides an HTTP client
func ProvideOpenSea(cfg config.Config, logger *zap.SugaredLogger) OpenSeaClient {
	tr := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
	}

	return OpenSeaClient{
		httpClient: &http.Client{
			Transport: tr,
		},
		apiKey: cfg.OpenSeaAPIKey,
		logger: logger,
	}
}

var Options = ProvideOpenSea

// GetCollectionsForAddress returns the collections for an address
func (o *OpenSeaClient) GetCollectionsForAddress(address string, offset int) ([]OpenSeaCollectionCollection, error) {
	u, err := url.Parse("https://api.opensea.io/api/v1/collections")
	if err != nil {
		o.logger.Error(err)
		return []OpenSeaCollectionCollection{}, nil
	}
	q := u.Query()
	q.Set("offset", fmt.Sprintf("%d", offset))
	q.Set("limit", fmt.Sprintf("%d", 50))
	q.Set("asset_owner", address)
	u.RawQuery = q.Encode()

	// Fetch collections
	o.logger.Infow("Fetching collections from OpenSea", "address", address, "offset", offset)
	req, err := http.NewRequest("GET", u.String(), nil)
	req.Header.Set("X-API-KEY", o.apiKey)
	if err != nil {
		o.logger.Error(err)
		return []OpenSeaCollectionCollection{}, nil
	}

	resp, err := o.httpClient.Do(req)
	if err != nil {
		o.logger.Error(err)
		return []OpenSeaCollectionCollection{}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		o.logger.Error("Too many requests, please try again later", "status", resp.StatusCode)
		return []OpenSeaCollectionCollection{}, nil
	}

	var openSeaCollections []OpenSeaCollectionCollection
	err = json.NewDecoder(resp.Body).Decode(&openSeaCollections)
	if err != nil {

		o.logger.Error(err)
		return []OpenSeaCollectionCollection{}, nil
	}

	// TODO: Remove once OpenSea fixes rate limit
	if offset > 50 {
		time.Sleep(time.Millisecond * 250)
	}

	return openSeaCollections, nil
}

// GetCollectionStatsForSlug returns the stats for a collection
func (o *OpenSeaClient) GetCollectionStatsForSlug(slug string) (OpenSeaCollectionStat, error) {
	u, err := url.Parse(fmt.Sprintf("https://api.opensea.io/api/v1/collection/%s/stats", slug))
	if err != nil {
		o.logger.Error(err)
		return OpenSeaCollectionStat{}, nil
	}
	q := u.Query()
	u.RawQuery = q.Encode()

	// Fetch stats
	req, err := http.NewRequest("GET", u.String(), nil)
	req.Header.Set("X-API-KEY", o.apiKey)
	if err != nil {
		o.logger.Error(err)
		return OpenSeaCollectionStat{}, nil
	}

	resp, err := o.httpClient.Do(req)
	if resp.StatusCode == http.StatusNotFound {
		// TODO: Delete collection from Firestore
		return OpenSeaCollectionStat{}, nil
	}
	if err != nil {
		o.logger.Error(err)
		return OpenSeaCollectionStat{}, nil
	}
	defer resp.Body.Close()

	var stat OpenSeaCollectionStatResp
	err = json.NewDecoder(resp.Body).Decode(&stat)
	if err != nil {
		o.logger.Error(err)
		return OpenSeaCollectionStat{}, nil
	}

	return stat.Stats, nil
}

// GetAssetsForAddress returns the assets for an address
func (o *OpenSeaClient) GetAssetsForAddress(address string, offset int) ([]OpenSeaAsset, error) {
	u, err := url.Parse(fmt.Sprintf("https://api.opensea.io/api/v1/assets?&offset=%d&limit=50", offset))
	if err != nil {
		o.logger.Error(err)
		return []OpenSeaAsset{}, nil
	}
	q := u.Query()
	q.Set("owner", address)
	u.RawQuery = q.Encode()

	// Fetch assets
	req, err := http.NewRequest("GET", u.String(), nil)
	req.Header.Set("X-API-KEY", o.apiKey)
	if err != nil {
		o.logger.Error(err)
		return []OpenSeaAsset{}, nil
	}
	resp, err := o.httpClient.Do(req)
	if err != nil {
		o.logger.Error(err)
		return []OpenSeaAsset{}, nil
	}
	defer resp.Body.Close()

	var openSeaGetAssetsResp OpenSeaGetAssetsResp
	err = json.NewDecoder(resp.Body).Decode(&openSeaGetAssetsResp)
	if err != nil {
		o.logger.Error(err)
		return []OpenSeaAsset{}, nil
	}

	return openSeaGetAssetsResp.Assets, nil
}

func (o *OpenSeaClient) GetAllCollectionsForAddress(address string) ([]OpenSeaCollectionCollection, error) {
	var allCollections []OpenSeaCollectionCollection
	offset := 0
	for {
		collections, err := o.GetCollectionsForAddress(address, offset)
		if err != nil {
			return []OpenSeaCollectionCollection{}, err
		}
		if len(collections) == 0 {
			break
		}
		allCollections = append(allCollections, collections...)
		offset += 50
	}

	o.logger.Infow("Found collections from OpenSea", "address", address, "count", len(allCollections))
	return allCollections, nil
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

// GetCollection returns the collection from OpenSea
func (o *OpenSeaClient) GetCollection(slug string) (OpenSeaCollectionResp, error) {
	var collection OpenSeaCollectionResp
	u, err := url.Parse(fmt.Sprintf("https://api.opensea.io/api/v1/collection/%s", slug))
	if err != nil {
		o.logger.Error(err)
		return collection, nil
	}
	q := u.Query()
	u.RawQuery = q.Encode()

	// Fetch collection
	req, err := http.NewRequest("GET", u.String(), nil)
	req.Header.Set("X-API-KEY", o.apiKey)
	if err != nil {
		o.logger.Infof("Error creating request: %s", slug)
		o.logger.Error(err)
		return collection, nil
	}

	resp, err := o.httpClient.Do(req)
	if err != nil {
		o.logger.Infof("Error doing request: %s", slug)
		o.logger.Error(err)
		return collection, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		o.logger.Infof("Collection not found: %s", slug)
		// TODO: Delete Collection from Firestore
		return collection, nil
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		log.Println("Too many requests, please try again later")
		return collection, nil
	}

	err = json.NewDecoder(resp.Body).Decode(&collection)
	if err != nil {
		o.logger.Error(err)
		return collection, nil
	}

	return collection, nil
}

func GetOpenSeaCollectionURL(docID string) string {
	return fmt.Sprintf("https://opensea.io/collection/%s", docID)
}
