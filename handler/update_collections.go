package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"cloud.google.com/go/firestore"
	os "github.com/mager/sweeper/opensea"
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
	ForceUpdate    bool           `json:"force_update"`
	StartAt        string         `json:"start_at"`
	Slug           string         `json:"slug"`
	Slugs          []string       `json:"slugs"`
}

type UpdateCollectionsResp struct {
	Queued bool `json:"queued"`
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

	if req.CollectionType == "" {
		if req.Slug == "" {
			http.Error(w, "Missing collection type or slug", http.StatusBadRequest)
			return
		}
		go h.Sweeper.UpdateCollection(req.Slug)
	} else {
		go h.updateCollectionsByType(req)
	}
	resp.Queued = true

	json.NewEncoder(w).Encode(resp)
}

// updateCollectionsByType updates the collections in the database based on a custom config
func (h *Handler) updateCollectionsByType(r UpdateCollectionsReq) UpdateCollectionsResp {
	// Fetch config
	c, found := UpdateCollectionsConfig[r.CollectionType]

	var resp = UpdateCollectionsResp{}
	if !found {
		h.Logger.Errorf("Invalid collection type: %s", r.CollectionType)
		return resp
	}

	var (
		collections = h.Database.Collection("collections")
		count       = 0
		iter        *firestore.DocumentIterator
	)

	// Unused for now
	if c.queryCond.path != "" {
		iter = collections.Where(c.queryCond.path, c.queryCond.op, c.queryCond.value).Documents(h.Context)
		// If it gets stuck, you can pick a collection to start at
	} else if r.StartAt != "" {
		h.Logger.Infow("Updating all collections starting with collection", "startAt", r.StartAt)
		iter = collections.OrderBy(firestore.DocumentID, firestore.Asc).StartAt(r.StartAt).Documents(h.Context)
		// Otherwise only update collections that haven't been updated in over 24 hours
	} else if r.ForceUpdate {
		h.Logger.Info("Force updating all collections")
		iter = collections.Documents(h.Context)
		// By default, update all collections that haven't been updated in over 24 hours
	} else {
		h.Logger.Info("Updating all collections that haven't been updated in 24 hours")
		updatedSince := time.Now().Add(-24 * time.Hour)
		iter = collections.Where("updated", "<", updatedSince).Documents(h.Context)
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

		h.Logger.Infow("Updating collection", "collection", doc.Ref.ID)
		updatedResp := h.Sweeper.UpdateCollection(doc.Ref.ID)

		// Sleep because OpenSea throttles requests
		time.Sleep(os.OpenSeaRateLimit)

		if updatedResp.Success {
			count++
		}
	}

	h.Logger.Infof("Updated %d collections", count)

	resp.Queued = true

	return resp
}
