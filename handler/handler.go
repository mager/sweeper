package handler

import (
	"context"
	"net/http"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	"github.com/gorilla/mux"
	"github.com/mager/sweeper/database"
	"github.com/mager/sweeper/opensea"
	"github.com/mager/sweeper/reservoir"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Handler struct for HTTP requests
type Handler struct {
	fx.In

	Context   context.Context
	Router    *mux.Router
	Logger    *zap.SugaredLogger
	OpenSea   opensea.OpenSeaClient
	Database  *firestore.Client
	BigQuery  *bigquery.Client
	Reservoir *reservoir.ReservoirClient
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
func New(h Handler) *Handler {
	h.registerRoutes()
	return &h
}

// RegisterRoutes registers all the routes for the route handler
func (h *Handler) registerRoutes() {
	// Update collections
	h.Router.HandleFunc("/update/collections", h.updateCollections).
		Methods("POST")
	// Update users
	h.Router.HandleFunc("/update/users", h.updateUsers).
		Methods("POST")
	h.Router.HandleFunc("/update/user", h.updateUser).
		Methods("POST")
	h.Router.HandleFunc("/health", h.health).
		Methods("GET")
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	h.Logger.Info("Health check")
	w.WriteHeader(http.StatusOK)
}

// asyncGetOpenSeaCollections gets the collections from OpenSea
func (h *Handler) asyncGetOpenSeaCollections(address string, rc chan []opensea.OpenSeaCollectionV2) {
	collections, err := h.OpenSea.GetAllCollectionsForAddressV2(address)

	if err != nil {
		h.Logger.Error(err)
		return
	}

	rc <- collections
}

// getOpenSeaAssets gets the assets for the given address
func (h *Handler) getOpenSeaAssets(address string) []opensea.OpenSeaAssetV2 {
	assets, err := h.OpenSea.GetAllAssetsForAddressV2(address)

	if err != nil {
		h.Logger.Error(err)
	}

	return assets
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
