package permissions

import (
	"errors"
	"fmt"
)

// GrammarVersion names the PostgreSQL grammar used to parse permission
// statements. It is fixed by the pg_query_go release in go.mod (v6.x embeds the
// PostgreSQL 17 parser). Statements are validated against this grammar
// regardless of the version of the server that eventually executes them.
const GrammarVersion = "PostgreSQL 17"

// matrixHint is appended to every validation error so users can find the
// documented support matrix.
const matrixHint = `see the "Permission support matrix" section of the README`

// Typed errors returned (wrapped) by Parse, Validate and Rule.SQL. Use
// errors.Is to test for them.
var (
	// ErrSyntax means the statement is not valid in the PostgreSQL 17 grammar
	// or is empty.
	ErrSyntax = errors.New("invalid permission statement syntax")
	// ErrInvalidInput means the input contains a NUL byte, which the C parser
	// would treat as the end of the string and silently ignore what follows.
	ErrInvalidInput = errors.New("invalid input")
	// ErrMultipleStatements means the input contained more than one statement.
	ErrMultipleStatements = errors.New("multiple statements are not allowed")
	// ErrUnsupportedStatement means the statement is not a GRANT or REVOKE on
	// privileges (for example role membership, DO, COPY or CALL).
	ErrUnsupportedStatement = errors.New("unsupported statement type")
	// ErrUnsupportedObject means the grant targets an object type, or an
	// object form, outside the support matrix.
	ErrUnsupportedObject = errors.New("unsupported object")
	// ErrUnsupportedPrivilege means a privilege is not allowed for the object
	// type.
	ErrUnsupportedPrivilege = errors.New("unsupported privilege")
	// ErrUnsupportedOption means the statement uses an option or grantee form
	// outside the support matrix (PUBLIC, GRANTED BY, grant options, CASCADE,
	// multiple grantees, mixed column lists, ...).
	ErrUnsupportedOption = errors.New("unsupported option")
	// ErrGranteeMismatch means the grantee is not the principal being
	// reconciled.
	ErrGranteeMismatch = errors.New("grantee does not match principal")
	// ErrActionNotAllowed means the statement's action (GRANT or REVOKE) is
	// not permitted in this context.
	ErrActionNotAllowed = errors.New("action not allowed")
)

// wrap builds an error that wraps kind, names the offending element and points
// to the README support matrix.
func wrap(kind error, format string, args ...any) error {
	return fmt.Errorf("%w: %s (%s)", kind, fmt.Sprintf(format, args...), matrixHint)
}
