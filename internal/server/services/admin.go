package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"connectrpc.com/connect"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/httpserver"
	"Metarr/internal/server/passwordhash"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
)

// AdminServer implements metarrv1connect.AdminServiceHandler: the single
// administrative account on AIP standard methods (docs/adr/0010). Reads come
// from live config with the credential fields blanked; the write goes
// through AppConfigStore.MutateSync, which persists under the store lock and
// propagates in-process before returning (docs/adr/0002).
type AdminServer struct {
	*handlers.Handlers
}

// AdminAuthPolicies is this service's method-name -> policy map. Every route
// is GroupConfig; the read is read-only.
var AdminAuthPolicies = map[string]httpserver.RPCPolicy{
	"GetAdminUser":    {Group: auth.GroupConfig, ReadOnly: true},
	"UpdateAdminUser": {Group: auth.GroupConfig},
}

// adminIdentityFields is the set of paths UpdateAdminUser's update_mask may
// name. The credential never travels in the mask (docs/adr/0005) — a new
// password rides new_password instead.
var adminIdentityFields = map[string]bool{"username": true, "email": true}

// blankAdminCredential clears the stored credential fields on a clone before
// it goes out on the wire (docs/adr/0005).
func blankAdminCredential(admin *appconfig.AdminUser) *appconfig.AdminUser {
	out := cloneMsg(admin)
	if out != nil {
		out.PasswordSalt = ""
		out.PasswordHash = ""
	}
	return out
}

func (s *AdminServer) GetAdminUser(
	ctx context.Context,
	req *connect.Request[metarrv1.GetAdminUserRequest],
) (*connect.Response[metarrv1.AdminUser], error) {
	admin := appconfig.Get().GetAdmin()
	if admin == nil {
		admin = &appconfig.AdminUser{}
	}
	return connect.NewResponse(blankAdminCredential(admin)), nil
}

// UpdateAdminUser is an AIP-134 partial update of the identity fields plus an
// out-of-band new_password. The mask may name only username / email; a set
// field that is explicitly empty is rejected rather than silently clearing
// the value. new_password is never masked and is acted on only when
// non-empty. A request carrying only new_password (empty mask) is allowed.
func (s *AdminServer) UpdateAdminUser(
	ctx context.Context,
	req *connect.Request[metarrv1.UpdateAdminUserRequest],
) (*connect.Response[metarrv1.AdminUser], error) {
	correlationID := correlation.FromContext(ctx)

	patch := req.Msg.GetAdmin()
	newPassword := req.Msg.GetNewPassword()
	maskPaths := req.Msg.GetUpdateMask().GetPaths()

	if len(maskPaths) == 0 && newPassword == "" {
		return nil, connectError(http.StatusBadRequest,
			errors.New("update_mask must name at least one field, or new_password must be set"))
	}
	for _, path := range maskPaths {
		if !adminIdentityFields[path] {
			return nil, connectError(http.StatusBadRequest,
				fmt.Errorf("%w: the admin update_mask may name only username or email", errUnknownPath))
		}
	}
	if patch == nil && len(maskPaths) > 0 {
		return nil, connectError(http.StatusBadRequest, errors.New("admin is required when update_mask names a field"))
	}
	for _, path := range maskPaths {
		if path == "username" && patch.GetUsername() == "" {
			return nil, connectError(http.StatusBadRequest, errors.New("username cannot be empty"))
		}
		if path == "email" && patch.GetEmail() == "" {
			return nil, connectError(http.StatusBadRequest, errors.New("email cannot be empty"))
		}
	}

	var salt, hash string
	if newPassword != "" {
		var err error
		salt, hash, err = passwordhash.Hash(newPassword)
		if err != nil {
			s.Logger.Error("failed to hash password", "correlation_id", correlationID, "error", err)
			return nil, connectError(http.StatusInternalServerError, errors.New("failed to update admin credentials"))
		}
	}

	var stored *appconfig.AdminUser
	err := s.AppConfigStore.MutateSync(ctx, func(cfg *appconfig.Config) error {
		if cfg.Admin == nil {
			cfg.Admin = &appconfig.AdminUser{}
		}
		// Clone-and-mask like the sibling services (AGENTS.md CRUD rule): the
		// identity fields ride the FieldMask through applyUpdateMask, the
		// credential is never the mask's to move and is carried over from the
		// stored copy, then replaced only for a non-empty new_password.
		merged := cloneMsg(cfg.Admin)
		if len(maskPaths) > 0 {
			if err := applyUpdateMask(merged, patch, req.Msg.GetUpdateMask()); err != nil {
				return err
			}
		}
		merged.PasswordSalt = cfg.Admin.PasswordSalt
		merged.PasswordHash = cfg.Admin.PasswordHash
		if newPassword != "" {
			merged.PasswordSalt = salt
			merged.PasswordHash = hash
		}
		cfg.Admin = merged
		stored = blankAdminCredential(merged)
		return nil
	})
	if err != nil {
		return nil, mutateConfigErr(s.Logger, correlationID, err)
	}
	return connect.NewResponse(stored), nil
}
