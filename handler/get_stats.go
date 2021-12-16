package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

type StatsResp struct {
	Total   int       `json:"total"`
	Updated time.Time `json:"updated"`
}

// getStats is the route handler for the GET /stats endpoint
func (h *Handler) getStats(w http.ResponseWriter, r *http.Request) {
	var (
		ctx         = context.TODO()
		resp        = StatsResp{}
		docs        = make([]*firestore.DocumentRef, 0)
		collections = h.database.Collection("collections")
		updated     = time.Time{}
	)

	// Fetch all collections
	iter := collections.Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			h.logger.Errorf("Error fetching collections: %v", err)
			break
		}
		docs = append(docs, doc.Ref)
		if doc.Data()["updated"].(time.Time).After(updated) {
			updated = doc.Data()["updated"].(time.Time)
		}
	}

	// Set total
	resp.Total = len(docs)

	// Set last updated
	resp.Updated = updated

	json.NewEncoder(w).Encode(resp)
}
