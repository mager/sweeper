package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

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
	DryRun   bool     `json:"dry_run"`
}

type UpdateUsersResp struct {
	Success bool `json:"success"`
}

func (h *Handler) updateUsers(w http.ResponseWriter, r *http.Request) {
	var (
		req  UpdateUsersReq
		resp = UpdateUsersResp{}
	)

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp.Success = h.doUpdateAddresses()

	json.NewEncoder(w).Encode(resp)
}

// doUpdateAddresses updates a collection of addresses
func (h *Handler) doUpdateAddresses() bool {
	var (
		ctx         = context.Background()
		collections = h.database.Collection("users").Where("shouldIndex", "==", true)
		iter        = collections.Documents(h.ctx)
		u           database.User
		count       = 0
	)

	// Fetch users from Firestore
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			h.logger.Error(err)
		}

		err = doc.DataTo(&u)
		if err != nil {
			h.logger.Error(err)
		}

		updated := h.updateSingleAddressV2(ctx, doc)
		if updated {
			count++
		}
	}

	// Post to Discord
	// if !dryRun && count > 0 {
	// 	h.bot.ChannelMessageSendEmbed(
	// 		"920371422457659482",
	// 		&discordgo.MessageEmbed{
	// 			Title: fmt.Sprintf("Updated %d wallets", count),
	// 		},
	// 	)
	// }

	h.logger.Infof("Updated %d addresses", count)

	return true
}

func (h *Handler) updateSingleAddressV2(ctx context.Context, doc *firestore.DocumentSnapshot) bool {

	var (
		address        = doc.Ref.ID
		openseaAssets  = make([]opensea.OpenSeaAssetV2, 0)
		collectionsMap = make(map[string]database.WalletCollection)
	)

	// Fetch the user's collections & NFTs from OpenSea
	openseaAssets = h.getOpenSeaAssets(address)

	// Create a list of wallet collections
	for _, asset := range openseaAssets {
		// If we do have a collection for this asset, add to it
		if _, ok := collectionsMap[asset.Collection.Slug]; ok {
			w := collectionsMap[asset.Collection.Slug]
			w.NFTs = append(w.NFTs, database.WalletAsset{
				Name:     asset.Name,
				ImageURL: asset.ImageURL,
				TokenID:  asset.TokenID,
			})
			collectionsMap[asset.Collection.Slug] = w
			continue
		} else {
			// If we don't have a collection for this asset, create it
			collectionsMap[asset.Collection.Slug] = database.WalletCollection{
				Name:     asset.Collection.Name,
				Slug:     asset.Collection.Slug,
				ImageURL: asset.Collection.ImageURL,
				NFTs: []database.WalletAsset{{
					Name:     asset.Name,
					TokenID:  asset.TokenID,
					ImageURL: asset.ImageURL,
				}},
			}
		}

	}

	// Construct a wallet object
	var collections = make([]database.WalletCollection, 0)
	for _, collection := range collectionsMap {
		collections = append(collections, collection)
	}

	if len(collections) == 0 {
		h.logger.Info("No collections found for user", address)
		return false
	}

	wallet := database.Wallet{
		Collections: collections,
	}

	// Update collections
	wr, err := doc.Ref.Update(ctx, []firestore.Update{
		{Path: "wallet", Value: wallet},
	})

	if err != nil {
		h.logger.Error(err)
		return false
	}

	h.logger.Infow(
		"Address updated",
		"address", address,
		"updated", wr.UpdateTime,
	)

	// Make sure the collections exist in our database
	var (
		collectionSlugDocs    = make([]*firestore.DocumentRef, 0)
		slugToOSCollectionMap = make(map[string]database.WalletCollection)
		// collectionMap         = make(map[string]database.Collection)
	)

	for _, collection := range wallet.Collections {
		collectionSlugDocs = append(collectionSlugDocs, h.database.Collection("collections").Doc(collection.Slug))
		slugToOSCollectionMap[collection.Slug] = collection
	}

	docsnaps, err := h.database.GetAll(h.ctx, collectionSlugDocs)
	if err != nil {
		h.logger.Error(err)
		return false
	}

	for _, docsnap := range docsnaps {
		if !docsnap.Exists() {
			database.AddCollectionToDB(h.ctx, &h.os, h.logger, h.database, docsnap.Ref.ID)
			time.Sleep(time.Millisecond * 250)
		}
	}

	return true
}
