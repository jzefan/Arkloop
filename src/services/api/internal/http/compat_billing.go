package http

import "encoding/json"

type creditBalanceResponse struct {
	OrgID   string `json:"org_id"`
	Balance int64  `json:"balance"`
}

type creditTransactionResponse struct {
	ID            string           `json:"id"`
	OrgID         string           `json:"org_id"`
	Amount        int64            `json:"amount"`
	Type          string           `json:"type"`
	ReferenceType *string          `json:"reference_type,omitempty"`
	ReferenceID   *string          `json:"reference_id,omitempty"`
	Note          *string          `json:"note,omitempty"`
	Metadata      *json.RawMessage `json:"metadata,omitempty"`
	ThreadTitle   *string          `json:"thread_title,omitempty"`
	CreatedAt     string           `json:"created_at"`
}

type meCreditsResponse struct {
	Balance      int64                       `json:"balance"`
	Transactions []creditTransactionResponse `json:"transactions"`
}

type redemptionCodeResponse struct {
	ID              string  `json:"id"`
	Code            string  `json:"code"`
	Type            string  `json:"type"`
	Value           string  `json:"value"`
	MaxUses         int     `json:"max_uses"`
	UseCount        int     `json:"use_count"`
	ExpiresAt       *string `json:"expires_at,omitempty"`
	IsActive        bool    `json:"is_active"`
	BatchID         *string `json:"batch_id,omitempty"`
	CreatedByUserID string  `json:"created_by_user_id"`
	CreatedAt       string  `json:"created_at"`
}
