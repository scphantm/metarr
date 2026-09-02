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
// (AdminService, ApiKeyService, SonarrInterfaceService, …), so ADR-0001 is
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
	// The response is a clone: it must carry blanked admin credentials, and
	// live config holds the running server's own password hash, so blanking
	// in place would erase it and lock the administrator out until the next
	// reload. AdminService.UpdateAdminUser is the only write path for a new
	// password (docs/adr/0005).
	response := cloneMsg(appconfig.Get())
	if response.Admin != nil {
		response.Admin.PasswordSalt = ""
		response.Admin.PasswordHash = ""
	}

	return connect.NewResponse(&metarrv1.GetConfigResponse{
		Config: response,
	}), nil
}
