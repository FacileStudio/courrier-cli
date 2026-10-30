package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// Template is a reusable message skeleton.
type Template struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Subject   string `json:"subject"`
	BodyHTML  string `json:"body_html"`
	BodyText  string `json:"body_text"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// TemplateInput is the body of a template create or update. An update replaces
// the subject and both bodies wholesale and keeps the name when Name is empty,
// so callers merge with the stored template first. SpaceID scopes a new
// template to a space; leave it empty for a personal one.
type TemplateInput struct {
	Name     string  `json:"name"`
	Subject  string  `json:"subject"`
	BodyHTML string  `json:"body_html"`
	BodyText string  `json:"body_text"`
	SpaceID  *string `json:"space_id"`
}

// Templates lists the user's templates, narrowed to one space when spaceID is
// not empty.
func (c *Client) Templates(ctx context.Context, spaceID string) ([]Template, error) {
	query := url.Values{}
	if spaceID != "" {
		query.Set("space_id", spaceID)
	}
	var out struct {
		Templates []Template `json:"templates"`
	}
	err := c.do(ctx, http.MethodGet, withQuery("/api/templates/", query), nil, &out)
	return out.Templates, err
}

// CreateTemplate stores a new template. The instance allows fifty per user.
func (c *Client) CreateTemplate(ctx context.Context, in TemplateInput) (Template, error) {
	var out Template
	err := c.do(ctx, http.MethodPost, "/api/templates/", in, &out)
	return out, err
}

// UpdateTemplate replaces a template's subject and bodies and returns it.
func (c *Client) UpdateTemplate(ctx context.Context, id int64, in TemplateInput) (Template, error) {
	var out Template
	err := c.do(ctx, http.MethodPut, "/api/templates/"+strconv.FormatInt(id, 10), in, &out)
	return out, err
}

// DeleteTemplate removes a template.
func (c *Client) DeleteTemplate(ctx context.Context, id int64) error {
	return c.do(ctx, http.MethodDelete, "/api/templates/"+strconv.FormatInt(id, 10), nil, nil)
}
