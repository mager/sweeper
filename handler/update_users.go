package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/mager/go-opensea/opensea"
	"github.com/mager/sweeper/database"
	os "github.com/mager/sweeper/opensea"
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
	StartAt  string   `json:"start_at"`
	DryRun   bool     `json:"dry_run"`
}

type UpdateUsersResp struct {
	Queued bool `json:"queued"`
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

	go h.doUpdateAddresses(req)

	resp.Queued = true

	json.NewEncoder(w).Encode(resp)
}

// doUpdateAddresses updates a collection of addresses
func (h *Handler) doUpdateAddresses(r UpdateUsersReq) bool {
	var (
		users = h.Database.Collection("users")
		u     database.User
		count = 0
		iter  = users.Documents(h.Context)
	)

	if r.StartAt != "" {
		iter = users.OrderBy(firestore.DocumentID, firestore.Asc).StartAt(r.StartAt).Documents(h.Context)
	}

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
		h.Logger.Errorf("Error getting user: %v, adding them to the database", err)

		// Add user to the database
		_, err = users.Doc(address).Set(h.Context, map[string]interface{}{
			"address":  address,
			"updating": true,
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
	var collectionFloorMap = make(map[string]float64)
	for _, docsnap := range docsnaps {
		if !docsnap.Exists() {
			h.Logger.Infof("Collection %s does not exist, adding", docsnap.Ref.ID)

			_, updated := database.AddCollectionToDB(h.Context, h.OpenSea, h.NFTFloorPrice, h.Logger, h.Database, docsnap.Ref.ID)
			time.Sleep(time.Millisecond * os.OpenSeaRateLimit)

			if updated {
				database.UpdateCollectionStats(h.Context, h.Logger, h.OpenSea, h.BigQuery, h.NFTStats, h.Reservoir, docsnap)
				time.Sleep(time.Millisecond * os.OpenSeaRateLimit)
			}
		} else {
			// Get attribute floors
			var c database.Collection
			err = docsnap.DataTo(&c)
			if err != nil {
				h.Logger.Error(err)
			}

			collectionAttributesMap[c.Slug] = c.Attributes
			collectionFloorMap[c.Slug] = c.Floor
		}
	}

	wallet := database.Wallet{
		Collections: adaptWalletCollections(walletCollections, collectionAttributesMap, collectionFloorMap),
		UpdatedAt:   time.Now(),
	}

	// Update collections
	wr, err := doc.Ref.Update(h.Context, []firestore.Update{
		{Path: "wallet", Value: wallet},
		{Path: "updating", Value: false},
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
	var adapted = make([]database.WalletAsset, 0)
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

func adaptWalletCollections(
	collections []database.WalletCollection,
	collectionAttributesMap map[string][]database.Attribute,
	collectionFloorMap map[string]float64,
) []database.WalletCollection {
	var adapted = make([]database.WalletCollection, 0)
	// Determine NFT floor based on collection attribute floors
	for _, collection := range collections {
		var attributes = collectionAttributesMap[collection.Slug]
		var nfts = make([]database.WalletAsset, 0)
		for _, nft := range collection.NFTs {
			var floor = collectionFloorMap[collection.Slug]
			// Loop through the nft attributes and find a matching attribute in our collection attributes
			var matchedAttrsMap = make(map[string]float64)
			for _, attr := range nft.Attributes {
				for _, collectionAttr := range attributes {
					if attr.Key == collectionAttr.Key && attr.Value == collectionAttr.Value {
						var key = fmt.Sprintf("%s-%s", attr.Key, attr.Value)
						matchedAttrsMap[key] = collectionAttr.Floor
					}
				}
			}

			// Find the attribute with the highest floor that matches the collection attribute
			for _, f := range matchedAttrsMap {
				if f > floor {
					floor = f
				}
			}

			nft.Floor = floor
			nfts = append(nfts, database.WalletAsset{
				Name:     nft.Name,
				ImageURL: nft.ImageURL,
				TokenID:  nft.TokenID,
				Floor:    nft.Floor,
			})
		}
		adapted = append(adapted, database.WalletCollection{
			Slug:     collection.Slug,
			Name:     collection.Name,
			NFTs:     nfts,
			ImageURL: collection.ImageURL,
			Floor:    collectionFloorMap[collection.Slug],
		})
	}

	return adapted
}
