package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/mager/sweeper/database"
	"google.golang.org/api/iterator"
)

type CollectionType string

var (
	CollectionTypeAll       CollectionType = "all"
	UpdateCollectionsConfig                = map[CollectionType]Config{
		CollectionTypeAll: {
			desc: "All collections",
			log:  "Updating all collections",
		},
	}
)

type UpdateCollectionsReq struct {
	CollectionType CollectionType `json:"collection_type"`
	Slug           string         `json:"slug"`
	Slugs          []string       `json:"slugs"`
}

type UpdateCollectionsResp struct {
	Success    bool                `json:"success"`
	Collection database.Collection `json:"collection"`
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
		resp.Collection, resp.Success = h.updateSingleCollection(req, &resp)
	} else {
		resp.Success = h.updateCollectionsByType(req.CollectionType)
	}

	json.NewEncoder(w).Encode(resp)
}

// updateSingleCollection updates a single collection
func (h *Handler) updateSingleCollection(req UpdateCollectionsReq, resp *UpdateCollectionsResp) (database.Collection, bool) {
	var (
		err        error
		floor      float64
		collection database.Collection
		updated    bool
		slug       = req.Slug
	)

	docsnap, err := h.Database.Collection("collections").Doc(slug).Get(h.Context)

	if err != nil {
		h.Logger.Errorw(
			"Error fetching collection from Firestore, trying to add collection",
			"err", err,
		)
		floor, updated := database.AddCollectionToDB(h.Context, h.OpenSea, h.Logger, h.Database, slug)
		h.Logger.Infow(
			"Collection added",
			"collection", slug,
			"floor", floor,
			"updated", updated,
		)

		if updated {
			// Fetch collection
			collection = database.GetCollection(h.Context, h.Logger, h.Database, req.Slug)
		}

		return collection, updated
	}

	if docsnap.Exists() {
		// Update collection
		h.Logger.Info("Collection found, updating")
		updated = database.UpdateCollectionStats(
			h.Context,
			h.Logger,
			h.OpenSea,
			h.BigQuery,
			docsnap,
		)
		h.Logger.Infow(
			"Collection updated",
			"collection", docsnap.Ref.ID,
			"floor", floor,
			"updated", updated,
		)
	}

	if updated {
		// Fetch collection
		collection = database.GetCollection(h.Context, h.Logger, h.Database, req.Slug)
	}

	return collection, updated
}

// updateCollectionsBySlugs updates a single collection
func (h *Handler) updateCollectionsBySlugs(req UpdateCollectionsReq, resp *UpdateCollectionsResp) bool {
	var (
		slugs              = req.Slugs
		updatedCollections int
	)

	for _, slug := range slugs {
		_, updated := database.AddCollectionToDB(h.Context, h.OpenSea, h.Logger, h.Database, slug)
		time.Sleep(time.Millisecond * 250)
		if updated {
			updatedCollections++
		}

	}

	h.Logger.Infow(
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
		h.Logger.Errorf("Invalid collection type: %s", collectionType)
		return false
	}

	var (
		collections = h.Database.Collection("collections")
		count       = 0
		iter        *firestore.DocumentIterator
	)

	if c.queryCond.path != "" {
		iter = collections.Where(c.queryCond.path, c.queryCond.op, c.queryCond.value).Documents(h.Context)
	} else {
		h.Logger.Info("Updating all collections")
		iter = collections.Documents(h.Context)
	}

	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
			h.Logger.Error(err)
		}

		// Update the floor price
		var updated bool
		updated = database.UpdateCollectionStats(
			h.Context,
			h.Logger,
			h.OpenSea,
			h.BigQuery,
			doc,
		)

		// Sleep because OpenSea throttles requests
		// Rate limit is 4 requests per second
		time.Sleep(time.Millisecond * 250)

		if updated {
			count++
		}
	}

	h.Logger.Infof("Updated %d collections", count)

	return true
}

func (h *Handler) logNoUpdate(doc *firestore.DocumentSnapshot, c Config) {
	h.Logger.Infow(
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
