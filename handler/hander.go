package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	eth "github.com/ethereum/go-ethereum/common"
	"github.com/gorilla/mux"
	"github.com/kr/pretty"
	"github.com/mager/sweeper/coinstats"
	"github.com/mager/sweeper/config"
	"github.com/mager/sweeper/opensea"
	"github.com/mager/sweeper/utils"
	"go.uber.org/zap"
)

// Handler struct for HTTP requests
type Handler struct {
	logger   *zap.SugaredLogger
	router   *mux.Router
	os       opensea.OpenSeaClient
	bq       *bigquery.Client
	cs       coinstats.CoinstatsClient
	cfg      config.Config
	database *firestore.Client
}

// New creates a Handler struct
func New(
	logger *zap.SugaredLogger,
	router *mux.Router,
	os opensea.OpenSeaClient,
	bq *bigquery.Client,
	cs coinstats.CoinstatsClient,
	cfg config.Config,
	database *firestore.Client,
) *Handler {
	h := Handler{logger, router, os, bq, cs, cfg, database}
	h.registerRoutes()

	return &h
}

// RegisterRoutes registers all the routes for the route handler
func (h *Handler) registerRoutes() {
	h.router.HandleFunc("/info", h.getInfo).Methods("POST")
	h.router.HandleFunc("/v2/info", h.getInfoV2).Methods("POST")
}

type BQInfoRequestRecord struct {
	Address       string
	NumNFTs       int
	UnrealizedBag float64
	RequestTime   time.Time
}

type NFT struct {
	Name     string     `json:"name"`
	TokenID  string     `json:"tokenId"`
	ImageURL string     `json:"imageUrl"`
	Traits   []NFTTrait `json:"traits"`
}

type NFTTrait struct {
	Name       string `json:"name"`
	Value      string `json:"value"`
	OpenSeaURL string `json:"openSeaURL"`
}

// Collection represents floor report collection
type Collection struct {
	Name            string  `json:"name"`
	FloorPrice      float64 `json:"floorPrice"`
	OneDayChange    float64 `json:"oneDayChange"`
	ImageURL        string  `json:"imageUrl"`
	NFTs            []NFT   `json:"nfts"`
	OpenSeaURL      string  `json:"openSeaURL"`
	OwnedAssetCount int     `json:"ownedAssetCount"`
	UnrealizedValue float64 `json:"unrealizedValue"`
}

type CollectionStat struct {
	Slug       string  `json:"slug"`
	FloorPrice float64 `json:"floorPrice"`
}

// InfoReq is a request to /info
type InfoReq struct {
	Address string `json:"address"`
	SkipBQ  bool   `json:"skipBQ"`
}

// GetInfoResp is the response for the GET /info endpoint
type GetInfoResp struct {
	Collections      []Collection `json:"collections"`
	UnrealizedBagETH float64      `json:"unrealizedBagETH"`
	UnrealizedBagUSD float64      `json:"unrealizedBagUSD"`
	Username         string       `json:"username"`
	Photo            string       `json:"photo"`
}

// GetInfoRespV2 is the response for the GET /v2/info endpoint
type GetInfoRespV2 struct {
	Collections []Collection `json:"collections"`
	ETHPrice    float64      `json:"ethPrice"`
}

