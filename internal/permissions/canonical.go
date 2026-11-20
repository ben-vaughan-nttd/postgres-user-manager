package permissions

import (
	"errors"
	"fmt"
)

// CanonicalGrants validates declarative permission statements for principal
// and returns their canonical SQL, in input order. Only GRANT is accepted:
// revokes come from removing a grant from the configuration, not from a REVOKE
// statement. Every statement is checked and all failures are returned together.
//
// This is the single validation path shared by configuration preflight and by
// execution, so what is checked is exactly what runs.
func CanonicalGrants(principal string, stmts []string) ([]string, error) {
	canonical := make([]string, 0, len(stmts))
	var errs []error
	for i, stmt := range stmts {
		rule, err := Validate(stmt, principal, ActionGrant)
		if err == nil {
			var sql string
			if sql, err = rule.SQL(); err == nil {
				canonical = append(canonical, sql)
				continue
			}
		}
		if errors.Is(err, ErrActionNotAllowed) {
			err = fmt.Errorf("permissions are GRANT-only desired state, to revoke access remove the grant from the configuration: %w", err)
		}
		errs = append(errs, fmt.Errorf("permissions[%d] for %s: %w", i, principal, err))
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return canonical, nil
}

// CanonicalDatabaseStatements validates the legacy privileges x databases
// fields for principal and returns the canonical SQL for every combination,
// databases first, then privileges. All failures are returned together.
func CanonicalDatabaseStatements(action Action, principal string, privileges, databases []string) ([]string, error) {
	var (
		statements []string
		errs       []error
	)
	for _, db := range databases {
		for _, priv := range privileges {
			rule, err := DatabaseRule(action, priv, db, principal)
			if err == nil {
				var sql string
				if sql, err = rule.SQL(); err == nil {
					statements = append(statements, sql)
					continue
				}
			}
			errs = append(errs, fmt.Errorf("privilege %q on database %q for %s: %w", priv, db, principal, err))
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return statements, nil
}
