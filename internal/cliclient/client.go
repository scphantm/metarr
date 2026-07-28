// Package cliclient is a thin HTTP client for the Metarr API, used by
// cmd/metarrctl. It shares request/response types directly with
// internal/handlers and internal/appconfig rather than redefining them, so
// the wire contract can't drift between the server and this client.
package cliclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"Metarr/internal/appconfig"
	"Metarr/internal/handlers"
)

const apiKeyHeaderName = "X-Api-Key"

// Client talks to a running Metarr API server.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New constructs a Client for the server at baseURL, authenticating every
// request with apiKey (which may be empty for the unauthenticated
// endpoints — Heartbeat and Login).
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{},
	}
}

// APIError is returned when the server responds with a non-2xx status. The
// server's handlers report failures as a plain-text body via http.Error,
// so Message is that body verbatim.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("api error (%d): %s", e.StatusCode, e.Message)
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("cliclient: encoding request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("cliclient: building request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set(apiKeyHeaderName, c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cliclient: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("cliclient: reading response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Message: strings.TrimSpace(string(respBody))}
	}

	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("cliclient: decoding response: %w", err)
	}
	return nil
}

// Heartbeat calls GET /api/heartbeat.
func (c *Client) Heartbeat(ctx context.Context) (*handlers.HeartbeatResponse, error) {
	var resp handlers.HeartbeatResponse
	if err := c.do(ctx, http.MethodGet, "/api/heartbeat", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Login calls POST /api/auth/login.
func (c *Client) Login(ctx context.Context, username, password string) (*handlers.LoginResponse, error) {
	var resp handlers.LoginResponse
	req := handlers.LoginRequest{Username: username, Password: password}
	if err := c.do(ctx, http.MethodPost, "/api/auth/login", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Logout calls POST /api/auth/logout, revoking the session key configured
// on this Client.
func (c *Client) Logout(ctx context.Context) (*handlers.LogoutResponse, error) {
	var resp handlers.LogoutResponse
	if err := c.do(ctx, http.MethodPost, "/api/auth/logout", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetConfig calls GET /api/config.
func (c *Client) GetConfig(ctx context.Context) (*appconfig.Config, error) {
	var resp appconfig.Config
	if err := c.do(ctx, http.MethodGet, "/api/config", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SetConfig calls PUT /api/config, replacing the whole config document.
func (c *Client) SetConfig(ctx context.Context, config appconfig.Config) (*handlers.AcceptedResponse, error) {
	var resp handlers.AcceptedResponse
	if err := c.do(ctx, http.MethodPut, "/api/config", config, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UpdateAdmin calls PUT /api/config/admin.
func (c *Client) UpdateAdmin(ctx context.Context, req handlers.UpdateAdminRequest) (*handlers.AcceptedResponse, error) {
	var resp handlers.AcceptedResponse
	if err := c.do(ctx, http.MethodPut, "/api/config/admin", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListSonarrInterfaces calls GET /api/config/interfaces/sonarr.
func (c *Client) ListSonarrInterfaces(ctx context.Context) ([]appconfig.SonarrInstance, error) {
	var resp []appconfig.SonarrInstance
	if err := c.do(ctx, http.MethodGet, "/api/config/interfaces/sonarr", nil, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetSonarrInterface calls GET /api/config/interfaces/sonarr/{slug}.
func (c *Client) GetSonarrInterface(ctx context.Context, slug string) (*appconfig.SonarrInstance, error) {
	var resp appconfig.SonarrInstance
	if err := c.do(ctx, http.MethodGet, "/api/config/interfaces/sonarr/"+url.PathEscape(slug), nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateSonarrInterface calls POST /api/config/interfaces/sonarr.
func (c *Client) CreateSonarrInterface(ctx context.Context, instance appconfig.SonarrInstance) (*handlers.AcceptedResponse, error) {
	var resp handlers.AcceptedResponse
	if err := c.do(ctx, http.MethodPost, "/api/config/interfaces/sonarr", instance, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UpdateSonarrInterface calls PUT /api/config/interfaces/sonarr/{slug}.
func (c *Client) UpdateSonarrInterface(ctx context.Context, slug string, instance appconfig.SonarrInstance) (*handlers.AcceptedResponse, error) {
	var resp handlers.AcceptedResponse
	if err := c.do(ctx, http.MethodPut, "/api/config/interfaces/sonarr/"+url.PathEscape(slug), instance, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DeleteSonarrInterface calls DELETE /api/config/interfaces/sonarr/{slug}.
func (c *Client) DeleteSonarrInterface(ctx context.Context, slug string) (*handlers.AcceptedResponse, error) {
	var resp handlers.AcceptedResponse
	if err := c.do(ctx, http.MethodDelete, "/api/config/interfaces/sonarr/"+url.PathEscape(slug), nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// TriggerSonarrCacheData calls POST /api/tasks/sonarr_cache_data.
func (c *Client) TriggerSonarrCacheData(ctx context.Context) (*handlers.AcceptedResponse, error) {
	var resp handlers.AcceptedResponse
	req := handlers.TaskRequest{Command: "run"}
	if err := c.do(ctx, http.MethodPost, "/api/tasks/sonarr_cache_data", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