// getInfo is the route handler for the GET /info endpoint
func (h *Handler) getInfo(w http.ResponseWriter, r *http.Request) {
	var (
		err error
		req InfoReq
	)

	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Make sure that the request includes an address
	if req.Address == "" {
		http.Error(w, "you must include an ETH address in the request", http.StatusBadRequest)
		return
	}

	// Validate address
	if !eth.IsHexAddress(req.Address) {
		http.Error(w, "you must include a valid ETH address in the request", http.StatusBadRequest)
		return
	}

	var (
		// ctx             = context.TODO()
		collections     = make([]opensea.OpenSeaCollection, 0)
		nfts            = make([]opensea.OpenSeaAsset, 0)
		ethPrice        float64
		stats           []CollectionStat
		collectionsChan = make(chan []opensea.OpenSeaCollection)
		nftsChan        = make(chan []opensea.OpenSeaAsset)
		ethPriceChan    = make(chan float64)
		// statsChan       = make(chan []CollectionStat)
	)

	// Fetch collections & NFTs from OpenSea
	go h.asyncGetOpenSeaCollections(req.Address, w, collectionsChan)
	collections = <-collectionsChan

	go h.asyncGetOpenSeaAssets(req.Address, w, nftsChan)
	nfts = <-nftsChan

	// Get ETH price
	go h.asyncGetETHPrice(w, ethPriceChan)
	ethPrice = <-ethPriceChan

	// Fetch floor prices from stats endpoint
	// go h.asyncGetOpenSeaCollectionStats(collections, w, statsChan)
	// stats = <-statsChan
	// Transform collections
	adaptedCollections, unrealizedBag := h.adaptCollections(collections, nfts, stats)

	// Record request in BigQuery
	if !req.SkipBQ {
		h.recordRequestInBigQuery(req.Address, len(nfts), adaptedCollections, unrealizedBag)
	}

	sort.Slice(adaptedCollections[:], func(i, j int) bool {
		return adaptedCollections[i].FloorPrice > adaptedCollections[j].FloorPrice
	})

	json.NewEncoder(w).Encode(&GetInfoResp{
		Collections:      adaptedCollections,
		UnrealizedBagETH: unrealizedBag,
		UnrealizedBagUSD: utils.RoundFloat(unrealizedBag*ethPrice, 2),
		Username:         getUsername(nfts),
		Photo:            getPhoto(nfts),
	})
}

// getInfoV2 is the route handler for the GET /v2/info endpoint
func (h *Handler) getInfoV2(w http.ResponseWriter, r *http.Request) {
	var (
		err error
		req InfoReq
	)

	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Make sure that the request includes an address
	if req.Address == "" {
		http.Error(w, "you must include an ETH address in the request", http.StatusBadRequest)
		return
	}

	// Validate address
	if !eth.IsHexAddress(req.Address) {
		http.Error(w, "you must include a valid ETH address in the request", http.StatusBadRequest)
		return
	}

	var (
		ctx         = context.TODO()
		collections = make([]opensea.OpenSeaCollection, 0)
		nfts        = make([]opensea.OpenSeaAsset, 0)
		ethPrice    float64
		// stats           []CollectionStat
		collectionsChan = make(chan []opensea.OpenSeaCollection)
		ethPriceChan    = make(chan float64)
		resp            = GetInfoRespV2{}
		nftsChan        = make(chan []opensea.OpenSeaAsset)
		// statsChan       = make(chan []CollectionStat)
	)

	// Fetch collections & NFTs from OpenSea
	go h.asyncGetOpenSeaCollections(req.Address, w, collectionsChan)
	collections = <-collectionsChan

	go h.asyncGetOpenSeaAssets(req.Address, w, nftsChan)
	nfts = <-nftsChan

	// Get ETH price
	go h.asyncGetETHPrice(w, ethPriceChan)
	ethPrice = <-ethPriceChan

	for _, collection := range collections {
		if collection.Slug == "waveblocks" {
			// Call Firestore to see if the collection exists
			dsnap, err := h.database.Collection("collections").Doc(collection.Slug).Get(ctx)
			if err != nil {
				// This slug does not exist in the Firestore database, initialize it
				h.logger.Infof("Creating collection %s", collection.Slug)

				_, err := h.database.Collection("collections").Doc(collection.Slug).Set(ctx, map[string]interface{}{
					"name":  collection.Name,
					"floor": float64(0.00),
				})
				if err != nil {
					h.logger.Error(err)
				}

				// TODO: Kick off async request to update the collection's floor price

			}
			pretty.Print(dsnap)
			pretty.Print(dsnap.Data())

			resp.Collections = append(resp.Collections, Collection{
				Name:       collection.Name,
				FloorPrice: float64(0.00),
				NFTs:       h.getNFTsV2(collection, nfts),
			})

		}

	}

	// Fetch floor prices from stats endpoint
	// go h.asyncGetOpenSeaCollectionStats(collections, w, statsChan)
	// stats = <-statsChan
	// pretty.Print("STATS!")
	// pretty.Print(stats)
	// Transform collections
	// adaptedCollections, unrealizedBag := h.adaptCollectionsV2(resp, collections, nfts, stats)

	// Record request in BigQuery
	// if !req.SkipBQ {
	// 	h.recordRequestInBigQuery(req.Address, len(nfts), adaptedCollections, unrealizedBag)
	// }

	// sort.Slice(adaptedCollections[:], func(i, j int) bool {
	// return adaptedCollections[i].FloorPrice > adaptedCollections[j].FloorPrice
	// })

	resp.ETHPrice = ethPrice

	json.NewEncoder(w).Encode(resp)
}

