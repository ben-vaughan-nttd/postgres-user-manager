package config

import (
	"errors"
	"fmt"

	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/permissions"
	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/structs"
)

// ValidateConfig runs the permission preflight for every user and group. It is
// called from LoadConfig, so the validate command and every execution flow that
// loads a configuration share it, and a bad configuration fails before any
// database change. All problems are reported together.
func ValidateConfig(cfg *structs.Config) error {
	var errs []error
	for i := range cfg.Groups {
		if err := ValidateGroup(&cfg.Groups[i]); err != nil {
			errs = append(errs, fmt.Errorf("groups[%d]: %w", i, err))
		}
	}
	for i := range cfg.Users {
		if err := ValidateUser(&cfg.Users[i]); err != nil {
			errs = append(errs, fmt.Errorf("users[%d]: %w", i, err))
		}
	}
	return errors.Join(errs...)
}

// ValidateUser validates the permissions and legacy privileges/databases of a
// user against the permission support matrix, with the user as grantee.
func ValidateUser(user *structs.UserConfig) error {
	return validatePrincipal(user.Username, user.Privileges, user.Databases, user.Permissions)
}

// ValidateGroup validates the permissions and legacy privileges/databases of a
// group against the permission support matrix, with the group as grantee.
func ValidateGroup(group *structs.GroupConfig) error {
	return validatePrincipal(group.Name, group.Privileges, group.Databases, group.Permissions)
}

func validatePrincipal(principal string, privileges, databases, stmts []string) error {
	var errs []error
	if _, err := permissions.CanonicalDatabaseStatements(permissions.ActionGrant, principal, privileges, databases); err != nil {
		errs = append(errs, err)
	}
	if _, err := permissions.CanonicalGrants(principal, stmts); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
