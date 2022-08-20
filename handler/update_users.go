package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/kr/pretty"
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
		users = h.Database.Collection("users")
		iter  = users.Documents(h.Context)
		u     database.User
		count = 0
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
		u           database.User
		doc         *firestore.DocumentSnapshot
		err         error
		address     = strings.ToLower(a)
		users       = h.Database.Collection("users")
		collections = h.Database.Collection("collections")
	)

	doc, err = users.Doc(address).Get(h.Context)
	if err != nil {
		h.Logger.Errorf("Error getting user: %v, ading them to the database", err)

		// Add user to the database
		_, err = users.Doc(address).Set(h.Context, map[string]interface{}{
			"address": address,
		})
		if err != nil {
			h.Logger.Error(err)
		}
		doc, err = users.Doc(address).Get(h.Context)
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
	var walletCollections = make([]database.WalletCollection, 0)
	for _, collection := range collectionsMap {
		walletCollections = append(walletCollections, collection)
	}

	if len(walletCollections) == 0 {
		h.Logger.Info("No collections found for user", address)
		return false
	}

	// Make sure the collections exist in our database
	var (
		collectionSlugDocs    = make([]*firestore.DocumentRef, 0)
		slugToOSCollectionMap = make(map[string]database.WalletCollection)
	)

	for _, collection := range walletCollections {
		collectionSlugDocs = append(collectionSlugDocs, collections.Doc(collection.Slug))
		slugToOSCollectionMap[collection.Slug] = collection
	}

	docsnaps, err := h.Database.GetAll(h.Context, collectionSlugDocs)
	if err != nil {
		h.Logger.Error(err)
		return false
	}

	var collectionAttributesMap = make(map[string][]database.Attribute)
	for _, docsnap := range docsnaps {
		if !docsnap.Exists() {
			h.Logger.Infof("Collection %s does not exist, adding", docsnap.Ref.ID)

			database.AddCollectionToDB(h.Context, h.OpenSea, h.NFTFloorPrice, h.Logger, h.Database, docsnap.Ref.ID)
			time.Sleep(time.Millisecond * 250)

			database.UpdateCollectionStats(h.Context, h.Logger, h.OpenSea, h.BigQuery, h.NFTStats, h.Reservoir, docsnap)
			time.Sleep(time.Millisecond * 250)
		} else {
			// Get attribute floors
			var c database.Collection
			err = docsnap.DataTo(&c)
			if err != nil {
				h.Logger.Error(err)
			}

			collectionAttributesMap[c.Slug] = c.Attributes
		}
	}

	wallet := database.Wallet{
		Collections: adaptWalletCollections(walletCollections, collectionAttributesMap),
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

	return true
}

func adaptNFTs(nfts []database.WalletAsset) []database.WalletAsset {
	var adapted = make([]database.WalletAsset, len(nfts))
	for _, nft := range nfts {
		adapted = append(nfts, database.WalletAsset{
			Name:     nft.Name,
			ImageURL: nft.ImageURL,
			TokenID:  nft.TokenID,
			Floor:    nft.Floor,
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

func adaptWalletCollections(collections []database.WalletCollection, collectionAttributesMap map[string][]database.Attribute) []database.WalletCollection {
	var adapted = make([]database.WalletCollection, len(collections))

	// Determine NFT floor based on collection attribute floors
	for _, collection := range collections {
		var attributes = collectionAttributesMap[collection.Slug]
		for _, attribute := range attributes {
			for _, nft := range collection.NFTs {
				var nftFloor float64
				var matchedAttrsMap = make(map[string]float64)

				for _, nftAttr := range nft.Attributes {
					if nftAttr.Key == attribute.Key {
						var key = fmt.Sprintf("%s-%s", nftAttr.Key, nftAttr.Value)
						matchedAttrsMap[key] = attribute.Floor
					}
				}

				// Find the attribute with the highest floor that matches the collection attribute
				for key, floor := range matchedAttrsMap {
					pretty.Print("FFF", key, floor, "\n")
					if floor > nftFloor {
						nftFloor = floor
					}
				}
				nft.Floor = nftFloor
			}
		}
	}

	for _, collection := range collections {
		adapted = append(adapted, database.WalletCollection{
			Name:     collection.Name,
			Slug:     collection.Slug,
			ImageURL: collection.ImageURL,
			NFTs:     adaptNFTs(collection.NFTs),
		})
	}
	return adapted
}
