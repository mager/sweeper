package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	"github.com/bwmarrin/discordgo"
	"github.com/gorilla/mux"
	"github.com/mager/sweeper/database"
	"github.com/mager/sweeper/opensea"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
)

var (
	CollectionTypeNew   CollectionType = "new"
	CollectionTypeAll   CollectionType = "all"
	CollectionTypeTierA CollectionType = "a"
	CollectionTypeTierB CollectionType = "b"
	UpdateConfig                       = map[CollectionType]Config{
		CollectionTypeAll: {
			desc: "All collections",
			log:  "Updating all collections",
		},
		CollectionTypeNew: {
			desc: "Collections added in the last 4 hours",
			log:  "Updating new collections",
			queryCond: Condition{
				path:  "floor",
				op:    "==",
				value: -1,
			},
		},
		CollectionTypeTierA: {
			desc: "7 day volume is over 0.5 ETH",
			log:  "Updating Tier A collections (7 day volume is over 0.5 ETH)",
			queryCond: Condition{
				path:  "updated",
				op:    "<",
				value: time.Now().Add(-2 * time.Hour),
			},
			updateCond: Condition{
				path:  "7d",
				op:    ">",
				value: 0.5,
			},
		},
		CollectionTypeTierB: {
			desc: "7 day volume is under 0.6 ETH",
			log:  "Updating Tier B collections (7 day volume is under 0.6 ETH)",
			queryCond: Condition{
				path:  "updated",
				op:    "<",
				value: time.Now().Add(-3 * 24 * time.Hour),
			},
			updateCond: Condition{
				path:  "7d",
				op:    "<",
				value: 0.6,
			},
		},
	}
)

type CollectionType string

type UpdateCollectionsReq struct {
	CollectionType CollectionType `json:"collection_type"`
	Slug           string         `json:"slug"`
}

type UpdateCollectionsResp struct {
	Success bool `json:"success"`
}

type UpdateUsersResp struct {
	Success bool `json:"success"`
}

type Condition struct {
	path, op string
	value    interface{}
}

type Config struct {
	desc       string
	queryCond  Condition
	updateCond Condition
	log        string
}

// Handler struct for HTTP requests
type Handler struct {
	ctx      context.Context
	router   *mux.Router
	logger   *zap.SugaredLogger
	os       opensea.OpenSeaClient
	database *firestore.Client
	bq       *bigquery.Client
	bot      *discordgo.Session
}

// New creates a Handler struct
func New(
	ctx context.Context,
	router *mux.Router,
	logger *zap.SugaredLogger,
	os opensea.OpenSeaClient,
	database *firestore.Client,
	bq *bigquery.Client,
	bot *discordgo.Session,
) *Handler {
	h := Handler{ctx, router, logger, os, database, bq, bot}
	h.registerRoutes()
	return &h
}

// RegisterRoutes registers all the routes for the route handler
func (h *Handler) registerRoutes() {
	// Update collections
	h.router.HandleFunc("/update/collections", h.updateCollections).
		Methods("POST")
	// Update users
	h.router.HandleFunc("/update/users", h.updateUsers).
		Methods("POST")
}

func (h *Handler) updateCollections(w http.ResponseWriter, r *http.Request) {
	var (
		req  UpdateCollectionsReq
		resp = UpdateCollectionsResp{}
	)

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.CollectionType == "" {
		if req.Slug == "" {
			http.Error(w, "Missing collection type or slug", http.StatusBadRequest)
			return
		}
		resp.Success = h.updateSingleCollection(req, &resp)
	} else {
		resp.Success = h.updateCollectionsByType(req.CollectionType)
	}

	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) updateUsers(w http.ResponseWriter, r *http.Request) {
	var (
		// req  UpdateCollectionsReq
		resp = UpdateCollectionsResp{}
	)

	// if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
	// 	http.Error(w, err.Error(), http.StatusBadRequest)
	// 	return
	// }

	resp.Success = h.updateAddresses()

	json.NewEncoder(w).Encode(resp)
}

// updateSingleCollection updates a single collection
func (h *Handler) updateSingleCollection(req UpdateCollectionsReq, resp *UpdateCollectionsResp) bool {
	var (
		collections = h.database.Collection("collections")
		iter        = collections.Where("slug", "==", req.Slug).Documents(h.ctx)
		err         error
		floor       float64
		updated     bool
	)

	// Fetch collection from Firestore
	doc, err := iter.Next()
	if err != nil {
		h.logger.Errorw(
			"Error fetching collection from Firestore",
			"err", err,
		)

		// Add the collection if it doesn't exist
		if err == iterator.Done {
			floor, updated = database.AddCollectionToDB(h.ctx, &h.os, h.logger, h.database, req.Slug)

			h.logger.Infow(
				"Collection added",
				"collection", req.Slug,
				"floor", floor,
				"updated", updated,
			)

			return updated
		}

		return updated
	}

	// Update collection
	floor, updated = database.UpdateCollectionStats(h.ctx, &h.os, h.bq, h.logger, doc)

	h.logger.Infow(
		"Collection updated",
		"collection", doc.Ref.ID,
		"floor", floor,
		"updated", updated,
	)

	return updated
}

