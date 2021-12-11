package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"cloud.google.com/go/firestore"
	eth "github.com/ethereum/go-ethereum/common"
	"github.com/mager/sweeper/opensea"
)

type CollectionV2 struct {
	Name    string    `firestore:"name" json:"name"`
	Floor   float64   `firestore:"floor" json:"floor"`
	Slug    string    `firestore:"slug" json:"slug"`
	Updated time.Time `firestore:"updated" json:"updated"`
}

// GetInfoRespV2 is the response for the GET /v2/info endpoint
type GetInfoRespV2 struct {
	Collections []CollectionV2 `json:"collections"`
	ETHPrice    float64        `json:"ethPrice"`
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
		// nfts               = make([]opensea.OpenSeaAsset, 0)
		collectionSlugDocs = make([]*firestore.DocumentRef, 0)
		ethPrice           float64
		collectionsChan    = make(chan []opensea.OpenSeaCollection)
		ethPriceChan       = make(chan float64)
		resp               = GetInfoRespV2{}
		// nftsChan           = make(chan []opensea.OpenSeaAsset)
	)

	// Fetch the user's collections & NFTs from OpenSea
	go h.asyncGetOpenSeaCollections(req.Address, w, collectionsChan)
	collections = <-collectionsChan

	// go h.asyncGetOpenSeaAssets(req.Address, w, nftsChan)
	// nfts = <-nftsChan

	// Get ETH price
	go h.asyncGetETHPrice(w, ethPriceChan)
	ethPrice = <-ethPriceChan

	for _, collection := range collections {
		collectionSlugDocs = append(collectionSlugDocs, h.database.Collection("collections").Doc(collection.Slug))
	}

	// Check if the user's collections are in our database
	docsnaps, err := h.database.GetAll(ctx, collectionSlugDocs)
	if err != nil {
		h.logger.Error(err)
		return
	}

	var docSnapMap = make(map[string]CollectionV2)
	for _, ds := range docsnaps {
		if ds.Exists() {
			docSnapMap[ds.Ref.ID] = CollectionV2{
				Floor:   ds.Data()["floor"].(float64),
				Name:    ds.Data()["name"].(string),
				Slug:    ds.Ref.ID,
				Updated: ds.Data()["updated"].(time.Time),
			}
		}
	}

	for _, collection := range collections {
		// Check docSnapMap to see if collection slug is in there
		if _, ok := docSnapMap[collection.Slug]; ok {
			resp.Collections = append(resp.Collections, docSnapMap[collection.Slug])
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
			resp.Collections = append(resp.Collections, c)
		}
	}
	// if !req.SkipBQ {
	// 	h.recordRequestInBigQuery(req.Address, len(nfts), adaptedCollections, unrealizedBag)
	// }

	sort.Slice(resp.Collections[:], func(i, j int) bool {
		return resp.Collections[i].Floor > resp.Collections[j].Floor
	})

	resp.ETHPrice = ethPrice

	json.NewEncoder(w).Encode(resp)
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

func (h *Handler) addCollectionToDB(ctx context.Context, collection opensea.OpenSeaCollection, c CollectionV2) {
	h.logger.Infof("collection slug %s not found in docSnapMap", collection.Slug)
	// Add collection to db
	c.Updated = time.Now()
	_, err := h.database.Collection("collections").Doc(collection.Slug).Set(ctx, c)
	if err != nil {
		h.logger.Error(err)
		return
	}
	h.logger.Infof("added collection %s to db", collection.Slug)
}
