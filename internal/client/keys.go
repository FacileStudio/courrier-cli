package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// Key is one registered API key record returned by the instance.
type Key struct {
	ID             int64    `json:"id"`
	App            string   `json:"app"`
	Kind           string   `json:"kind"`
	Prefix         string   `json:"prefix"`
	AllowedOrigins []string `json:"allowed_origins,omitempty"`
	DailyQuota     int      `json:"daily_quota,omitempty"`
	UsedToday      int64    `json:"used_today,omitempty"`
	CreatedAt      string   `json:"created_at"`
	RevokedAt      *string  `json:"revoked_at,omitempty"`
}

// CreateKeyRequest carries parameters to mint a new API key.
type CreateKeyRequest struct {
	App            string   `json:"app"`
	Kind           string   `json:"kind"`
	AllowedOrigins []string `json:"allowed_origins,omitempty"`
	DailyQuota     int      `json:"daily_quota,omitempty"`
}

// CreateKeyResponse returns the created key metadata and the raw one-time token.
type CreateKeyResponse struct {
	Key   Key    `json:"key"`
	Token string `json:"token"`
}

// ListKeysResponse is the envelope returned by the API key listing endpoint.
type ListKeysResponse struct {
	Keys []Key `json:"keys"`
}

// ListKeys lists the API keys registered on the instance, optionally filtered by app name.
func (c *Client) ListKeys(ctx context.Context, app string) ([]Key, error) {
	query := url.Values{}
	if app != "" {
		query.Set("app", app)
	}
	var out ListKeysResponse
	err := c.do(ctx, http.MethodGet, withQuery("/api/apikeys", query), nil, &out)
	if err != nil {
		return nil, err
	}
	if out.Keys == nil {
		return []Key{}, nil
	}
	return out.Keys, nil
}

// CreateKey mints a new API key for an application.
func (c *Client) CreateKey(ctx context.Context, req CreateKeyRequest) (CreateKeyResponse, error) {
	var out CreateKeyResponse
	err := c.do(ctx, http.MethodPost, "/api/apikeys", req, &out)
	return out, err
}

// RevokeKey revokes an API key by its numeric id.
func (c *Client) RevokeKey(ctx context.Context, id int64) error {
	path := "/api/apikeys/" + strconv.FormatInt(id, 10)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}
