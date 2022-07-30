package handler

import (
	"context"
	"net/http"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
	"github.com/gorilla/mux"
	"github.com/mager/go-opensea/opensea"
	"github.com/mager/sweeper/etherscan"
	"github.com/mager/sweeper/nftstats"
	"github.com/mager/sweeper/reservoir"
	"github.com/mager/sweeper/sweeper"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Handler struct for HTTP requests
type Handler struct {
	fx.In

	BigQuery  *bigquery.Client
	Context   context.Context
	Database  *firestore.Client
	Etherscan *etherscan.EtherscanClient
	Logger    *zap.SugaredLogger
	NFTStats  *nftstats.NFTStatsClient
	OpenSea   *opensea.OpenSeaClient
	Reservoir *reservoir.ReservoirClient
	Router    *mux.Router
	Storage   *storage.Client
	Sweeper   *sweeper.SweeperClient
}

type Condition struct {
	path, op string
	value    interface{}
}

type Config struct {
	desc      string
	queryCond Condition
	log       string
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
	h.Router.HandleFunc("/update/user/metadata", h.updateUserMetadata).
		Methods("POST")
	h.Router.HandleFunc("/update/stats", h.updateStats).
		Methods("POST")
	h.Router.HandleFunc("/update/random_nft", h.updateRandomNFT).
		Methods("POST")
	h.Router.HandleFunc("/health", h.health).
		Methods("GET")

	h.Router.HandleFunc("/update/contract/{slug}", h.updateContract).
		Methods("POST")

	h.Router.HandleFunc("/rename/users", h.renameUsers).
		Methods("POST")
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	h.Logger.Info("Health check")
	w.WriteHeader(http.StatusOK)
}

// getOpenSeaAssets gets the assets for the given address
func (h *Handler) getOpenSeaAssets(address string) []opensea.Asset {
	assets, err := h.OpenSea.GetAssets(address)

	if err != nil {
		h.Logger.Error(err)
	}

	return assets
}
