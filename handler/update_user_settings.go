package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/mager/sweeper/database"
)

type UpdateUserSettingsReq struct {
	Address  string                `json:"address"`
	Settings database.UserSettings `json:"settings"`
}

type UpdateUserSettingsResp struct {
	Success bool `json:"success"`
}

func (h *Handler) updateUserSettings(w http.ResponseWriter, r *http.Request) {
	// Get address from path params
	var (
		req  = UpdateUserSettingsReq{}
		resp UpdateUserSettingsResp
	)

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Fetch the user
	user, err := h.getUser(strings.ToLower(req.Address))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Update the user
	_, err = user.Ref.Set(h.Context, map[string]interface{}{
		"settings": req.Settings,
	}, firestore.MergeAll)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp.Success = true

	json.NewEncoder(w).Encode(resp)
}
