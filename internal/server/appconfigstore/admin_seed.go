package appconfigstore

import (
	"context"

	"Metarr/internal/server/passwordhash"
	"Metarr/internal/shared/appconfig"
)

const defaultAdminPasswordChars = 12

// AdminSeedResult reports what SeedAdmin did. Password is the plaintext
// password if one was (re)generated, empty if nothing changed. Recovered
// distinguishes the two cases that produce a password — main.go prints a
// different one-time message for each.
type AdminSeedResult struct {
	Username  string
	Password  string
	Recovered bool
}

// SeedAdmin seeds the admin account the first time the app starts against
// this database, or recovers one left locked out by the whole-document
// config update bug ADR 0001 removed — a scoped edit round-tripped the
// redacted Get response, which carries no password fields, and the
// resulting write wrote them back empty. The two cases are mutually
// exclusive and must run in this order: recovery only applies to an account
// seeding didn't just create, since seeding always leaves both password
// fields set. Combining them in one Bootstrap call makes that ordering
// impossible to break by reordering call sites, unlike when both lived as
// separate blocks in main.go. See docs/adr/0003.
func (s *Store) SeedAdmin(ctx context.Context) (AdminSeedResult, error) {
	var result AdminSeedResult
	err := s.Bootstrap(ctx, func(cfg *appconfig.Config) (bool, error) {
		// The admin section is a pointer, so a document that has never had
		// one arrives nil rather than zeroed. Seeding is exactly the case
		// that handles it, so it is filled in here rather than guarded at
		// every field read below.
		if cfg.Admin == nil {
			cfg.Admin = &appconfig.AdminUser{}
		}
		if cfg.Admin.Username == "" {
			password, salt, hash, err := generateAdminCredentials()
			if err != nil {
				return false, err
			}
			username, email := appconfig.DefaultAdminIdentity()
			cfg.Admin = &appconfig.AdminUser{
				Username:     username,
				Email:        email,
				PasswordSalt: salt,
				PasswordHash: hash,
			}
			result = AdminSeedResult{Username: cfg.Admin.Username, Password: password}
			return true, nil
		}

		password, recovered, err := recoverLockedOutAdmin(cfg.Admin)
		if err != nil {
			return false, err
		}
		if !recovered {
			return false, nil
		}
		result = AdminSeedResult{Username: cfg.Admin.Username, Password: password, Recovered: true}
		return true, nil
	})
	return result, err
}

// recoverLockedOutAdmin detects an admin account with a username but no
// usable password — a casualty of the whole-document config update bug (ADR
// 0001), not a fresh install — and issues it a new one in place. A record
// with no username, or with both password fields intact, is left untouched
// and recovered is false.
func recoverLockedOutAdmin(admin *appconfig.AdminUser) (plaintextPassword string, recovered bool, err error) {
	if admin.Username == "" {
		return "", false, nil
	}
	if admin.PasswordSalt != "" && admin.PasswordHash != "" {
		return "", false, nil
	}

	plaintextPassword, salt, hash, err := generateAdminCredentials()
	if err != nil {
		return "", false, err
	}
	admin.PasswordSalt = salt
	admin.PasswordHash = hash
	return plaintextPassword, true, nil
}

func generateAdminCredentials() (password, salt, hash string, err error) {
	password, err = passwordhash.GenerateRandomPassword(defaultAdminPasswordChars)
	if err != nil {
		return "", "", "", err
	}
	salt, hash, err = passwordhash.Hash(password)
	if err != nil {
		return "", "", "", err
	}
	return password, salt, hash, nil
}
