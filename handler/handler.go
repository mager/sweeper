package handler

import (
	"context"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	"github.com/bwmarrin/discordgo"
	"github.com/gorilla/mux"
	"github.com/mager/sweeper/database"
	"github.com/mager/sweeper/opensea"
	"go.uber.org/zap"
)

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
	h.router.HandleFunc("/update/user", h.updateUser).
		Methods("POST")
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
				// TODO: Add traits
			})
		}
	}
	return result
}
