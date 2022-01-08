package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/mager/sweeper/database"
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

	resp.Success = h.doUpdateAddress(req.DryRun, req.Address)

	json.NewEncoder(w).Encode(resp)
}

// doUpdateAddresses updates a single address
func (h *Handler) doUpdateAddress(dryRun bool, address string) bool {
	var (
		ctx = context.Background()
		u   database.User
	)

	docsnap, err := h.database.Collection("users").Doc(address).Get(h.ctx)

	if err != nil {
		h.logger.Errorf("Error getting user: %v", err)
		return false
	}

	err = docsnap.DataTo(&u)
	if err != nil {
		h.logger.Error(err)
	}

	h.updateSingleAddressV2(ctx, docsnap)

	return true
}
