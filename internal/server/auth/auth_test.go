package auth

import (
	"net/http"
	"testing"
)

// Authorized is the whole authorization decision now that the role arrives
// pre-resolved from the JWT claims: a role/group gate plus the read-only
// role's GET-only restriction. This pins that matrix.
func TestAuthorized(t *testing.T) {
	cases := []struct {
		role   Role
		group  Group
		method string
		want   bool
	}{
		{RoleAdmin, GroupConfig, http.MethodPost, true},
		{RoleAdmin, GroupTasks, http.MethodPost, true},
		{RoleAdmin, GroupWebhook, http.MethodPost, true},
		{RoleUser, GroupConfig, http.MethodPost, false},
		{RoleUser, GroupTasks, http.MethodPost, true},
		{RoleUser, GroupWebhook, http.MethodPost, true},
		{RoleReadOnly, GroupTasks, http.MethodGet, true},
		{RoleReadOnly, GroupTasks, http.MethodPost, false},
		{RoleReadOnly, GroupConfig, http.MethodGet, false},
		{RoleWebhook, GroupWebhook, http.MethodPost, true},
		{RoleWebhook, GroupTasks, http.MethodPost, false},
		{Role("bogus"), GroupTasks, http.MethodGet, false},
	}

	for _, tc := range cases {
		if got := Authorized(tc.role, tc.group, tc.method); got != tc.want {
			t.Errorf("Authorized(%q, %q, %s) = %v, want %v", tc.role, tc.group, tc.method, got, tc.want)
		}
	}
}
