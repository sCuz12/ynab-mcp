package tools

import (
	"context"
	"encoding/json"

	"ynab-mcp/internal/ynab"
)

var transactionFields = map[string]field{
	"account_id":  {Kind: "string", Description: "Account ID."},
	"date":        dateField,
	"amount":      {Kind: "integer", Description: "Amount in milliunits; negative is an outflow."},
	"payee_id":    {Kind: "string", Description: "Existing payee ID."},
	"payee_name":  {Kind: "string", Description: "Payee name; YNAB may create a new payee."},
	"category_id": {Kind: "string", Description: "Category ID."},
	"memo":        {Kind: "string", Description: "Transaction memo (up to 500 characters)."},
	"cleared":     {Kind: "string", Description: "Cleared status.", Enum: []string{"cleared", "uncleared", "reconciled"}},
	"approved":    {Kind: "boolean", Description: "Whether the transaction is approved."},
	"flag_color":  {Kind: "string", Description: "YNAB flag color."},
}

func (a arguments) transactionFields() ynab.TransactionFields {
	stringValue := func(name string) *string {
		value, ok := a[name].(string)
		if !ok {
			return nil
		}
		return &value
	}
	fields := ynab.TransactionFields{
		AccountID: stringValue("account_id"), Date: stringValue("date"),
		PayeeID: stringValue("payee_id"), PayeeName: stringValue("payee_name"),
		CategoryID: stringValue("category_id"), Memo: stringValue("memo"),
		Cleared: stringValue("cleared"), FlagColor: stringValue("flag_color"),
	}
	if value, ok := a["amount"].(int64); ok {
		amount := ynab.Milliunits(value)
		fields.Amount = &amount
	}
	if value, ok := a["approved"].(bool); ok {
		fields.Approved = &value
	}
	return fields
}

func writeDefinitions(client *ynab.Client) []definition {
	return []definition{
		{
			Name: "create_transaction", Description: "Create one transaction. Amount is in milliunits; negative means outflow.",
			Required: []string{"plan_id", "account_id", "date", "amount"}, Fields: withPlan(transactionFields),
			Call: func(ctx context.Context, a arguments) (json.RawMessage, error) {
				return client.CreateTransaction(ctx, a.string("plan_id"), a.transactionFields())
			},
		},
		{
			Name: "update_transaction", Description: "Update supplied fields on one transaction.",
			Required: []string{"plan_id", "transaction_id"}, Fields: withPlan(mergeFields(transactionFields, map[string]field{"transaction_id": identifier})),
			Call: func(ctx context.Context, a arguments) (json.RawMessage, error) {
				return client.UpdateTransaction(ctx, a.string("plan_id"), a.string("transaction_id"), a.transactionFields())
			},
		},
		{
			Name: "delete_transaction", Description: "Delete one transaction. This changes the YNAB plan.",
			Required: []string{"plan_id", "transaction_id"}, Fields: withPlan(map[string]field{"transaction_id": identifier}),
			Call: func(ctx context.Context, a arguments) (json.RawMessage, error) {
				return client.DeleteTransaction(ctx, a.string("plan_id"), a.string("transaction_id"))
			},
		},
	}
}

func mergeFields(a, b map[string]field) map[string]field {
	result := make(map[string]field, len(a)+len(b))
	for key, value := range a {
		result[key] = value
	}
	for key, value := range b {
		result[key] = value
	}
	return result
}
