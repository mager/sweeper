package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
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

		h.Logger.Info("Updating user: %s", doc.Ref.ID)
		updated := h.Sweeper.UpdateUser(doc.Ref.ID)
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
				Name:       asset.Name,
				ImageURL:   asset.ImageURL,
				TokenID:    asset.TokenID,
				Attributes: adaptTraits(asset.Traits),
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
					Name:       asset.Name,
					TokenID:    asset.TokenID,
					ImageURL:   asset.ImageURL,
					Attributes: adaptTraits(asset.Traits),
				}},
			}
		}
	}

	// Construct a wallet object
	var userCollections = make([]database.WalletCollection, 0)
	for _, collection := range collectionsMap {
		userCollections = append(userCollections, collection)
	}

	if len(userCollections) == 0 {
		h.Logger.Info("No collections found for user", address)
		return false
	}

	wallet := database.Wallet{
		UpdatedAt: time.Now(),
	}

	// Make sure the collections exist in the database
	var (
		collectionSlugDocs    = make([]*firestore.DocumentRef, 0)
		slugToOSCollectionMap = make(map[string]database.WalletCollection)
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
			h.Logger.Infof("Collection %s does not exist, adding", docsnap.Ref.ID)
			database.AddCollectionToDB(h.Context, h.OpenSea, h.NFTFloorPrice, h.Logger, h.Database, docsnap.Ref.ID)
			time.Sleep(time.Millisecond * 250)
			database.UpdateCollectionStats(h.Context, h.Logger, h.OpenSea, h.BigQuery, h.NFTStats, h.Reservoir, docsnap)
			time.Sleep(time.Millisecond * 250)
		}
	}

	// TODO: Newly added collections won't have NFT prices yet
	// Update specific floor NFTs
	for _, collection := range userCollections {
		h.Logger.Info("Updating collection: ", collection.Slug)
		// pretty.Print(collection)
	}

	// TODO: Remove attributes (no need to save in db)
	for _, collection := range userCollections {
		wallet.Collections = append(wallet.Collections, database.WalletCollection{
			Name:     collection.Name,
			Slug:     collection.Slug,
			ImageURL: collection.ImageURL,
			NFTs:     adaptNFTs(collection.NFTs),
		})
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

	return true
}

func adaptNFTs(nfts []database.WalletAsset) []database.WalletAsset {
	var adapted = make([]database.WalletAsset, len(nfts))
	for _, nft := range nfts {
		adapted = append(nfts, database.WalletAsset{
			Name:     nft.Name,
			ImageURL: nft.ImageURL,
			TokenID:  nft.TokenID,
		})
	}
	return adapted
}

func adaptTraits(traits []opensea.AssetTrait) []database.Attribute {
	var attributes []database.Attribute
	// TODO: Use generics
	for _, trait := range traits {
		// Convert trait.Value to string
		var value string
		switch trait.Value.(type) {
		case string:
			value = trait.Value.(string)
		case int:
			value = strconv.Itoa(trait.Value.(int))
		case float64:
			value = strconv.FormatFloat(trait.Value.(float64), 'f', -1, 64)
		}

		attributes = append(attributes, database.Attribute{
			Key:   trait.TraitType,
			Value: value,
		})
	}
	return attributes
}
