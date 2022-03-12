package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/mager/sweeper/database"
	"google.golang.org/api/iterator"
)

type RenameUsersResp struct {
	Success bool `json:"success"`
}

func (h *Handler) renameUsers(w http.ResponseWriter, r *http.Request) {
	var (
		resp = RenameUsersResp{}
	)

	resp.Success = h.doRenameUsers()

	json.NewEncoder(w).Encode(resp)
}

// doUpdateUsers updates a collection of addresses
func (h *Handler) doRenameUsers() bool {
	var (
		ctx         = context.Background()
		collections = h.Database.Collection("users")
		iter        = collections.Documents(h.Context)
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

		updated, err := h.renameUser(ctx, doc)
		if err != nil {
			h.Logger.Error(err)
		}
		if updated {
			count++
		}

	}

	// Log the number of users updated
	h.Logger.Info("Renamed", count, "users")

	return true
}

func (h *Handler) renameUser(ctx context.Context, doc *firestore.DocumentSnapshot) (bool, error) {
	// Make a copy of the doc and rename it with a lowercased ID
	var (
		u database.User
	)

	err := doc.DataTo(&u)

	if err != nil {
		return false, err
	}

	lowercasedID := strings.ToLower(doc.Ref.ID)

	// Create a new document
	newDocRef := h.Database.Collection("users").Doc(lowercasedID)

	// Set all the fields from doc to newDocRef
	_, err = newDocRef.Set(ctx, doc.Data())

	if err != nil {
		return false, err
	}

	// Delete the old document
	_, err = doc.Ref.Delete(ctx)

	if err != nil {
		return false, err
	}

	return true, nil
}
