package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/kr/pretty"
)

type GetCollectionOwnersResp struct {
	Owners []string `json:"owners"`
}

// NOTE: This is a temporary endpoint for testing
func (h *Handler) getCollectionOwners(w http.ResponseWriter, r *http.Request) {
	var (
		resp = GetCollectionOwnersResp{Owners: []string{}}
		slug = mux.Vars(r)["slug"]
	)

	h.logger.Infow("Getting collection owners", "collection", slug)

	contract, address := "0x90bae7c0d86b2583d02c072d45bd64ace0b8db86",
		"0x064DcA21b1377D1655AC3CA3e95282D9494E5611"
	start, end := 13557530, 13857876

	// Get token transactions from Etherscan
	txs, err := h.etherscanClient.Client.ERC721Transfers(
		&contract,
		&address,
		&start,
		&end,
		0,
		0,
		true,
	)
	if err != nil {
		h.logger.Errorw("Error getting token transactions", "error", err)
		http.Error(w, "Error getting token transactions", http.StatusInternalServerError)
		return
	}

	h.logger.Infow("Got token transactions", "collection", slug, "txs", len(txs))

	pretty.Print(txs)

	json.NewEncoder(w).Encode(resp)
}
