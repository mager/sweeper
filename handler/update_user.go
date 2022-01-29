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

	h.Logger.Infow("Updating user address", "address", req.Address)
	resp.Success = h.doUpdateAddress(req.DryRun, req.Address)

	json.NewEncoder(w).Encode(resp)
}

// doUpdateAddresses updates a single address
func (h *Handler) doUpdateAddress(dryRun bool, address string) bool {
	var (
		ctx = context.Background()
		u   database.User
	)

	h.Logger.Info("Updating address", "address", address)
	docsnap, err := h.Database.Collection("users").Doc(address).Get(h.Context)

	if err != nil {
		h.Logger.Errorf("Error getting user: %v", err)
		return false
	}

	err = docsnap.DataTo(&u)
	if err != nil {
		h.Logger.Error(err)
	}

	h.updateSingleAddress(ctx, docsnap)

	return true
}