// recordRequestInBigQuery posts a info request event to BigQuery
func (h *Handler) recordRequestInBigQuery(
	address string,
	numNFTs int,
	adaptedCollections []Collection,
	unrealizedBag float64,
) {
	var (
		ctx     = context.Background()
		dataset = h.bq.DatasetInProject("floor-report-327113", "info")
		table   = dataset.Table("requests")
		u       = table.Inserter()

		items = []*BQInfoRequestRecord{
			{
				Address:       address,
				NumNFTs:       numNFTs,
				UnrealizedBag: unrealizedBag,
				RequestTime:   time.Now(),
			},
		}
	)
	if err := u.Put(ctx, items); err != nil {
		h.logger.Error(err)
	}
}

func (h *Handler) adaptCollections(collections []opensea.OpenSeaCollection, assets []opensea.OpenSeaAsset, stats []CollectionStat) ([]Collection, float64) {
	var (
		result             []Collection
		totalUnrealizedBag float64 = 0.0
	)
	for _, collection := range collections {
		var (
			floorPrice      = getFloorPrice(collection, stats)
			nfts            = h.getNFTs(collection, assets)
			unrealizedValue = utils.RoundFloat(float64(len(nfts))*floorPrice, 4)
		)
		c := Collection{
			Name:            collection.Name,
			FloorPrice:      floorPrice,
			OneDayChange:    getOneDayChange(collection),
			ImageURL:        collection.ImageURL,
			OpenSeaURL:      fmt.Sprintf("https://opensea.io/collection/%s", collection.Slug),
			OwnedAssetCount: collection.OwnedAssetCount,
			NFTs:            nfts,
			UnrealizedValue: unrealizedValue,
		}
		totalUnrealizedBag += unrealizedValue
		result = append(result, c)
	}
	return result, utils.RoundFloat(totalUnrealizedBag, 3)
}

func getFloorPrice(collection opensea.OpenSeaCollection, stats []CollectionStat) float64 {
	// Find the matching slug from the CollectionStat in the OpenSeaCollection collection
	for _, stat := range stats {
		if stat.Slug == collection.Slug {
			return stat.FloorPrice
		}
	}

	return collection.OpenSeaStats.FloorPrice
}

func getOneDayChange(collection opensea.OpenSeaCollection) float64 {
	return collection.OpenSeaStats.OneDayChange
}

func (h *Handler) getNFTs(collection opensea.OpenSeaCollection, assets []opensea.OpenSeaAsset) []NFT {
	if len(collection.PrimaryAssetContracts) == 0 {
		return []NFT{}
	}

	// Use first contract for now
	contractAddress := collection.PrimaryAssetContracts[0].ContractAddress
	var result []NFT
	for _, asset := range assets {
		if asset.AssetContract.Address == contractAddress {
			nft := NFT{
				Name:     asset.Name,
				TokenID:  asset.TokenID,
				ImageURL: asset.ImageThumbnailURL,
				Traits:   getNFTTraits(asset),
			}
			result = append(result, nft)
		}
	}
	return result
}

