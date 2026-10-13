package ynab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type Milliunits int64

type TransactionQuery struct {
	SinceDate string
	UntilDate string
	Type      string
	Knowledge *int64
}

func (q TransactionQuery) values() (url.Values, error) {
	values, err := knowledgeQuery(q.Knowledge)
	if err != nil {
		return nil, err
	}
	if q.SinceDate != "" {
		if err := ValidateDate("since_date", q.SinceDate); err != nil {
			return nil, err
		}
		values.Set("since_date", q.SinceDate)
	}
	if q.UntilDate != "" {
		if err := ValidateDate("until_date", q.UntilDate); err != nil {
			return nil, err
		}
		values.Set("until_date", q.UntilDate)
	}
	if q.Type != "" {
		if q.Type != "uncategorized" && q.Type != "unapproved" {
			return nil, fmt.Errorf("invalid type value")
		}
		values.Set("type", q.Type)
	}
	return values, nil
}

type TransactionFields struct {
	AccountID  *string     `json:"account_id,omitempty"`
	Date       *string     `json:"date,omitempty"`
	Amount     *Milliunits `json:"amount,omitempty"`
	PayeeID    *string     `json:"payee_id,omitempty"`
	PayeeName  *string     `json:"payee_name,omitempty"`
	CategoryID *string     `json:"category_id,omitempty"`
	Memo       *string     `json:"memo,omitempty"`
	Cleared    *string     `json:"cleared,omitempty"`
	Approved   *bool       `json:"approved,omitempty"`
	FlagColor  *string     `json:"flag_color,omitempty"`
}

func (f TransactionFields) validate(create bool) error {
	if create {
		if f.AccountID == nil {
			return fmt.Errorf("missing required argument %q", "account_id")
		}
		if f.Date == nil {
			return fmt.Errorf("missing required argument %q", "date")
		}
		if f.Amount == nil {
			return fmt.Errorf("missing required argument %q", "amount")
		}
	} else if f == (TransactionFields{}) {
		return fmt.Errorf("provide at least one transaction field to update")
	}
	for name, value := range map[string]*string{
		"account_id": f.AccountID, "payee_id": f.PayeeID, "payee_name": f.PayeeName,
		"category_id": f.CategoryID, "memo": f.Memo, "cleared": f.Cleared, "flag_color": f.FlagColor,
	} {
		if value != nil && *value == "" {
			return fmt.Errorf("%s must be a nonempty string", name)
		}
	}
	if f.Date != nil {
		if err := ValidateDate("date", *f.Date); err != nil {
			return err
		}
	}
	if f.Memo != nil && len([]rune(*f.Memo)) > 500 {
		return fmt.Errorf("memo exceeds 500 characters")
	}
	if f.PayeeName != nil && len([]rune(*f.PayeeName)) > 200 {
		return fmt.Errorf("payee_name exceeds 200 characters")
	}
	if f.Cleared != nil && *f.Cleared != "cleared" && *f.Cleared != "uncleared" && *f.Cleared != "reconciled" {
		return fmt.Errorf("invalid cleared value")
	}
	return nil
}

func (c *Client) ListTransactions(ctx context.Context, planID string, filter TransactionQuery) (json.RawMessage, error) {
	path, err := planPath(planID)
	if err != nil {
		return nil, err
	}
	query, err := filter.values()
	if err != nil {
		return nil, err
	}
	return c.request(ctx, http.MethodGet, path+"/transactions", query, nil)
}

func (c *Client) ListAccountTransactions(ctx context.Context, planID, accountID string, filter TransactionQuery) (json.RawMessage, error) {
	path, err := childPath(planID, "accounts", "account_id", accountID)
	if err != nil {
		return nil, err
	}
	query, err := filter.values()
	if err != nil {
		return nil, err
	}
	return c.request(ctx, http.MethodGet, path+"/transactions", query, nil)
}

func (c *Client) GetTransaction(ctx context.Context, planID, transactionID string) (json.RawMessage, error) {
	path, err := childPath(planID, "transactions", "transaction_id", transactionID)
	if err != nil {
		return nil, err
	}
	return c.request(ctx, http.MethodGet, path, nil, nil)
}

func (c *Client) CreateTransaction(ctx context.Context, planID string, fields TransactionFields) (json.RawMessage, error) {
	if err := fields.validate(true); err != nil {
		return nil, err
	}
	path, err := planPath(planID)
	if err != nil {
		return nil, err
	}
	return c.request(ctx, http.MethodPost, path+"/transactions", nil, map[string]any{"transaction": fields})
}

func (c *Client) UpdateTransaction(ctx context.Context, planID, transactionID string, fields TransactionFields) (json.RawMessage, error) {
	if err := fields.validate(false); err != nil {
		return nil, err
	}
	path, err := childPath(planID, "transactions", "transaction_id", transactionID)
	if err != nil {
		return nil, err
	}
	return c.request(ctx, http.MethodPut, path, nil, map[string]any{"transaction": fields})
}

func (c *Client) DeleteTransaction(ctx context.Context, planID, transactionID string) (json.RawMessage, error) {
	path, err := childPath(planID, "transactions", "transaction_id", transactionID)
	if err != nil {
		return nil, err
	}
	return c.request(ctx, http.MethodDelete, path, nil, nil)
}
