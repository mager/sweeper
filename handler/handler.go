package handler

import (
	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	"github.com/bwmarrin/discordgo"
	"github.com/gorilla/mux"
	"github.com/mager/sweeper/coinstats"
	"github.com/mager/sweeper/config"
	"github.com/mager/sweeper/opensea"
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
	bot      *discordgo.Session
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
	bot *discordgo.Session,
) *Handler {
	h := Handler{logger, router, os, bq, cs, cfg, database, bot}
	h.registerRoutes()

	return &h
}

// RegisterRoutes registers all the routes for the route handler
func (h *Handler) registerRoutes() {
	h.router.HandleFunc("/info", h.getInfo).Methods("POST")
	h.router.HandleFunc("/v2/info", h.getInfoV2).Methods("POST")

	h.router.HandleFunc("/stats", h.getStats).Methods("GET")
}
