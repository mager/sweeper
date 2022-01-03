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

type Req struct {
	CollectionType CollectionType `json:"collection_type"`
	Slug           string         `json:"slug"`
}

type Resp struct {
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
	h.router.HandleFunc("/update", h.update).
		Methods("POST")
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var (
		req  Req
		resp = Resp{}
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
		resp.Success = h.updateCollections(req.CollectionType)
	}

	json.NewEncoder(w).Encode(resp)
}

// updateSingleCollection updates a single collection
func (h *Handler) updateSingleCollection(req Req, resp *Resp) bool {
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

// updateCollections updates the collections in the database based on a custom config
func (h *Handler) updateCollections(collectionType CollectionType) bool {
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
