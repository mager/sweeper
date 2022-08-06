package handler

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/mager/sweeper/database"
	"google.golang.org/api/iterator"
)

type UpdateRandomNFTReq struct {
	Success bool `json:"success"`
}

func (h *Handler) updateRandomNFT(w http.ResponseWriter, r *http.Request) {
	var (
		resp = UpdateRandomNFTReq{}
	)

	resp.Success = h.doUpdateRandomNFT()

	json.NewEncoder(w).Encode(resp)
}

// TODO: Optimize this function
func (h *Handler) doUpdateRandomNFT() bool {
	var (
		docs  = make([]*firestore.DocumentRef, 0)
		users = h.Database.Collection("users")
	)

	// Initialize local pseudorandom generator
	rand.Seed(time.Now().Unix())

	// Fetch a random user
	iter := users.Where("isFren", "==", true).Documents(h.Context)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			h.Logger.Errorf("Error fetching collections: %v", err)
			break
		}
		docs = append(docs, doc.Ref)
	}

	// Get random user
	user := docs[rand.Intn(len(docs))]
	u, err := user.Get(h.Context)
	if err != nil {
		h.Logger.Errorf("Error fetching user: %v", err)
	}

	// Get random NFT
	var userData database.User
	err = u.DataTo(&userData)
	if err != nil {
		h.Logger.Errorf("Error fetching user: %v", err)
	}

	collection := userData.Wallet.Collections[rand.Intn(len(userData.Wallet.Collections))]
	nft := collection.NFTs[rand.Intn(len(collection.NFTs))]

	// Update NFT
	h.Database.Collection("features").Doc("nftoftheday").Set(h.Context, map[string]interface{}{
		"collectionName": collection.Name,
		"collectionSlug": collection.Slug,
		"imageUrl":       nft.ImageURL,
		"name":           nft.Name,
		"owner":          getOwner(u, userData),
		"ownerName":      userData.Name,
		"updated":        time.Now(),
	}, firestore.MergeAll)

	return true
}

func getOwner(u *firestore.DocumentSnapshot, user database.User) string {
	if user.ENSName != "" {
		return user.ENSName
	}
	return u.Ref.ID
}
