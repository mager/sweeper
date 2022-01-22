package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/bwmarrin/discordgo"
	"github.com/mager/sweeper/database"
	"google.golang.org/api/iterator"
)

type CollectionType string

var (
	CollectionTypeNew       CollectionType = "new"
	CollectionTypeAll       CollectionType = "all"
	CollectionTypeTierA     CollectionType = "a"
	CollectionTypeTierB     CollectionType = "b"
	UpdateCollectionsConfig                = map[CollectionType]Config{
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
				value: time.Now().Add(-4 * time.Hour),
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

type UpdateCollectionsReq struct {
	CollectionType CollectionType `json:"collection_type"`
	Slug           string         `json:"slug"`
	Slugs          []string       `json:"slugs"`
}

type UpdateCollectionsResp struct {
	Success bool `json:"success"`
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

	// Update multiple collections by slug
	if len(req.Slugs) > 0 {
		resp.Success = h.updateCollectionsBySlugs(req, &resp)
		json.NewEncoder(w).Encode(resp)
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

// updateCollectionsBySlugs updates a single collection
func (h *Handler) updateCollectionsBySlugs(req UpdateCollectionsReq, resp *UpdateCollectionsResp) bool {
	var (
		slugs              = req.Slugs
		updatedCollections int
	)

	for _, slug := range slugs {
		_, updated := database.AddCollectionToDB(h.ctx, &h.os, h.logger, h.database, slug)
		time.Sleep(time.Millisecond * 250)
		if updated {
			updatedCollections++
		}

	}

	h.logger.Infow(
		"Collections updated",
		"slugs", slugs,
	)

	return len(slugs) == updatedCollections
}

// updateCollectionsByType updates the collections in the database based on a custom config
func (h *Handler) updateCollectionsByType(collectionType CollectionType) bool {
	// Fetch config
	c, found := UpdateCollectionsConfig[collectionType]

	if !found {
		h.logger.Errorf("Invalid collection type: %s", collectionType)
		return false
	}

	h.logger.Info(c.log)

	var (
		collections = h.database.Collection("collections")
		count       = 0
		slugs       = make([]string, 0)
		iter        *firestore.DocumentIterator
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
