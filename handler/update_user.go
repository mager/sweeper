package handler

import (
	"encoding/json"
	"net/http"
)

type UpdateUserReq struct {
	Address string `json:"address"`
	DryRun  bool   `json:"dry_run"`
}

type UpdateUserResp struct {
	Queued bool `json:"queued"`
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
	go h.doUpdateAddress(req.DryRun, req.Address)

	resp.Queued = true

	json.NewEncoder(w).Encode(resp)
}

// doUpdateAddresses updates a single address
func (h *Handler) doUpdateAddress(dryRun bool, address string) bool {
	return h.updateSingleAddress(address)
}
