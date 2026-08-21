package loopback

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

type waitResult struct {
	code string
	err  error
}

func startWait(ctx context.Context, l *Listener, state string) <-chan waitResult {
	results := make(chan waitResult, 1)
	go func() {
		code, err := l.WaitForCode(ctx, state)
		results <- waitResult{code: code, err: err}
	}()
	return results
}

func callback(t *testing.T, l *Listener, path string) int {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d%s", l.Port(), path))
	if err != nil {
		t.Fatalf("callback %s failed: %v", path, err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	return response.StatusCode
}

func TestLoginURLMatchesPorteContract(t *testing.T) {
	listener, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	parsed, err := url.Parse(listener.LoginURL("https://courrier.facile.studio", "deadbeef"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "https" || parsed.Host != "courrier.facile.studio" {
		t.Fatalf("origin = %s://%s, want https://courrier.facile.studio", parsed.Scheme, parsed.Host)
	}
	if parsed.Path != "/api/auth/oidc" {
		t.Errorf("path = %q, want /api/auth/oidc", parsed.Path)
	}
	query := parsed.Query()
	if query.Get("flow") != "cli" {
		t.Errorf("flow = %q, want cli", query.Get("flow"))
	}
	if query.Get("cli_state") != "deadbeef" {
		t.Errorf("cli_state = %q, want deadbeef", query.Get("cli_state"))
	}
	if query.Get("port") != strconv.Itoa(listener.Port()) {
		t.Errorf("port = %q, want %d", query.Get("port"), listener.Port())
	}
	if listener.Port() == 0 {
		t.Error("port = 0, want the real bound port")
	}
}

func TestLoginURLToleratesTrailingSlash(t *testing.T) {
	listener, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	raw := listener.LoginURL("https://courrier.facile.studio/", "deadbeef")
	if !strings.HasPrefix(raw, "https://courrier.facile.studio/api/auth/oidc?") {
		t.Fatalf("url = %q, want a single slash before /api", raw)
	}
}

func TestRandomStateFitsPorteBounds(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 64; i++ {
		state, err := RandomState()
		if err != nil {
			t.Fatal(err)
		}
		if state == "" || len(state) > 128 {
			t.Fatalf("state %q has length %d, want 1..128", state, len(state))
		}
		if strings.Trim(state, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_") != "" {
			t.Fatalf("state %q leaves porte's [A-Za-z0-9-_] alphabet", state)
		}
		if seen[state] {
			t.Fatalf("state %q repeated", state)
		}
		seen[state] = true
	}
}

func TestWaitForCodeReturnsTheCode(t *testing.T) {
	listener, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := startWait(ctx, listener, "expected")

	if status := callback(t, listener, "/?code=abc&state=expected"); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	select {
	case got := <-results:
		if got.err != nil {
			t.Fatal(got.err)
		}
		if got.code != "abc" {
			t.Fatalf("code = %q, want abc", got.code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the code")
	}
}

func TestWaitForCodeRejectsAMismatchedStateAndKeepsListening(t *testing.T) {
	listener, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := startWait(ctx, listener, "expected")

	if status := callback(t, listener, "/?code=stolen&state=wrong"); status != http.StatusBadRequest {
		t.Fatalf("status for a mismatched state = %d, want 400", status)
	}
	select {
	case got := <-results:
		t.Fatalf("wait ended on a mismatched state: code=%q err=%v", got.code, got.err)
	case <-time.After(200 * time.Millisecond):
	}

	if status := callback(t, listener, "/?code=abc&state=expected"); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	select {
	case got := <-results:
		if got.err != nil {
			t.Fatal(got.err)
		}
		if got.code != "abc" {
			t.Fatalf("code = %q, want abc", got.code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the code after the rejected callback")
	}
}

func TestWaitForCodeIgnoresFavicon(t *testing.T) {
	listener, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := startWait(ctx, listener, "expected")

	if status := callback(t, listener, "/favicon.ico"); status != http.StatusNotFound {
		t.Fatalf("status for /favicon.ico = %d, want 404", status)
	}
	select {
	case got := <-results:
		t.Fatalf("wait ended on /favicon.ico: code=%q err=%v", got.code, got.err)
	case <-time.After(200 * time.Millisecond):
	}

	if status := callback(t, listener, "/?code=abc&state=expected"); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	select {
	case got := <-results:
		if got.err != nil {
			t.Fatal(got.err)
		}
		if got.code != "abc" {
			t.Fatalf("code = %q, want abc", got.code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the code after /favicon.ico")
	}
}

func TestWaitForCodeStopsOnACancelledContext(t *testing.T) {
	listener, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	results := startWait(ctx, listener, "expected")

	started := time.Now()
	time.AfterFunc(50*time.Millisecond, cancel)

	select {
	case got := <-results:
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", got.err)
		}
		if elapsed := time.Since(started); elapsed > 3*time.Second {
			t.Fatalf("returned after %s, want promptly", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelling the context did not end the wait")
	}
}

func TestCloseIsSafeAfterWaitForCode(t *testing.T) {
	listener, err := Listen()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := listener.WaitForCode(ctx, "expected"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("Close after WaitForCode = %v, want nil", err)
	}
}
