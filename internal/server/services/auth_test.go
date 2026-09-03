package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/handlers"
	"Metarr/internal/shared/appconfig"
)

// GetAuthScheme is the pre-login probe (docs/adr/0012): it reads the scheme
// from live config and succeeds with no credential on the context, so the UI
// can decide the render gate on a cold load.
func TestGetAuthScheme_ReturnsTheConfiguredScheme(t *testing.T) {
	for _, tc := range []struct {
		name string
		want metarrv1.AuthenticationScheme
	}{
		{"none", appconfig.AuthSchemeNone},
		{"password", appconfig.AuthSchemePassword},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withLiveConfig(t, &appconfig.Config{
				Admin: &appconfig.AdminUser{AuthenticationScheme: tc.want},
			})
			server := &AuthServer{Handlers: &handlers.Handlers{}}

			resp, err := server.GetAuthScheme(context.Background(),
				connect.NewRequest(&metarrv1.AuthServiceGetAuthSchemeRequest{}))
			if err != nil {
				t.Fatalf("GetAuthScheme: %v", err)
			}
			if resp.Msg.GetScheme() != tc.want {
				t.Fatalf("scheme = %v, want %v", resp.Msg.GetScheme(), tc.want)
			}
		})
	}
}

// A config that never named a scheme normalises to None on the way into live
// config, so the probe answers None rather than UNSPECIFIED.
func TestGetAuthScheme_NormalisesAnUnsetScheme(t *testing.T) {
	withLiveConfig(t, &appconfig.Config{})
	server := &AuthServer{Handlers: &handlers.Handlers{}}

	resp, err := server.GetAuthScheme(context.Background(),
		connect.NewRequest(&metarrv1.AuthServiceGetAuthSchemeRequest{}))
	if err != nil {
		t.Fatalf("GetAuthScheme: %v", err)
	}
	if resp.Msg.GetScheme() != appconfig.AuthSchemeNone {
		t.Fatalf("scheme = %v, want None", resp.Msg.GetScheme())
	}
}

// The probe is a NoAuth RPC: its policy entry must say so, since that is what
// lets the UI call it before any credential exists.
func TestAuthPolicies_GetAuthSchemeIsNoAuth(t *testing.T) {
	policy, ok := AuthAuthPolicies["GetAuthScheme"]
	if !ok {
		t.Fatal("no auth policy registered for GetAuthScheme")
	}
	if !policy.NoAuth {
		t.Errorf("GetAuthScheme policy = %+v, want NoAuth", policy)
	}
}
