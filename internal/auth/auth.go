// Package auth implements API key based authorization: resolving a
// presented key to the role that issued it, and deciding whether that role
// may access a given router group and HTTP method.
package auth

import (
	"context"
	"net/http"

	"Metarr/internal/appconfig"
)

// Role identifies which api_keys category a resolved API key belongs to.
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleUser     Role = "user"
	RoleWebhook  Role = "webhook"
	RoleReadOnly Role = "read_only"
)

// Group identifies a group of routes for authorization purposes.
type Group string

const (
	GroupConfig  Group = "config"
	GroupTasks   Group = "tasks"
	GroupWebhook Group = "webhook"
)

// allowedGroups lists, for each role, which router groups it may access at
// all, independent of HTTP method.
var allowedGroups = map[Role]map[Group]bool{
	RoleAdmin:    {GroupConfig: true, GroupTasks: true, GroupWebhook: true},
	RoleUser:     {GroupTasks: true, GroupWebhook: true},
	RoleReadOnly: {GroupTasks: true, GroupWebhook: true},
	RoleWebhook:  {GroupWebhook: true},
}

// Resolve looks up apiKey against the configured API key categories,
// returning the matching role. ok is false if apiKey is empty or doesn't
// match any configured key.
func Resolve(config *appconfig.Config, apiKey string) (role Role, ok bool) {
	if apiKey == "" {
		return "", false
	}

	for _, entry := range config.APIKeys.Admin {
		if entry.Key == apiKey {
			return RoleAdmin, true
		}
	}
	for _, entry := range config.APIKeys.User {
		if entry.Key == apiKey {
			return RoleUser, true
		}
	}
	for _, entry := range config.APIKeys.Webhook {
		if entry.Key == apiKey {
			return RoleWebhook, true
		}
	}
	for _, entry := range config.APIKeys.ReadOnly {
		if entry.Key == apiKey {
			return RoleReadOnly, true
		}
	}

	return "", false
}

// Authorized reports whether role may perform method against group.
// RoleReadOnly is additionally restricted to GET regardless of group; every
// other role that can access a group at all may use any method within it.
func Authorized(role Role, group Group, method string) bool {
	if !allowedGroups[role][group] {
		return false
	}
	if role == RoleReadOnly && method != http.MethodGet {
		return false
	}
	return true
}

type apiKeyContextKey struct{}

// WithAPIKey returns a new context carrying the API key that authenticated
// the current request.
func WithAPIKey(ctx context.Context, apiKey string) context.Context {
	return context.WithValue(ctx, apiKeyContextKey{}, apiKey)
}

// APIKeyFromContext returns the API key stored in ctx, or "" if none is
// set.
func APIKeyFromContext(ctx context.Context) string {
	apiKey, _ := ctx.Value(apiKeyContextKey{}).(string)
	return apiKey
}
