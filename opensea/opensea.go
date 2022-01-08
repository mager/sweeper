package opensea

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

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

// OpenSeaCollectionV2 is an experiment
type OpenSeaCollectionV2 struct {
	Name            string `json:"name"`
	Slug            string `json:"slug"`
	ImageURL        string `json:"image_url"`
	Hidden          bool   `json:"hidden"`
	OwnedAssetCount int    `json:"owned_asset_count"`
}

// OpenSeaAssetV2 is an experiment
type OpenSeaAssetV2 struct {
	Name       string                   `json:"name"`
	TokenID    string                   `json:"token_id"`
	ImageURL   string                   `json:"image_url"`
	Collection OpenSeaAssetV2Collection `json:"collection"`
}

// OpenSeaAssetV2Collection is an experiment
type OpenSeaAssetV2Collection struct {
	Slug   string `json:"slug"`
	Hidden bool   `json:"hidden"`
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

// OpenSeaGetAssetsRespV2 represents the response from OpenSea's v1/assets endpoint
type OpenSeaGetAssetsRespV2 struct {
	Assets []OpenSeaAssetV2 `json:"assets"`
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

var (
	DEFAULT_LIMIT = 50
)

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

	o.logger.Info("Fetched collections from OpenSea", "address", address, "offset", offset, "count", len(openSeaCollections))

	// TODO: Remove once OpenSea fixes rate limit
	if offset > DEFAULT_LIMIT {
		time.Sleep(OpenSeaRateLimit)
	}

	return openSeaCollections, nil
}

// GetCollectionsForAddressV2 returns the collections for an address SIMPLER
func (o *OpenSeaClient) GetCollectionsForAddressV2(address string, offset int) ([]OpenSeaCollectionV2, error) {
	var collections = []OpenSeaCollectionV2{}
	u, err := url.Parse("https://api.opensea.io/api/v1/collections")
	if err != nil {
		o.logger.Error(err)
		return collections, nil
	}
	q := u.Query()
	q.Set("offset", fmt.Sprintf("%d", offset))
	q.Set("limit", fmt.Sprintf("%d", 50))
	q.Set("asset_owner", address)
	u.RawQuery = q.Encode()

	// Fetch collections
	o.logger.Infow("Fetching collections from OpenSea V2", "address", address, "offset", offset)
	req, err := http.NewRequest("GET", u.String(), nil)
	req.Header.Set("X-API-KEY", o.apiKey)
	if err != nil {
		o.logger.Error(err)
		return collections, nil
	}

	resp, err := o.httpClient.Do(req)
	if err != nil {
		o.logger.Error(err)
		return collections, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		o.logger.Error("Too many requests, please try again later", "status", resp.StatusCode)
		return collections, nil
	}

	err = json.NewDecoder(resp.Body).Decode(&collections)
	if err != nil {

		o.logger.Error(err)
		return collections, nil
	}

	// TODO: Remove once OpenSea fixes rate limit
	if offset >= DEFAULT_LIMIT {
		time.Sleep(OpenSeaRateLimit)
	}

	// Filter out hidden collections
	var filtered = []OpenSeaCollectionV2{}
	for _, collection := range collections {
		if !collection.Hidden {
			filtered = append(filtered, collection)
		}
	}

	return filtered, nil
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
		return OpenSeaCollectionStat{}, NewOpenSeaNotFoundError()
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

// GetAssetsForAddressV2 returns the assets for an address
func (o *OpenSeaClient) GetAssetsForAddressV2(address string, offset int) ([]OpenSeaAssetV2, error) {
	var assets = []OpenSeaAssetV2{}
	u, err := url.Parse(fmt.Sprintf("https://api.opensea.io/api/v1/assets?&offset=%d&limit=50", offset))
	if err != nil {
		o.logger.Error(err)
		return assets, nil
	}
	q := u.Query()
	q.Set("owner", address)
	u.RawQuery = q.Encode()

	// Fetch assets
	req, err := http.NewRequest("GET", u.String(), nil)
	req.Header.Set("X-API-KEY", o.apiKey)
	if err != nil {
		o.logger.Error(err)
		return assets, nil
	}
	resp, err := o.httpClient.Do(req)
	if err != nil {
		o.logger.Error(err)
		return assets, nil
	}
	defer resp.Body.Close()

	var openSeaGetAssetsResp OpenSeaGetAssetsRespV2
	err = json.NewDecoder(resp.Body).Decode(&openSeaGetAssetsResp)
	if err != nil {
		o.logger.Error(err)
		return assets, nil
	}

	// Filter out assets with hidden collections
	// var filtered = []OpenSeaAssetV2{}
	// for _, asset := range openSeaGetAssetsResp.Assets {
	// 	if !asset.Collection.Hidden {
	// 		filtered = append(filtered, asset)
	// 	}
	// }

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
		offset += DEFAULT_LIMIT
	}

	o.logger.Infow("Found collections from OpenSea", "address", address, "count", len(allCollections))
	return allCollections, nil
}

func (o *OpenSeaClient) GetAllCollectionsForAddressV2(address string) ([]OpenSeaCollectionV2, error) {
	var (
		allCollections []OpenSeaCollectionV2
		offset         = 0
	)

	for {
		collections, err := o.GetCollectionsForAddressV2(address, offset)
		if err != nil {
			return allCollections, err
		}
		if len(collections) == 0 {
			break
		}
		allCollections = append(allCollections, collections...)
		offset += DEFAULT_LIMIT
	}

	o.logger.Infow("Found collections from OpenSea", "address", address, "count", len(allCollections))
	return allCollections, nil
}

// GetAllAssetsForAddressV2 returns the assets for an address
func (o *OpenSeaClient) GetAllAssetsForAddressV2(address string) ([]OpenSeaAssetV2, error) {
	var (
		allAssets []OpenSeaAssetV2
		offset    = 0
	)

	for {
		assets, err := o.GetAssetsForAddressV2(address, offset)
		if err != nil {
			return allAssets, err
		}

		if len(assets) == 0 {
			break
		}

		allAssets = append(allAssets, assets...)
		offset += DEFAULT_LIMIT
	}

	o.logger.Infow("Found assets from OpenSea", "address", address, "count", len(allAssets))

	return allAssets, nil
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
		o.logger.Infow("Collection not found", "collection", slug)
		return collection, NewOpenSeaNotFoundError()
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
