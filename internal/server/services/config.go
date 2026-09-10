package services

import (
	"context"

	"connectrpc.com/connect"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/httpserver"
	"Metarr/internal/shared/appconfig"
)

// ConfigServer implements metarrv1connect.ConfigServiceHandler: the
// read-only aggregate. GetConfig returns the whole Config document so the UI
// can paint every settings screen from one call. It has no write — every
// mutation goes through the per-resource service that owns that section
// (AdminService, SonarrInterfaceService, …), so ADR-0001 is
// untouched (docs/adr/0010).
type ConfigServer struct {
	*handlers.Handlers
}

// ConfigAuthPolicies is this service's method-name -> policy map. The one
// route is a read.
var ConfigAuthPolicies = map[string]httpserver.RPCPolicy{
	"GetConfig": {Group: auth.GroupConfig, ReadOnly: true},
}

func (s *ConfigServer) GetConfig(
	ctx context.Context,
	req *connect.Request[metarrv1.GetConfigRequest],
) (*connect.Response[metarrv1.GetConfigResponse], error) {
	// The response is a clone: it must carry blanked secrets, and live config
	// holds the running server's own password hash and JWT signing secret, so
	// blanking in place would erase them — locking the administrator out and
	// invalidating every issued token until the next reload.
	// AdminService.UpdateAdminUser is the only write path for a new password
	// (docs/adr/0005); the HMAC secret is never client-writable at all.
	response := cloneMsg(appconfig.Get())
	if response.Admin != nil {
		response.Admin.PasswordSalt = ""
		response.Admin.PasswordHash = ""
	}
	if response.Auth != nil {
		// The HMAC secret signs every admin session and integration token;
		// a captured GetConfig response must not be enough to forge one.
		response.Auth.HmacSecret = ""
	}

	return connect.NewResponse(&metarrv1.GetConfigResponse{
		Config: response,
	}), nil
}
