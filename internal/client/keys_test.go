package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListKeys(t *testing.T) {
	var capturedPath string
	var capturedQuery string
	var capturedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		capturedAuth = r.Header.Get("Authorization")

		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"keys":[{
			"id":1,
			"app":"testapp",
			"kind":"secret",
			"prefix":"courrier_test_123",
			"daily_quota":100,
			"used_today":5,
			"created_at":"2026-09-01T12:00:00Z"
		}]}`)
	}))
	defer server.Close()

	c := New(server.URL, "tok-abc")
	keys, err := c.ListKeys(context.Background(), "")
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}

	if capturedPath != "/api/apikeys" {
		t.Errorf("path = %q, want /api/apikeys", capturedPath)
	}
	if capturedQuery != "" {
		t.Errorf("query = %q, want empty", capturedQuery)
	}
	if capturedAuth != "Bearer tok-abc" {
		t.Errorf("Authorization = %q, want Bearer tok-abc", capturedAuth)
	}
	if len(keys) != 1 {
		t.Fatalf("len(keys) = %d, want 1", len(keys))
	}
	first := keys[0]
	if first.ID != 1 || first.App != "testapp" || first.Kind != "secret" || first.DailyQuota != 100 {
		t.Errorf("first = %+v, unexpected fields", first)
	}
	if first.UsedToday != 5 {
		t.Errorf("UsedToday = %d, want 5", first.UsedToday)
	}
}

func TestListKeysWithAppFilter(t *testing.T) {
	var capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"keys":[]}`)
	}))
	defer server.Close()

	c := New(server.URL, "tok-abc")
	keys, err := c.ListKeys(context.Background(), "myapp")
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}

	if capturedQuery != "app=myapp" {
		t.Errorf("query = %q, want app=myapp", capturedQuery)
	}
	if keys == nil || len(keys) != 0 {
		t.Errorf("keys = %v, want empty non-nil slice", keys)
	}
}

func TestCreateKey(t *testing.T) {
	var capturedPath string
	var capturedMethod string
	var capturedBody CreateKeyRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedMethod = r.Method
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"key":{
			"id":42,
			"app":"web",
			"kind":"public",
			"prefix":"courrier_pub_web_",
			"allowed_origins":["https://example.com"],
			"daily_quota":500,
			"created_at":"2026-09-01T12:00:00Z"
		},"token":"courrier_pub_web_secret_token_123"}`)
	}))
	defer server.Close()

	c := New(server.URL, "tok-abc")
	res, err := c.CreateKey(context.Background(), CreateKeyRequest{
		App:            "web",
		Kind:           "public",
		AllowedOrigins: []string{"https://example.com"},
		DailyQuota:     500,
	})
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}

	if capturedPath != "/api/apikeys" {
		t.Errorf("path = %q, want /api/apikeys", capturedPath)
	}
	if capturedMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", capturedMethod)
	}
	if capturedBody.App != "web" || capturedBody.Kind != "public" || capturedBody.DailyQuota != 500 {
		t.Errorf("capturedBody = %+v, unexpected values", capturedBody)
	}
	if len(capturedBody.AllowedOrigins) != 1 || capturedBody.AllowedOrigins[0] != "https://example.com" {
		t.Errorf("AllowedOrigins = %v, want [https://example.com]", capturedBody.AllowedOrigins)
	}
	if res.Token != "courrier_pub_web_secret_token_123" {
		t.Errorf("res.Token = %q, want courrier_pub_web_secret_token_123", res.Token)
	}
	if res.Key.ID != 42 || res.Key.App != "web" {
		t.Errorf("res.Key = %+v, unexpected values", res.Key)
	}
}

func TestRevokeKey(t *testing.T) {
	var capturedPath string
	var capturedMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := New(server.URL, "tok-abc")
	err := c.RevokeKey(context.Background(), 99)
	if err != nil {
		t.Fatalf("RevokeKey: %v", err)
	}

	if capturedPath != "/api/apikeys/99" {
		t.Errorf("path = %q, want /api/apikeys/99", capturedPath)
	}
	if capturedMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", capturedMethod)
	}
}

func TestRevokeKeyError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"error":{"code":"not_found","message":"API key not found"}}`)
	}))
	defer server.Close()

	c := New(server.URL, "tok-abc")
	err := c.RevokeKey(context.Background(), 123)
	if err == nil {
		t.Fatal("RevokeKey want error, got nil")
	}

	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v (%T), want *Error", err, err)
	}
	if !apiErr.NotFound() {
		t.Errorf("NotFound() = false, want true")
	}
	if apiErr.Code != "not_found" || apiErr.Message != "API key not found" {
		t.Errorf("got code %q message %q, want not_found / API key not found", apiErr.Code, apiErr.Message)
	}
}
