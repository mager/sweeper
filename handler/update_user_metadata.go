package handler

import (
	"encoding/json"
	"net/http"

	"github.com/mager/sweeper/storage"
)

type UpdateUserAvatarResp struct {
	Success bool `json:"success"`
}

func (h *Handler) updateUserAvatar(w http.ResponseWriter, r *http.Request) {
	// Get address from path params
	var (
		resp UpdateUserAvatarResp
	)

	resp.Success = storage.UploadUserMetadata(h.Context, h.Logger, h.Storage, r)

	json.NewEncoder(w).Encode(resp)
}
