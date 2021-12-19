package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/bwmarrin/discordgo"
	eth "github.com/ethereum/go-ethereum/common"
	"github.com/mager/sweeper/opensea"
	"github.com/mager/sweeper/utils"
)

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
}

// GetInfoRespV2 is the response for the GET /v2/info endpoint
type GetInfoRespV2 struct {
	Collections []CollectionResp `json:"collections"`
	TotalETH    float64          `json:"totalETH"`
	ETHPrice    float64          `json:"ethPrice"`
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
		collections = make([]opensea.OpenSeaCollectionCollection, 0)
		// nfts               = make([]opensea.OpenSeaAsset, 0)
		// nftsChan           = make(chan []opensea.OpenSeaAsset)
		collectionSlugDocs = make([]*firestore.DocumentRef, 0)
		ethPrice           float64
		collectionsChan    = make(chan []opensea.OpenSeaCollectionCollection)
		ethPriceChan       = make(chan float64)
		resp               = GetInfoRespV2{}
		totalETH           float64
	)

	// Fetch the user's collections & NFTs from OpenSea
	go h.asyncGetOpenSeaCollections(req.Address, w, collectionsChan)
	collections = <-collectionsChan

	// go h.asyncGetOpenSeaAssets(req.Address, w, nftsChan)
	// nfts = <-nftsChan

	// Get ETH price
	go h.asyncGetETHPrice(w, ethPriceChan)
	ethPrice = <-ethPriceChan

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
			Description: fmt.Sprintf("Wallet %s joined the party", req.Address),
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
		h.recordRequestInBigQuery(req.Address)
	}

	sort.Slice(resp.Collections[:], func(i, j int) bool {
		return resp.Collections[i].Floor > resp.Collections[j].Floor
	})

	resp.ETHPrice = ethPrice

	resp.TotalETH = totalETH

	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) getNFTsV2(collection opensea.OpenSeaCollectionCollection, assets []opensea.OpenSeaAsset) []NFT {
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
