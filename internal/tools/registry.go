// Package tools maps MCP tools to YNAB operations and owns their public schemas.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	"ynab-mcp/internal/mcp"
	"ynab-mcp/internal/ynab"
)

type field struct {
	Kind        string
	Description string
	Enum        []string
}

type arguments map[string]any

func (a arguments) string(name string) string {
	value, _ := a[name].(string)
	return value
}

func (a arguments) knowledge() *int64 {
	value, ok := a["last_knowledge_of_server"].(int64)
	if !ok {
		return nil
	}
	return &value
}

func (a arguments) transactionQuery() ynab.TransactionQuery {
	return ynab.TransactionQuery{
		SinceDate: a.string("since_date"), UntilDate: a.string("until_date"),
		Type: a.string("type"), Knowledge: a.knowledge(),
	}
}

type definition struct {
	Name        string
	Description string
	Required    []string
	Fields      map[string]field
	Call        func(context.Context, arguments) (json.RawMessage, error)
}

func (d definition) schema() map[string]any {
	properties := make(map[string]any, len(d.Fields))
	for name, f := range d.Fields {
		property := map[string]any{"type": f.Kind, "description": f.Description}
		if len(f.Enum) > 0 {
			property["enum"] = f.Enum
		}
		properties[name] = property
	}
	return map[string]any{
		"type": "object", "properties": properties, "required": append([]string{}, d.Required...),
		"additionalProperties": false,
	}
}

func (d definition) decode(raw json.RawMessage) (arguments, error) {
	var values map[string]json.RawMessage
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, fmt.Errorf("arguments must be an object")
		}
		if string(raw) != "null" && values == nil {
			return nil, fmt.Errorf("arguments must be an object")
		}
	}
	for _, name := range d.Required {
		if _, ok := values[name]; !ok {
			return nil, fmt.Errorf("missing required argument %q", name)
		}
	}
	args := make(arguments, len(values))
	for name, value := range values {
		f, ok := d.Fields[name]
		if !ok {
			return nil, fmt.Errorf("unknown argument %q", name)
		}
		if string(value) == "null" {
			return nil, fmt.Errorf("%s cannot be null", name)
		}
		switch f.Kind {
		case "string":
			var v string
			if err := json.Unmarshal(value, &v); err != nil || v == "" {
				return nil, fmt.Errorf("%s must be a nonempty string", name)
			}
			if len(f.Enum) > 0 && !slices.Contains(f.Enum, v) {
				return nil, fmt.Errorf("invalid %s value", name)
			}
			args[name] = v
		case "integer":
			var number json.Number
			if err := json.Unmarshal(value, &number); err != nil {
				return nil, fmt.Errorf("%s must be an integer", name)
			}
			v, err := strconv.ParseInt(number.String(), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("%s must be an integer", name)
			}
			args[name] = v
		case "boolean":
			var v bool
			if err := json.Unmarshal(value, &v); err != nil {
				return nil, fmt.Errorf("%s must be a boolean", name)
			}
			args[name] = v
		}
	}
	return args, nil
}

var (
	planID     = field{Kind: "string", Description: "Plan ID; last-used is also accepted."}
	identifier = field{Kind: "string", Description: "YNAB resource ID."}
	dateField  = field{Kind: "string", Description: "ISO date (YYYY-MM-DD)."}
	knowledge  = field{Kind: "integer", Description: "Server knowledge from an earlier response; returns only changes."}
	filterType = field{Kind: "string", Description: "Transaction filter.", Enum: []string{"uncategorized", "unapproved"}}
)

func withPlan(extra map[string]field) map[string]field {
	fields := map[string]field{"plan_id": planID}
	for key, value := range extra {
		fields[key] = value
	}
	return fields
}

func transactionFilters(extra map[string]field) map[string]field {
	fields := map[string]field{
		"since_date": dateField, "until_date": dateField, "type": filterType,
		"last_knowledge_of_server": knowledge,
	}
	for key, value := range extra {
		fields[key] = value
	}
	return withPlan(fields)
}

func New(client *ynab.Client) []mcp.Tool {
	definitions := readDefinitions(client)
	definitions = append(definitions, writeDefinitions(client)...)
	registered := make([]mcp.Tool, 0, len(definitions))
	for _, def := range definitions {
		def := def
		registered = append(registered, mcp.Tool{
			Name: def.Name, Description: def.Description, InputSchema: def.schema(),
			Handle: func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
				args, err := def.decode(raw)
				if err != nil {
					return nil, err
				}
				return def.Call(ctx, args)
			},
		})
	}
	return registered
}

