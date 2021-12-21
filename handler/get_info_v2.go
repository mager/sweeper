package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/bwmarrin/discordgo"
	"github.com/ethereum/go-ethereum/common"
	eth "github.com/ethereum/go-ethereum/common"
	"github.com/mager/sweeper/bigquery"
	"github.com/mager/sweeper/opensea"
	"github.com/mager/sweeper/utils"
	ens "github.com/wealdtech/go-ens/v3"
)

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

type CollectionV2 struct {
	Name           string    `firestore:"name" json:"name"`
	Floor          float64   `firestore:"floor" json:"floor"`
	Slug           string    `firestore:"slug" json:"slug"`
	SevenDayVolume float64   `firestore:"7d" json:"7d"`
	Updated        time.Time `firestore:"updated" json:"updated"`
}

type CollectionResp struct {
	Name     string    `json:"name"`
	Floor    float64   `json:"floor"`
	Slug     string    `json:"slug"`
	Thumb    string    `json:"thumb"`
	NumOwned int       `json:"numOwned"`
	Updated  time.Time `json:"updated"`
	NFTs     []NFT     `json:"nfts"`
}

// GetInfoRespV2 is the response for the GET /v2/info endpoint
type GetInfoRespV2 struct {
	Collections []CollectionResp `json:"collections"`
	TotalETH    float64          `json:"totalETH"`
	ETHPrice    float64          `json:"ethPrice"`
	ENSName     string           `json:"ensName"`
	Photo       string           `json:"photo"`
}

// getInfoV2 is the route handler for the GET /v2/info endpoint
func (h *Handler) getInfoV2(w http.ResponseWriter, r *http.Request) {
	var (
		err     error
		req     InfoReq
		address = req.Address
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
	if !eth.IsHexAddress(address) {
		// Fetch address from ENS if it's not a valid address
		address = h.GetAddressFromENSName(req.Address)
		if address == "" {
			http.Error(w, "you must include a valid ETH address in the request", http.StatusBadRequest)
			return
		}
	}

	var (
		ctx                = context.TODO()
		collections        = make([]opensea.OpenSeaCollectionCollection, 0)
		nfts               = make([]opensea.OpenSeaAsset, 0)
		nftsChan           = make(chan []opensea.OpenSeaAsset)
		collectionSlugDocs = make([]*firestore.DocumentRef, 0)
		ethPrice           float64
		collectionsChan    = make(chan []opensea.OpenSeaCollectionCollection)
		ethPriceChan       = make(chan float64)
		ensNameChan        = make(chan string)
		resp               = GetInfoRespV2{}
		totalETH           float64
	)

	// Fetch the user's collections & NFTs from OpenSea
	go h.asyncGetOpenSeaCollections(address, w, collectionsChan)
	collections = <-collectionsChan

	go h.asyncGetOpenSeaAssets(address, w, nftsChan)
	nfts = <-nftsChan
	resp.Photo = getPhoto(nfts)

	// Get ETH price
	go h.asyncGetETHPrice(ethPriceChan)
	ethPrice = <-ethPriceChan

	// Get ENS Name
	go h.asyncGetENSNameFromAddress(address, ensNameChan)
	resp.ENSName = <-ensNameChan

	var slugToOSCollectionMap = make(map[string]opensea.OpenSeaCollectionCollection)
	for _, collection := range collections {
		collectionSlugDocs = append(collectionSlugDocs, h.database.Collection("collections").Doc(collection.Slug))
		slugToOSCollectionMap[collection.Slug] = collection
	}

	// Check if the user's collections are in our database
	docsnaps, err := h.database.GetAll(ctx, collectionSlugDocs)
	if err != nil {
		h.logger.Error(err)
		return
	}

	var docSnapMap = make(map[string]CollectionV2)
	var collectionRespMap = make(map[string]CollectionResp)
	for _, ds := range docsnaps {
		if ds.Exists() {
			numOwned := slugToOSCollectionMap[ds.Ref.ID].OwnedAssetCount
			floor := ds.Data()["floor"].(float64)
			// This is for the response
			collectionRespMap[ds.Ref.ID] = CollectionResp{
				Name:     ds.Data()["name"].(string),
				Floor:    floor,
				Slug:     ds.Ref.ID,
				Updated:  ds.Data()["updated"].(time.Time),
				Thumb:    slugToOSCollectionMap[ds.Ref.ID].ImageURL,
				NumOwned: numOwned,
				NFTs:     h.getNFTsForCollection(ds.Ref.ID, nfts),
			}
			// This is for Firestore
			docSnapMap[ds.Ref.ID] = CollectionV2{
				Floor:   floor,
				Name:    ds.Data()["name"].(string),
				Slug:    ds.Ref.ID,
				Updated: ds.Data()["updated"].(time.Time),
			}

			totalETH += utils.RoundFloat(float64(numOwned)*floor, 4)
		}
	}

	var slugsToAdd = make([]string, 0)
	for _, collection := range collections {
		// Check docSnapMap to see if collection slug is in there
		if _, ok := docSnapMap[collection.Slug]; ok {
			resp.Collections = append(resp.Collections, collectionRespMap[collection.Slug])
		} else {
			// Otherwise, add it to the database with floor -1
			var c = CollectionV2{
				Name:    collection.Name,
				Slug:    collection.Slug,
				Floor:   -1,
				Updated: time.Now(),
			}
			go h.addCollectionToDB(ctx, collection, c)
			// TODO: Save to BQ
			resp.Collections = append(resp.Collections, collectionRespMap[collection.Slug])
			slugsToAdd = append(slugsToAdd, collection.Slug)
		}
	}

	// Post to Discord
	h.bot.ChannelMessageSendEmbed(
		"920371422457659482",
		&discordgo.MessageEmbed{
			Title:       fmt.Sprintf("Added %d new Collections", len(slugsToAdd)),
			Description: fmt.Sprintf("Wallet %s joined the party", address),
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   "Slugs",
					Value:  strings.Join(slugsToAdd, ", "),
					Inline: true,
				},
			},
		},
	)

	if !req.SkipBQ {
		bigquery.RecordRequestInBigQuery(
			h.bq.DatasetInProject("floor-report-327113", "info"),
			h.logger,
			address,
		)
	}

	sort.Slice(resp.Collections[:], func(i, j int) bool {
		return resp.Collections[i].Floor > resp.Collections[j].Floor
	})

	resp.ETHPrice = ethPrice

	resp.TotalETH = totalETH

	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) addCollectionToDB(ctx context.Context, collection opensea.OpenSeaCollectionCollection, c CollectionV2) {
	// Add collection to db
	c.Updated = time.Now()

	// Get stats
	stat := h.getOpenSeaStats(collection.Slug)
	c.Floor = stat.FloorPrice
	c.SevenDayVolume = stat.SevenDayVolume
	_, err := h.database.Collection("collections").Doc(collection.Slug).Set(ctx, c)
	if err != nil {
		h.logger.Error(err)
		return
	}

	h.logger.Infof("Added collection %s to db", collection.Slug)
}

