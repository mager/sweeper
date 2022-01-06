package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"cloud.google.com/go/firestore"
	"github.com/mager/sweeper/database"
	"github.com/mager/sweeper/opensea"
	"google.golang.org/api/iterator"
)

type UserType string

var (
	UserTypeAll       UserType = "all"
	UpdateUsersConfig          = map[UserType]Config{
		UserTypeAll: {
			desc: "All users",
			log:  "Updating all users",
		},
	}
)

type UpdateUsersReq struct {
	UserType UserType `json:"user_type"`
}

type UpdateUsersResp struct {
	Success bool `json:"success"`
}

func (h *Handler) updateUsers(w http.ResponseWriter, r *http.Request) {
	var (
		resp = UpdateUsersResp{}
	)

	// if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
	// 	http.Error(w, err.Error(), http.StatusBadRequest)
	// 	return
	// }

	resp.Success = h.updateAddresses()

	json.NewEncoder(w).Encode(resp)
}

// updateAddresses updates a single collection
func (h *Handler) updateAddresses() bool {
	var (
		ctx                    = context.Background()
		collections            = h.database.Collection("users").Where("shouldIndex", "==", true)
		iter                   = collections.Documents(h.ctx)
		u                      database.User
		openseaCollections     = make([]opensea.OpenSeaCollectionV2, 0)
		openseaAssets          = make([]opensea.OpenSeaAssetV2, 0)
		openseaAssetsChan      = make(chan []opensea.OpenSeaAssetV2)
		openseaCollectionsChan = make(chan []opensea.OpenSeaCollectionV2)
	)

	// Fetch users from Firestore
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
		}

		err = doc.DataTo(&u)
		if err != nil {
			h.logger.Error(err)
		}

		var (
			address = doc.Ref.ID
			wallet  = database.Wallet{}
		)

		// Fetch the user's collections & NFTs from OpenSea
		go h.asyncGetOpenSeaCollections(address, openseaCollectionsChan)
		openseaCollections = <-openseaCollectionsChan
		go h.asyncGetOpenSeaAssets(address, openseaAssetsChan)
		openseaAssets = <-openseaAssetsChan

		for _, collection := range openseaCollections {
			wallet.Collections = append(wallet.Collections, database.WalletCollection{
				Name:     collection.Name,
				Slug:     collection.Slug,
				ImageURL: collection.ImageURL,
				NFTs:     h.getNFTsForCollection(collection.Slug, openseaAssets),
				// TODO: Hydrate collection stats
			})
		}

		// Update collections
		wr, err := doc.Ref.Update(ctx, []firestore.Update{
			{Path: "wallet", Value: wallet},
		})

		if err != nil {
			h.logger.Error(err)
		}

		h.logger.Infow(
			"Address updated",
			"address", doc.Ref.ID,
			"updated", wr.UpdateTime,
		)
	}

	return true
}
