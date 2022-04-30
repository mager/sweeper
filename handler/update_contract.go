package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/mager/sweeper/database"
)

type UpdateContractsResp struct {
	Success bool `json:"success"`
}

type Token struct {
	ID       int64
	Owner    string
	LastSale int64
}

func (h *Handler) updateContract(w http.ResponseWriter, r *http.Request) {
	var (
		resp = UpdateContractsResp{}
		slug = mux.Vars(r)["slug"]
	)

	h.Logger.Infow("Updating contract slug", "slug", slug)
	resp.Success = h.updateSingleContract(slug)

	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) updateSingleContract(slug string) bool {
	// Fetch contract
	contract, err := h.Database.Collection("contracts").Doc(slug).Get(h.Context)
	if err != nil {
		h.Logger.Errorf("Error getting contract: %v", err)
		return false
	}

	var (
		c database.Contract
	)

	if err := contract.DataTo(&c); err != nil {
		h.Logger.Errorf("Error getting contract data: %v", err)
		return false
	}

	err = h.getLatestContractState(&c)
	if err != nil {
		h.Logger.Errorf("Error getting latest contract state: %v", err)
		return false
	}

	// Update contract in Firestore
	_, err = h.Database.Collection("contracts").Doc(slug).Set(h.Context, c)
	if err != nil {
		h.Logger.Errorf("Error updating contract: %v", err)
		return false
	}

	return true
}

func (h *Handler) getLatestContractState(c *database.Contract) error {
	var (
		latestBlock = c.LastBlock
	)

	// Fetch all transactions from Etherscan
	trxs, err := h.Etherscan.GetLatestTransactionsForContract(c.Address, c.LastBlock)
	if err != nil {
		return err
	}

	var (
		isNew         = c.LastBlock == 0 && len(c.Tokens) == 0
		updatedOwners = make(map[int64]Token)
		tokens        = make([]database.Token, 0)
	)

	if isNew {
		h.Logger.Info("New contract, updating state")
	} else {
		// Set capacity to avoid reallocation
		updatedOwners = make(map[int64]Token, len(c.Tokens))
		for _, token := range c.Tokens {
			// Set the current owners owners
			updatedOwners[token.ID] = Token{
				ID:       token.ID,
				Owner:    token.Owner,
				LastSale: token.LastSale,
			}
		}
	}

	// Loop through transactions
	for _, trx := range trxs {
		// Convert tokenID to int
		tokenID, err := strconv.ParseInt(trx.TokenID, 10, 64)
		if err != nil {
			h.Logger.Errorf("Error converting tokenID to int: %v", err)
			return err
		}

		// Convert timestamp to int
		timestamp, err := strconv.ParseInt(trx.Timestamp, 10, 64)
		if err != nil {
			h.Logger.Errorf("Error converting timestamp to int: %v", err)
			return err
		}

		// Update the map with the latest owner
		updatedOwners[tokenID] = Token{
			ID:       tokenID,
			Owner:    trx.To,
			LastSale: int64(timestamp),
		}
		h.Logger.Infow("Updated owner", "tokenID", tokenID, "owner", trx.To)

		// Set latest block
		var blockInt int64
		blockInt, err = strconv.ParseInt(trx.BlockNumber, 10, 64)
		h.Logger.Infow("Setting latest block number", "block", blockInt, "latestBlock", latestBlock)
		if err != nil {
			h.Logger.Errorf("Error converting block number to int: %v", err)
			return err
		}
		if latestBlock < blockInt {
			latestBlock = blockInt
			updated, err := strconv.ParseInt(trx.Timestamp, 10, 64)
			if err != nil {
				h.Logger.Errorf("Error converting timestamp to int: %v", err)
				return err
			}
			c.Updated = updated
		}
	}

	for _, token := range updatedOwners {
		tokens = append(tokens, database.Token{
			ID:       token.ID,
			Owner:    token.Owner,
			LastSale: token.LastSale,
		})
	}

	c.Tokens = sortTokens(tokens)
	c.LastBlock = latestBlock

	return nil
}

func sortTokens(tokens []database.Token) []database.Token {
	// Sort tokens
	for i := 0; i < len(tokens); i++ {
		for j := i + 1; j < len(tokens); j++ {
			if tokens[i].ID > tokens[j].ID {
				tokens[i], tokens[j] = tokens[j], tokens[i]
			}
		}
	}

	return tokens
}
