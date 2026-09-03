package services

import (
	"context"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/appconfigstore"
	"Metarr/internal/server/handlers"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
)

func newTestAdminServer(seed *appconfig.Config) (*AdminServer, *fakeConfigBackend) {
	backend := &fakeConfigBackend{cfg: seed}
	store := appconfigstore.New(backend, backend)
	return &AdminServer{Handlers: &handlers.Handlers{
		AppConfigStore: store,
		Logger:         slog.Default(),
	}}, backend
}

func seededAdmin() *appconfig.Config {
	return &appconfig.Config{Admin: &appconfig.AdminUser{
		Username:     "admin",
		Email:        "admin@example.com",
		PasswordSalt: "seed-salt",
		PasswordHash: "seed-hash",
	}}
}

func updateAdminReq(admin *metarrv1.AdminUser, newPassword string, paths ...string) *connect.Request[metarrv1.UpdateAdminUserRequest] {
	req := &metarrv1.UpdateAdminUserRequest{Admin: admin, NewPassword: newPassword}
	if len(paths) > 0 {
		req.UpdateMask = &fieldmaskpb.FieldMask{Paths: paths}
	}
	return connect.NewRequest(req)
}

func TestGetAdminUser_NeverCarriesTheCredential(t *testing.T) {
	withLiveConfig(t, seededAdmin())
	server, _ := newTestAdminServer(nil)

	resp, err := server.GetAdminUser(context.Background(), connect.NewRequest(&metarrv1.GetAdminUserRequest{}))
	if err != nil {
		t.Fatalf("GetAdminUser: %v", err)
	}
	if resp.Msg.GetPasswordSalt() != "" || resp.Msg.GetPasswordHash() != "" {
		t.Fatalf("credential leaked on read: %+v", resp.Msg)
	}
	if resp.Msg.GetUsername() != "admin" || resp.Msg.GetEmail() != "admin@example.com" {
		t.Fatalf("identity not returned: %+v", resp.Msg)
	}
}

func TestUpdateAdminUser_MaskOnlyEditLeavesThePasswordUntouched(t *testing.T) {
	server, backend := newTestAdminServer(seededAdmin())

	resp, err := server.UpdateAdminUser(correlation.WithID(context.Background(), "c1"),
		updateAdminReq(&metarrv1.AdminUser{Username: "renamed"}, "", "username"))
	if err != nil {
		t.Fatalf("UpdateAdminUser: %v", err)
	}
	if resp.Msg.GetUsername() != "renamed" {
		t.Fatalf("response username = %q, want renamed", resp.Msg.GetUsername())
	}
	if resp.Msg.GetPasswordSalt() != "" || resp.Msg.GetPasswordHash() != "" {
		t.Fatalf("response carried the credential: %+v", resp.Msg)
	}
	got := backend.cfg.GetAdmin()
	if got.GetUsername() != "renamed" || got.GetEmail() != "admin@example.com" {
		t.Fatalf("stored identity = %+v, want username renamed / email unchanged", got)
	}
	if got.GetPasswordSalt() != "seed-salt" || got.GetPasswordHash() != "seed-hash" {
		t.Fatalf("stored credential disturbed by a mask-only edit: %+v", got)
	}
}

func TestUpdateAdminUser_NonEmptyNewPasswordReplacesTheCredential(t *testing.T) {
	server, backend := newTestAdminServer(seededAdmin())

	_, err := server.UpdateAdminUser(correlation.WithID(context.Background(), "c1"),
		updateAdminReq(nil, "a-brand-new-password"))
	if err != nil {
		t.Fatalf("UpdateAdminUser: %v", err)
	}
	got := backend.cfg.GetAdmin()
	if got.GetPasswordHash() == "" || got.GetPasswordHash() == "seed-hash" {
		t.Fatalf("password hash not replaced: %q", got.GetPasswordHash())
	}
	if got.GetPasswordSalt() == "" || got.GetPasswordSalt() == "seed-salt" {
		t.Fatalf("password salt not replaced: %q", got.GetPasswordSalt())
	}
	if got.GetUsername() != "admin" {
		t.Fatalf("identity disturbed by a password-only change: %+v", got)
	}
}

func TestUpdateAdminUser_EmptyNewPasswordIsANoOp(t *testing.T) {
	server, backend := newTestAdminServer(seededAdmin())

	_, err := server.UpdateAdminUser(correlation.WithID(context.Background(), "c1"),
		updateAdminReq(&metarrv1.AdminUser{Email: "new@example.com"}, "", "email"))
	if err != nil {
		t.Fatalf("UpdateAdminUser: %v", err)
	}
	got := backend.cfg.GetAdmin()
	if got.GetPasswordHash() != "seed-hash" || got.GetPasswordSalt() != "seed-salt" {
		t.Fatalf("an empty new_password disturbed the credential: %+v", got)
	}
	if got.GetEmail() != "new@example.com" {
		t.Fatalf("email not applied: %+v", got)
	}
}

func TestUpdateAdminUser_EmptyMaskAndNoPasswordIsInvalidArgument(t *testing.T) {
	server, _ := newTestAdminServer(seededAdmin())

	_, err := server.UpdateAdminUser(context.Background(), updateAdminReq(&metarrv1.AdminUser{Username: "x"}, ""))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestUpdateAdminUser_UnknownMaskPathIsInvalidArgument(t *testing.T) {
	server, _ := newTestAdminServer(seededAdmin())

	_, err := server.UpdateAdminUser(context.Background(),
		updateAdminReq(&metarrv1.AdminUser{PasswordHash: "sneaky"}, "", "password_hash"))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestUpdateAdminUser_ExplicitEmptyIdentityFieldIsRejected(t *testing.T) {
	server, backend := newTestAdminServer(seededAdmin())

	_, err := server.UpdateAdminUser(context.Background(),
		updateAdminReq(&metarrv1.AdminUser{Username: ""}, "", "username"))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
	if backend.cfg.GetAdmin().GetUsername() != "admin" {
		t.Fatalf("stored username changed on a rejected request: %+v", backend.cfg.GetAdmin())
	}
}
