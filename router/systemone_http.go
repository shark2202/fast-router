package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const systemOneErrorBodyLimit = 4096

// SystemOneHTTPError describes a non-success response from a remote
// System One engine.
type SystemOneHTTPError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *SystemOneHTTPError) Error() string {
	if e == nil {
		return "system-one HTTP error"
	}
	if e.Body == "" {
		return fmt.Sprintf("system-one HTTP error: %s", e.Status)
	}
	return fmt.Sprintf("system-one HTTP error: %s: %s", e.Status, e.Body)
}

// HTTPSystemOneClient implements SystemOneEngine by forwarding complete
// System One requests to a compatible /v1/systemone service.
type HTTPSystemOneClient struct {
	endpoint string
	token    string
	client   *http.Client
}

func NewHTTPSystemOneClient(endpoint, token string, client *http.Client) (*HTTPSystemOneClient, error) {
	normalized, err := normalizeSystemOneEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPSystemOneClient{
		endpoint: normalized,
		token:    token,
		client:   client,
	}, nil
}

func (c *HTTPSystemOneClient) Evaluate(ctx context.Context, req System1Request) (System1Response, error) {
	if err := ctx.Err(); err != nil {
		return System1Response{}, err
	}
	if c == nil || c.client == nil || c.endpoint == "" {
		return System1Response{}, ErrSystemOneEngineUnavailable
	}

	body, err := json.Marshal(req)
	if err != nil {
		return System1Response{}, fmt.Errorf("encode System One request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return System1Response{}, fmt.Errorf("build System One request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return System1Response{}, fmt.Errorf("System One request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		errorBody, readErr := io.ReadAll(io.LimitReader(resp.Body, systemOneErrorBodyLimit))
		if readErr != nil {
			return System1Response{}, fmt.Errorf("read System One error response: %w", readErr)
		}
		return System1Response{}, &SystemOneHTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       strings.TrimSpace(string(errorBody)),
		}
	}

	var result System1Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return System1Response{}, fmt.Errorf("decode System One response: %w", err)
	}
	return result, nil
}

func normalizeSystemOneEndpoint(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", errors.New("system-one endpoint is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse system-one endpoint: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("system-one endpoint must use http or https")
	}
	if u.Host == "" {
		return "", errors.New("system-one endpoint has no host")
	}
	if u.RawQuery != "" {
		return "", errors.New("system-one endpoint must not include a query")
	}
	if u.Fragment != "" {
		return "", errors.New("system-one endpoint must not include a fragment")
	}

	path := strings.TrimRight(u.Path, "/")
	switch {
	case path == "":
		path = "/v1/systemone"
	case path == "/v1":
		path = "/v1/systemone"
	case strings.HasSuffix(path, "/v1/systemone"):
		// Already normalized.
	default:
		path += "/v1/systemone"
	}
	u.Path = path
	u.RawPath = ""
	return u.String(), nil
}
