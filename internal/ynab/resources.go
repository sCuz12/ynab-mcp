package ynab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

func planPath(planID string) (string, error) {
	id, err := segment("plan_id", planID)
	if err != nil {
		return "", err
	}
	return "/plans/" + id, nil
}

func childPath(planID, collection, idName, id string) (string, error) {
	plan, err := planPath(planID)
	if err != nil {
		return "", err
	}
	child, err := segment(idName, id)
	if err != nil {
		return "", err
	}
	return plan + "/" + collection + "/" + child, nil
}

func knowledgeQuery(knowledge *int64) (url.Values, error) {
	if err := ValidateKnowledge(knowledge); err != nil {
		return nil, err
	}
	query := make(url.Values)
	if knowledge != nil {
		query.Set("last_knowledge_of_server", strconv.FormatInt(*knowledge, 10))
	}
	return query, nil
}

func (c *Client) ListPlans(ctx context.Context) (json.RawMessage, error) {
	return c.request(ctx, http.MethodGet, "/plans", nil, nil)
}

func (c *Client) GetPlan(ctx context.Context, planID string, knowledge *int64) (json.RawMessage, error) {
	path, err := planPath(planID)
	if err != nil {
		return nil, err
	}
	query, err := knowledgeQuery(knowledge)
	if err != nil {
		return nil, err
	}
	return c.request(ctx, http.MethodGet, path, query, nil)
}

func (c *Client) ListAccounts(ctx context.Context, planID string, knowledge *int64) (json.RawMessage, error) {
	path, err := planPath(planID)
	if err != nil {
		return nil, err
	}
	query, err := knowledgeQuery(knowledge)
	if err != nil {
		return nil, err
	}
	return c.request(ctx, http.MethodGet, path+"/accounts", query, nil)
}

func (c *Client) GetAccount(ctx context.Context, planID, accountID string) (json.RawMessage, error) {
	path, err := childPath(planID, "accounts", "account_id", accountID)
	if err != nil {
		return nil, err
	}
	return c.request(ctx, http.MethodGet, path, nil, nil)
}

func (c *Client) ListCategories(ctx context.Context, planID string, knowledge *int64) (json.RawMessage, error) {
	path, err := planPath(planID)
	if err != nil {
		return nil, err
	}
	query, err := knowledgeQuery(knowledge)
	if err != nil {
		return nil, err
	}
	return c.request(ctx, http.MethodGet, path+"/categories", query, nil)
}

func (c *Client) GetCategory(ctx context.Context, planID, categoryID string) (json.RawMessage, error) {
	path, err := childPath(planID, "categories", "category_id", categoryID)
	if err != nil {
		return nil, err
	}
	return c.request(ctx, http.MethodGet, path, nil, nil)
}

func (c *Client) ListPayees(ctx context.Context, planID string, knowledge *int64) (json.RawMessage, error) {
	path, err := planPath(planID)
	if err != nil {
		return nil, err
	}
	query, err := knowledgeQuery(knowledge)
	if err != nil {
		return nil, err
	}
	return c.request(ctx, http.MethodGet, path+"/payees", query, nil)
}

func (c *Client) ListMonths(ctx context.Context, planID string, knowledge *int64) (json.RawMessage, error) {
	path, err := planPath(planID)
	if err != nil {
		return nil, err
	}
	query, err := knowledgeQuery(knowledge)
	if err != nil {
		return nil, err
	}
	return c.request(ctx, http.MethodGet, path+"/months", query, nil)
}

func (c *Client) GetMonth(ctx context.Context, planID, month string) (json.RawMessage, error) {
	if err := ValidateMonth(month); err != nil {
		return nil, err
	}
	path, err := childPath(planID, "months", "month", month)
	if err != nil {
		return nil, err
	}
	return c.request(ctx, http.MethodGet, path, nil, nil)
}