func (h *Handler) getNFTsV2(collection opensea.OpenSeaCollection, assets []opensea.OpenSeaAsset) []NFT {
	if len(collection.PrimaryAssetContracts) == 0 {
		return []NFT{}
	}

	// Use first contract for now
	var result []NFT
	for _, asset := range assets {
		if asset.Collection.Slug == collection.Slug {
			nft := NFT{
				Name:     asset.Name,
				TokenID:  asset.TokenID,
				ImageURL: asset.ImageThumbnailURL,
				Traits:   getNFTTraits(asset),
			}
			result = append(result, nft)
		}
	}
	return result
}

func getNFTTraits(asset opensea.OpenSeaAsset) []NFTTrait {
	var result []NFTTrait
	if len(asset.Traits) == 0 {
		return result
	}
	for _, trait := range asset.Traits {
		traitValueStr := getNFTTraitValue(trait.Value)
		nftTrait := NFTTrait{
			Name:       trait.TraitType,
			Value:      traitValueStr,
			OpenSeaURL: getOpenSeaTraitURL(asset, trait),
		}
		result = append(result, nftTrait)
	}
	return result
}

func getNFTTraitValue(t interface{}) string {
	switch t.(type) {
	case string:
		return t.(string)
	case int:
		return fmt.Sprintf("%d", t.(int))
	case float64:
		return fmt.Sprintf("%f", t.(float64))
	default:
		return ""
	}
}

func getOpenSeaTraitURL(asset opensea.OpenSeaAsset, trait opensea.OpenSeaAssetTrait) string {
	return fmt.Sprintf(
		"https://opensea.io/collection/%s?search[stringTraits][0][name]=%s&search[stringTraits][0][values][0]=%s",
		asset.Collection.Slug,
		url.QueryEscape(trait.TraitType),
		url.QueryEscape(getNFTTraitValue(trait.Value)),
	)
}

// getUsername gets the username of the user
func getUsername(nfts []opensea.OpenSeaAsset) string {
	if len(nfts) == 0 {
		return ""
	}

	// Use first NFT for now
	return nfts[0].Owner.User.Username
}

// getPhoto gets the profile photo of the user
func getPhoto(nfts []opensea.OpenSeaAsset) string {
	if len(nfts) == 0 {
		return ""
	}

	// Use first NFT for now
	return nfts[0].Owner.ProfileImgURL
}

// getOpenSeaCollectionStats gets the stats from collections on OpenSea
func (h *Handler) asyncGetOpenSeaCollectionStats(collections []opensea.OpenSeaCollection, w http.ResponseWriter, rc chan []CollectionStat) {
	stats := make([]CollectionStat, 0)

	for _, collection := range collections {
		var (
			cs CollectionStat
		)
		cs.Slug = collection.Slug

		// Call OpenSea API to get stats
		// TODO: Async
		stat, err := h.os.GetCollectionStatsForSlug(collection.Slug)
		if err != nil {
			h.logger.Error(err)
			continue
		}
		cs.FloorPrice = stat.FloorPrice
	}
	rc <- stats
}

// asyncGetOpenSeaCollections gets the collections from OpenSea
func (h *Handler) asyncGetOpenSeaCollections(address string, w http.ResponseWriter, rc chan []opensea.OpenSeaCollection) {
	collections, err := h.os.GetCollectionsForAddress(address)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rc <- collections
}

// asyncGetOpenSeaAssets gets the assets for the given address
func (h *Handler) asyncGetOpenSeaAssets(address string, w http.ResponseWriter, rc chan []opensea.OpenSeaAsset) {
	nfts, err := h.os.GetAllAssetsForAddress(address)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rc <- nfts
}

// asyncGetETHPrice gets the ETH price from the ETH price API
func (h *Handler) asyncGetETHPrice(w http.ResponseWriter, rc chan float64) {
	rc <- h.cs.GetETHPrice()
}
