package checkproto

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type closeErrorBody struct {
	io.Reader
	err error
}

func (b closeErrorBody) Close() error { return b.err }

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRequestPreservesResponseCloseFailure(t *testing.T) {
	closeErr := errors.New("fixture close failure")
	for _, decode := range []bool{false, true} {
		c := NewClient("http://127.0.0.1:1", "ws://127.0.0.1:1/api/v1/ws")
		c.HTTP.Transport = fixtureTransport(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: closeErrorBody{Reader: strings.NewReader("invalid-json"), err: closeErr}}, nil
		})
		var out any
		var target any
		if decode {
			target = &out
		}
		err := c.request(context.Background(), http.MethodGet, "/rooms/", nil, http.StatusOK, target)
		if !errors.Is(err, closeErr) {
			t.Fatalf("decode=%t: close error lost: %v", decode, err)
		}
		if decode {
			var syntaxErr *json.SyntaxError
			if !errors.As(err, &syntaxErr) {
				t.Fatalf("decode error lost: %v", err)
			}
		}
	}
}

func TestWriteUniquePreservesExistingArtifact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "summary.json")
	if err := writeUnique(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := writeUnique(path, []byte("replacement")); !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected exclusive creation failure: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "first\n" {
		t.Fatalf("original artifact changed: %q, %v", data, err)
	}
}