func readDefinitions(client *ynab.Client) []definition {
	return []definition{
		{Name: "list_plans", Description: "List YNAB plans and their IDs.", Fields: map[string]field{}, Call: func(ctx context.Context, _ arguments) (json.RawMessage, error) { return client.ListPlans(ctx) }},
		{Name: "get_plan", Description: "Get a full plan export. This response can be large.", Required: []string{"plan_id"}, Fields: withPlan(map[string]field{"last_knowledge_of_server": knowledge}), Call: func(ctx context.Context, a arguments) (json.RawMessage, error) {
			return client.GetPlan(ctx, a.string("plan_id"), a.knowledge())
		}},
		{Name: "list_accounts", Description: "List accounts and balances for a plan.", Required: []string{"plan_id"}, Fields: withPlan(map[string]field{"last_knowledge_of_server": knowledge}), Call: func(ctx context.Context, a arguments) (json.RawMessage, error) {
			return client.ListAccounts(ctx, a.string("plan_id"), a.knowledge())
		}},
		{Name: "get_account", Description: "Get one account and its balance.", Required: []string{"plan_id", "account_id"}, Fields: withPlan(map[string]field{"account_id": identifier}), Call: func(ctx context.Context, a arguments) (json.RawMessage, error) {
			return client.GetAccount(ctx, a.string("plan_id"), a.string("account_id"))
		}},
		{Name: "list_categories", Description: "List categories grouped by category group, with current month amounts.", Required: []string{"plan_id"}, Fields: withPlan(map[string]field{"last_knowledge_of_server": knowledge}), Call: func(ctx context.Context, a arguments) (json.RawMessage, error) {
			return client.ListCategories(ctx, a.string("plan_id"), a.knowledge())
		}},
		{Name: "get_category", Description: "Get one category with current month amounts.", Required: []string{"plan_id", "category_id"}, Fields: withPlan(map[string]field{"category_id": identifier}), Call: func(ctx context.Context, a arguments) (json.RawMessage, error) {
			return client.GetCategory(ctx, a.string("plan_id"), a.string("category_id"))
		}},
		{Name: "list_payees", Description: "List payees for a plan.", Required: []string{"plan_id"}, Fields: withPlan(map[string]field{"last_knowledge_of_server": knowledge}), Call: func(ctx context.Context, a arguments) (json.RawMessage, error) {
			return client.ListPayees(ctx, a.string("plan_id"), a.knowledge())
		}},
		{Name: "list_months", Description: "List plan months and their summary amounts.", Required: []string{"plan_id"}, Fields: withPlan(map[string]field{"last_knowledge_of_server": knowledge}), Call: func(ctx context.Context, a arguments) (json.RawMessage, error) {
			return client.ListMonths(ctx, a.string("plan_id"), a.knowledge())
		}},
		{Name: "get_month", Description: "Get one plan month, including its categories.", Required: []string{"plan_id", "month"}, Fields: withPlan(map[string]field{"month": {Kind: "string", Description: "YYYY-MM-01 or current."}}), Call: func(ctx context.Context, a arguments) (json.RawMessage, error) {
			return client.GetMonth(ctx, a.string("plan_id"), a.string("month"))
		}},
		{Name: "list_transactions", Description: "List plan transactions. Defaults to the last year if since_date is omitted.", Required: []string{"plan_id"}, Fields: transactionFilters(nil), Call: func(ctx context.Context, a arguments) (json.RawMessage, error) {
			return client.ListTransactions(ctx, a.string("plan_id"), a.transactionQuery())
		}},
		{Name: "list_account_transactions", Description: "List transactions for one account.", Required: []string{"plan_id", "account_id"}, Fields: transactionFilters(map[string]field{"account_id": identifier}), Call: func(ctx context.Context, a arguments) (json.RawMessage, error) {
			return client.ListAccountTransactions(ctx, a.string("plan_id"), a.string("account_id"), a.transactionQuery())
		}},
		{Name: "get_transaction", Description: "Get one transaction.", Required: []string{"plan_id", "transaction_id"}, Fields: withPlan(map[string]field{"transaction_id": identifier}), Call: func(ctx context.Context, a arguments) (json.RawMessage, error) {
			return client.GetTransaction(ctx, a.string("plan_id"), a.string("transaction_id"))
		}},
	}
}