// getOpenSeaStats gets the floor price from collections on OpenSea
func (h *Handler) getOpenSeaStats(docID string) opensea.OpenSeaCollectionStat {
	stat, err := h.os.GetCollectionStatsForSlug(docID)
	if err != nil {
		h.logger.Error(err)
	}
	return stat
}

func (h *Handler) getNFTsForCollection(slug string, nfts []opensea.OpenSeaAsset) []NFT {
	var result []NFT
	for _, nft := range nfts {
		if nft.Collection.Slug == slug {
			result = append(result, NFT{
				Name:     nft.Name,
				TokenID:  nft.TokenID,
				ImageURL: nft.ImageThumbnailURL,
				Traits:   getNFTTraits(nft),
			})
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
func (h *Handler) asyncGetOpenSeaCollectionStats(collections []opensea.OpenSeaCollectionCollection, w http.ResponseWriter, rc chan []CollectionStat) {
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
func (h *Handler) asyncGetOpenSeaCollections(address string, w http.ResponseWriter, rc chan []opensea.OpenSeaCollectionCollection) {
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
func (h *Handler) asyncGetETHPrice(rc chan float64) {
	rc <- h.cs.GetETHPrice()
}

func (h *Handler) asyncGetENSNameFromAddress(address string, rc chan string) {
	domain, err := ens.ReverseResolve(h.infuraClient, common.HexToAddress(address))
	if err != nil {
		h.logger.Error(err)
		rc <- ""
	}

	rc <- domain
}

func (h *Handler) GetAddressFromENSName(ensName string) string {
	address, err := ens.Resolve(h.infuraClient, ensName)
	if err != nil {
		h.logger.Error(err)
		return ""
	}

	return address.Hex()
}
