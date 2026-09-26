package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPSystemOneClientPostsToSystemOneEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/systemone" {
			t.Errorf("path = %s, want /v1/systemone", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content type = %q, want application/json", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("authorization = %q, want bearer token", got)
		}

		var req System1Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req.State != "hello" {
			t.Errorf("state = %q, want hello", req.State)
		}
		q, ok := req.Questions["task_type"]
		if !ok {
			t.Fatal("task_type question missing")
		}
		if q.Type != "choice" || q.Instructions != "pick one" || q.Criteria["a"] != "first" {
			t.Fatalf("question = %#v", q)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"model":"remote","answers":{"task_type":{"type":"choice","choice":"a","probabilities":{"a":0.9,"b":0.1},"confidence":0.9}},"usage":{"input_tokens":3,"output_tokens":1}}`)
	}))
	defer server.Close()

	client, err := NewHTTPSystemOneClient(server.URL, "test-token", server.Client())
	if err != nil {
		t.Fatalf("NewHTTPSystemOneClient() error = %v", err)
	}

	got, err := client.Evaluate(context.Background(), System1Request{
		State: "hello",
		Questions: map[string]Question{
			"task_type": {
				Type:         "choice",
				Instructions: "pick one",
				Criteria:     map[string]string{"a": "first", "b": "second"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if got.Model != "remote" || got.Answers["task_type"].Choice != "a" {
		t.Fatalf("response = %#v", got)
	}
}

func TestHTTPSystemOneClientDoesNotDuplicateSystemOnePath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" {
			t.Errorf("path = %s, want /v1/systemone", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"model":"remote","answers":{},"usage":{}}`)
	}))
	defer server.Close()

	client, err := NewHTTPSystemOneClient(server.URL+"/v1/systemone", "", server.Client())
	if err != nil {
		t.Fatalf("NewHTTPSystemOneClient() error = %v", err)
	}
	if _, err := client.Evaluate(context.Background(), System1Request{}); err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
}

func TestHTTPSystemOneClientDoesNotSendEmptyBearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("authorization = %q, want empty", got)
		}
		_, _ = fmt.Fprint(w, `{"model":"remote","answers":{},"usage":{}}`)
	}))
	defer server.Close()

	client, err := NewHTTPSystemOneClient(server.URL, "", server.Client())
	if err != nil {
		t.Fatalf("NewHTTPSystemOneClient() error = %v", err)
	}
	if _, err := client.Evaluate(context.Background(), System1Request{}); err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
}

func TestHTTPSystemOneClientReturnsHTTPErrorForNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "sidecar unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	client, err := NewHTTPSystemOneClient(server.URL, "", server.Client())
	if err != nil {
		t.Fatalf("NewHTTPSystemOneClient() error = %v", err)
	}
	_, err = client.Evaluate(context.Background(), System1Request{})
	var remoteErr *SystemOneHTTPError
	if !errors.As(err, &remoteErr) {
		t.Fatalf("error = %v, want SystemOneHTTPError", err)
	}
	if remoteErr.StatusCode != http.StatusBadGateway || !strings.Contains(remoteErr.Body, "sidecar unavailable") {
		t.Fatalf("remote error = %#v", remoteErr)
	}
}

func TestHTTPSystemOneClientReturnsDecodeErrorForInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "{not-json")
	}))
	defer server.Close()

	client, err := NewHTTPSystemOneClient(server.URL, "", server.Client())
	if err != nil {
		t.Fatalf("NewHTTPSystemOneClient() error = %v", err)
	}
	_, err = client.Evaluate(context.Background(), System1Request{})
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("error = %v, want decode error", err)
	}
}

func TestHTTPSystemOneClientHonorsContextDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(200 * time.Millisecond):
			http.Error(w, "handler timeout", http.StatusGatewayTimeout)
		}
	}))
	defer server.Close()

	client, err := NewHTTPSystemOneClient(server.URL, "", server.Client())
	if err != nil {
		t.Fatalf("NewHTTPSystemOneClient() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = client.Evaluate(ctx, System1Request{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}
}

func TestNewHTTPSystemOneClientRejectsInvalidEndpoint(t *testing.T) {
	for _, endpoint := range []string{
		"",
		"://bad",
		"ftp://localhost:8080",
		"http://localhost:8080?query=not-allowed",
		"http://localhost:8080#fragment-not-allowed",
	} {
		t.Run(endpoint, func(t *testing.T) {
			if _, err := NewHTTPSystemOneClient(endpoint, "", nil); err == nil {
				t.Fatalf("NewHTTPSystemOneClient(%q) succeeded, want error", endpoint)
			}
		})
	}
}
