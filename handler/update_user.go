package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/mager/sweeper/database"
	"github.com/mager/sweeper/opensea"
)

type UpdateUserReq struct {
	Address string `json:"address"`
	DryRun  bool   `json:"dry_run"`
}

type UpdateUserResp struct {
	Success bool `json:"success"`
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	var (
		req  UpdateUserReq
		resp = UpdateUserResp{}
	)

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp.Success = h.updateAddress(req.DryRun, req.Address)

	json.NewEncoder(w).Encode(resp)
}

// updateAddresses updates a single address
func (h *Handler) updateAddress(dryRun bool, address string) bool {
	var (
		ctx                    = context.Background()
		u                      database.User
		openseaCollections     = make([]opensea.OpenSeaCollectionV2, 0)
		openseaAssets          = make([]opensea.OpenSeaAssetV2, 0)
		openseaAssetsChan      = make(chan []opensea.OpenSeaAssetV2)
		openseaCollectionsChan = make(chan []opensea.OpenSeaCollectionV2)
	)

	h.logger.Info("Updating address", "address", address)

	docsnap, err := h.database.Collection("users").Doc(address).Get(h.ctx)

	if err != nil {
		h.logger.Errorf("Error getting user: %v", err)
		return false
	}

	err = docsnap.DataTo(&u)
	if err != nil {
		h.logger.Error(err)
	}

	var (
		wallet = database.Wallet{}
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
		})
	}

	// Update collections
	if !dryRun {
		wr, err := docsnap.Ref.Update(ctx, []firestore.Update{
			{Path: "wallet", Value: wallet},
		})

		if err != nil {
			h.logger.Error(err)
		}

		h.logger.Infow(
			"Address updated",
			"address", docsnap.Ref.ID,
			"updated", wr.UpdateTime,
		)
	}

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
	}

	for _, docsnap := range docsnaps {
		if !docsnap.Exists() {
			database.AddCollectionToDB(h.ctx, &h.os, h.logger, h.database, docsnap.Ref.ID)
			time.Sleep(time.Millisecond * 250)
		}
	}

	return true
}
