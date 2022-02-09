package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

type Contract struct {
	Name      string  `firestore:"name" json:"name"`
	Address   string  `firestore:"address" json:"address"`
	NumTokens int     `firestore:"numTokens" json:"numTokens"`
	LastBlock int64   `firestore:"lastBlock" json:"lastBlock"`
	Tokens    []Token `firestore:"tokens" json:"tokens"`
}

type Token struct {
	ID    int64  `firestore:"id" json:"id"`
	Owner string `firestore:"owner" json:"owner"`
}

type UpdateContractsResp struct {
	Success bool `json:"success"`
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
		c Contract
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

func (h *Handler) getLatestContractState(c *Contract) error {
	var (
		latestBlock int64
	)

	// Fetch all transactions from Etherscan
	trxs, err := h.Etherscan.GetAllNFTTransactionsForContract(c.Address, c.LastBlock)
	if err != nil {
		return err
	}

	var (
		isNew         = c.LastBlock == 0 && len(c.Tokens) == 0
		updatedOwners = make(map[int64]string)
		tokens        = make([]Token, 0)
	)

	if isNew {
		h.Logger.Info("New contract, updating state")
	} else {
		// Set capacity to avoid reallocation
		updatedOwners = make(map[int64]string, len(c.Tokens))
		for _, token := range c.Tokens {
			// Set the current owners owners
			updatedOwners[token.ID] = token.Owner
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

		// Update the map with the latest owner
		updatedOwners[tokenID] = trx.To

		// Set latest block
		var blockInt int64
		blockInt, err = strconv.ParseInt(trx.BlockNumber, 10, 64)
		if err != nil {
			h.Logger.Errorf("Error converting block number to int: %v", err)
			return err
		}
		if latestBlock < blockInt {
			latestBlock = blockInt
		}
	}

	for id, token := range updatedOwners {
		tokens = append(tokens, Token{
			ID:    id,
			Owner: token,
		})
	}

	c.Tokens = sortTokens(tokens)
	c.LastBlock = latestBlock

	return nil
}

func sortTokens(tokens []Token) []Token {
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