// updateCollectionsByType updates the collections in the database based on a custom config
func (h *Handler) updateCollectionsByType(collectionType CollectionType) bool {
	// Fetch config
	c, found := UpdateConfig[collectionType]

	if !found {
		h.logger.Errorf("Invalid collection type: %s", collectionType)
		return false
	}

	h.logger.Info(c.log)

	var (
		collections = h.database.Collection("collections")
		iter        = collections.Documents(h.ctx)
		count       = 0
		slugs       = make([]string, 0)
	)

	if c.queryCond.path != "" {
		iter = collections.Where(c.queryCond.path, c.queryCond.op, c.queryCond.value).Documents(h.ctx)
	} else {
		iter = collections.Documents(h.ctx)
	}

	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
			h.logger.Error(err)
		}

		// Update the floor price
		var updated bool
		if c.updateCond.path != "" {
			if c.updateCond.op == "<" {
				if doc.Data()[c.updateCond.path].(float64) < c.updateCond.value.(float64) {
					_, updated = database.UpdateCollectionStats(
						h.ctx,
						&h.os,
						h.bq,
						h.logger,
						doc,
					)
				} else {
					h.logNoUpdate(doc, c)
				}
			} else if c.updateCond.op == ">" {
				if doc.Data()[c.updateCond.path].(float64) > c.updateCond.value.(float64) {
					_, updated = database.UpdateCollectionStats(
						h.ctx,
						&h.os,
						h.bq,
						h.logger,
						doc,
					)
				} else {
					h.logNoUpdate(doc, c)
				}
			}
		} else {
			_, updated = database.UpdateCollectionStats(
				h.ctx,
				&h.os,
				h.bq,
				h.logger,
				doc,
			)
		}

		// Sleep because OpenSea throttles requests
		// Rate limit is 4 requests per second
		time.Sleep(time.Millisecond * 250)

		if updated {
			count++
			slugs = append(slugs, doc.Ref.ID)
		}
	}

	// Post to Discord
	if count > 0 {
		h.bot.ChannelMessageSendEmbed(
			"920371422457659482",
			&discordgo.MessageEmbed{
				Title:       fmt.Sprintf("Updated %d collections", count),
				Description: c.desc,
				Fields: []*discordgo.MessageEmbedField{
					{
						Name:   "Slugs",
						Value:  strings.Join(slugs, ", "),
						Inline: true,
					},
				},
			},
		)
	}

	h.logger.Infof("Updated %d collections", count)

	return true
}

func (h *Handler) logNoUpdate(doc *firestore.DocumentSnapshot, c Config) {
	h.logger.Infow(
		"Floor not updated",
		"collection", doc.Ref.ID,
		"cond", fmt.Sprintf(
			"%s %.2f %s %.2f",
			c.updateCond.path,
			doc.Data()[c.updateCond.path].(float64),
			c.updateCond.op,
			c.updateCond.value,
		),
	)
}

// updateAddresses updates a single collection
func (h *Handler) updateAddresses() bool {
	var (
		ctx                    = context.Background()
		collections            = h.database.Collection("users")
		iter                   = collections.Where("isWhale", "==", true).Limit(1).Documents(h.ctx)
		u                      database.User
		openseaCollections     = make([]opensea.OpenSeaCollectionV2, 0)
		openseaAssets          = make([]opensea.OpenSeaAssetV2, 0)
		openseaAssetsChan      = make(chan []opensea.OpenSeaAssetV2)
		openseaCollectionsChan = make(chan []opensea.OpenSeaCollectionV2)
	)

	// Fetch collection from Firestore
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
		}

		err = doc.DataTo(&u)
		if err != nil {
			h.logger.Error(err)
		}

		var (
			address = doc.Ref.ID
			wallet  = database.Wallet{}
		)

		// Fetch the user's collections & NFTs from OpenSea
		go h.asyncGetOpenSeaCollections(address, openseaCollectionsChan)
		openseaCollections = <-openseaCollectionsChan
		go h.asyncGetOpenSeaAssets(address, openseaAssetsChan)
		openseaAssets = <-openseaAssetsChan

		for _, collection := range openseaCollections {

			wallet.Collections = append(wallet.Collections, database.WalletCollection{
				Name:     collection.Name,
				Slug:     collection.Slug,
				ImageURL: collection.ImageURL,
				NFTs:     h.getNFTsForCollection(collection.Slug, openseaAssets),
				// TODO: Hydrate collection stats
			})
		}

		// Update collections
		wr, err := doc.Ref.Update(ctx, []firestore.Update{
			{Path: "wallet", Value: wallet},
		})

		if err != nil {
			h.logger.Error(err)
		}

		h.logger.Infow(
			"Address updated",
			"address", doc.Ref.ID,
			"updated", wr.UpdateTime,
		)
	}

	return true
}

// asyncGetOpenSeaCollections gets the collections from OpenSea
func (h *Handler) asyncGetOpenSeaCollections(address string, rc chan []opensea.OpenSeaCollectionV2) {
	collections, err := h.os.GetAllCollectionsForAddressV2(address)

	if err != nil {
		h.logger.Error(err)
		return
	}

	rc <- collections
}

// asyncGetOpenSeaAssets gets the assets for the given address
func (h *Handler) asyncGetOpenSeaAssets(address string, rc chan []opensea.OpenSeaAssetV2) {
	assets, err := h.os.GetAllAssetsForAddressV2(address)

	if err != nil {
		h.logger.Error(err)
		return
	}

	rc <- assets
}

func (h *Handler) getNFTsForCollection(collectionSlug string, assets []opensea.OpenSeaAssetV2) []database.WalletAsset {
	var result []database.WalletAsset
	for _, asset := range assets {
		if asset.Collection.Slug == collectionSlug {
			result = append(result, database.WalletAsset{
				Name:     asset.Name,
				TokenID:  asset.TokenID,
				ImageURL: asset.ImageURL,
			})
		}
	}
	return result
}
