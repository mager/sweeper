package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/mager/go-opensea/opensea"
	"github.com/mager/sweeper/database"
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
		collections = h.Database.Collection("users")
		iter        = collections.Documents(h.Context)
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
			h.Logger.Error(err)
		}

		err = doc.DataTo(&u)
		if err != nil {
			h.Logger.Error(err)
		}

		updated := h.updateSingleAddress(doc.Ref.ID)
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

	h.Logger.Infof("Updated %d addresses", count)

	return true
}

func (h *Handler) updateSingleAddress(a string) bool {
	var (
		u       database.User
		doc     *firestore.DocumentSnapshot
		err     error
		address = strings.ToLower(a)
	)

	doc, err = h.Database.Collection("users").Doc(address).Get(h.Context)
	if err != nil {
		h.Logger.Errorf("Error getting user: %v, ading them to the database", err)

		// Add user to the database
		_, err = h.Database.Collection("users").Doc(address).Set(h.Context, map[string]interface{}{
			"address": address,
		})
		if err != nil {
			h.Logger.Error(err)
		}
		doc, err = h.Database.Collection("users").Doc(address).Get(h.Context)
		if err != nil {
			h.Logger.Errorf("Error getting user again: %v, returning", err)
			return false
		}
	}

	err = doc.DataTo(&u)
	if err != nil {
		h.Logger.Error(err)
	}

	var (
		openseaAssets  = make([]opensea.Asset, 0)
		collectionsMap = make(map[string]database.WalletCollection)
	)

	// Fetch the user's collections & NFTs from OpenSea
	openseaAssets = h.getOpenSeaAssets(address)
	h.Logger.Infow("Fetched OpenSea assets", "address", address, "count", len(openseaAssets))
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
		h.Logger.Info("No collections found for user", address)
		return false
	}

	wallet := database.Wallet{
		Collections: collections,
		UpdatedAt:   time.Now(),
	}

	// Update collections
	wr, err := doc.Ref.Update(h.Context, []firestore.Update{
		{Path: "wallet", Value: wallet},
	})

	if err != nil {
		h.Logger.Error(err)
		return false
	}

	h.Logger.Infow(
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
		collectionSlugDocs = append(collectionSlugDocs, h.Database.Collection("collections").Doc(collection.Slug))
		slugToOSCollectionMap[collection.Slug] = collection
	}

	docsnaps, err := h.Database.GetAll(h.Context, collectionSlugDocs)
	if err != nil {
		h.Logger.Error(err)
		return false
	}

	for _, docsnap := range docsnaps {
		if !docsnap.Exists() {
			database.AddCollectionToDB(h.Context, h.OpenSea, h.Logger, h.Database, docsnap.Ref.ID)
			time.Sleep(time.Millisecond * 250)
		}
	}

	return true
}
