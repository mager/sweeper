package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/kr/pretty"
	"github.com/mager/sweeper/database"
	"google.golang.org/api/iterator"
)

type DeleteCollectionsResp struct {
	Success bool `json:"success"`
}

func (h *Handler) deleteCollections(w http.ResponseWriter, r *http.Request) {
	var (
		resp = DeleteCollectionsResp{}
	)

	resp.Success = h.doDeleteCollections()

	json.NewEncoder(w).Encode(resp)
}

// doDeleteCollections deletes collections
func (h *Handler) doDeleteCollections() bool {
	var (
		ctx         = context.Background()
		collections = h.Database.Collection("collections").Where("floor", "==", 0)
		iter        = collections.Documents(h.Context)
		c           = make([]database.Collection, 0)
		count       = 0
	)

	// Fetch users from Firestore
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			h.Logger.Error(err)
		}

		var (
			collection = database.Collection{}
		)

		if err := doc.DataTo(&collection); err != nil {
			break
		}

		updated, err := h.deleteCollection(ctx, doc)
		if err != nil {
			h.Logger.Error(err)
		}
		if updated {
			count++
		}
	}

	pretty.Print(len(c))

	// Log the number of collections updated
	h.Logger.Info("Deleted", count, "collections")

	return true
}

func (h *Handler) deleteCollection(ctx context.Context, doc *firestore.DocumentSnapshot) (bool, error) {
	var (
		user = database.User{}
	)

	if err := doc.DataTo(&user); err != nil {
		return false, err
	}

	if _, err := doc.Ref.Delete(ctx); err != nil {
		return false, err
	}

	h.Logger.Infow("Deleted collection", "slug", doc.Ref.ID)
	time.Sleep(time.Millisecond * 100)

	return true, nil
}
