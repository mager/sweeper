package handler

import (
	"encoding/json"
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

	h.Logger.Infow("Updating collections", "collection_type", req.CollectionType, "slug", req.Slug)

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
		// resp.Collection, resp.Success = h.updateSingleCollection(req, &resp)
		updateResp := h.Sweeper.UpdateCollection(req.Slug)
		resp.Success = updateResp.Success
		resp.Collection = updateResp.Collection
	} else {
		resp.Success = h.updateCollectionsByType(req.CollectionType)
	}

	json.NewEncoder(w).Encode(resp)
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
		// updated := database.UpdateCollectionStats(
		// 	h.Context,
		// 	h.Logger,
		// 	h.OpenSea,
		// 	h.BigQuery,
		// 	h.NFTStats,
		// 	h.Reservoir,
		// 	doc,
		// )
		h.Logger.Info("Updating collection", "collection", doc.Ref.ID)
		updatedResp := h.Sweeper.UpdateCollection(doc.Ref.ID)

		// Sleep because OpenSea throttles requests
		// Rate limit is 4 requests per second
		time.Sleep(time.Millisecond * 250)

		if updatedResp.Success {
			count++
		}
	}

	h.Logger.Infof("Updated %d collections", count)

	return true
}
