package handler

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

type CollectionToken struct {
	TokenID  int       `json:"tokenID"`
	Address  string    `json:"address"`
	Acquired time.Time `json:"acquired"`
}

type GetCollectionTokensResp struct {
	Tokens []CollectionToken `json:"tokens"`
}

// NOTE: This is a temporary endpoint for testing
func (h *Handler) getCollectionTokens(w http.ResponseWriter, r *http.Request) {
	var (
		resp = GetCollectionTokensResp{}
		slug = mux.Vars(r)["slug"]
	)

	h.logger.Infow("Getting collection owners", "collection", slug)

	// TODO: Fetch contract address from OpenSea
	// contract := "0x90bae7c0d86b2583d02c072d45bd64ace0b8db86"
	contract := "0x8a90cab2b38dba80c64b7734e58ee1db38b8992e"

	// Get token transactions from Etherscan
	txs, err := h.etherscanClient.GetAllNFTTransactionsForContract(
		contract,
	)
	if err != nil {
		h.logger.Errorw("Error getting token transactions", "error", err)
		http.Error(w, "Error getting token transactions", http.StatusInternalServerError)
		return
	}

	h.logger.Infow("Got token transactions", "collection", slug, "txs", len(txs))

	// Set the ownersMap with the tokenID as the key and the to address as the value
	for _, tx := range txs {
		// Convert the tokenID to an int
		tokenID, err := strconv.Atoi(tx.TokenID)
		if err != nil {
			h.logger.Errorw("Error converting tokenID to int", "error", err)
			http.Error(w, "Error converting tokenID to int", http.StatusInternalServerError)
			return
		}

		// Convert the timestamp to an int
		ts, _ := strconv.ParseInt(tx.Timestamp, 10, 64)
		if err != nil {
			h.logger.Errorw("Error converting timestamp to int", "error", err)
			http.Error(w, "Error converting timestamp to int", http.StatusInternalServerError)
			return
		}

		resp.Tokens = append(resp.Tokens, CollectionToken{
			TokenID:  tokenID,
			Address:  tx.To,
			Acquired: time.Unix(ts, 0),
		})
	}

	// Sort by tokenID
	sort.Slice(resp.Tokens[:], func(i, j int) bool {
		return resp.Tokens[i].TokenID < resp.Tokens[j].TokenID
	})

	json.NewEncoder(w).Encode(resp)
}
