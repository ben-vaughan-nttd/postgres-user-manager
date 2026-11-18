package permissions

import (
	"strings"
	"sync"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// SQL returns the canonical SQL for the rule. The statement is assembled as a
// parse tree and rendered by pg_query.Deparse, so identifiers are quoted by
// PostgreSQL's own deparser and no input text is ever concatenated into SQL.
// SQL fails if the rule is outside the support matrix (for example a zero
// Rule or one built by hand with an unsupported privilege).
func (r Rule) SQL() (string, error) {
	if err := r.check(); err != nil {
		return "", err
	}

	stmt := &pg_query.GrantStmt{
		IsGrant:  r.Action == ActionGrant,
		Targtype: pg_query.GrantTargetType_ACL_TARGET_OBJECT,
		Objtype:  protoObjectType(r.ObjectType),
		Grantees: []*pg_query.Node{{
			Node: &pg_query.Node_RoleSpec{RoleSpec: &pg_query.RoleSpec{
				Roletype: pg_query.RoleSpecType_ROLESPEC_CSTRING,
				Rolename: r.Grantee,
			}},
		}},
		Behavior: pg_query.DropBehavior_DROP_RESTRICT,
	}

	if r.AllInSchema != "" {
		stmt.Targtype = pg_query.GrantTargetType_ACL_TARGET_ALL_IN_SCHEMA
		stmt.Objects = []*pg_query.Node{pg_query.MakeStrNode(r.AllInSchema)}
	} else {
		for _, obj := range r.Objects {
			stmt.Objects = append(stmt.Objects, objectNode(r.ObjectType, obj))
		}
	}

	var cols []*pg_query.Node
	for _, col := range r.Columns {
		cols = append(cols, pg_query.MakeStrNode(col))
	}
	switch {
	case r.grantsAll() && len(cols) == 0:
		// Empty privilege list renders as ALL.
	case r.grantsAll():
		// ALL (columns): empty privilege name with a column list.
		stmt.Privileges = []*pg_query.Node{{
			Node: &pg_query.Node_AccessPriv{AccessPriv: &pg_query.AccessPriv{Cols: cols}},
		}}
	default:
		for _, priv := range r.Privileges {
			stmt.Privileges = append(stmt.Privileges, &pg_query.Node{
				Node: &pg_query.Node_AccessPriv{AccessPriv: &pg_query.AccessPriv{
					PrivName: strings.ToLower(string(priv)),
					Cols:     cols,
				}},
			})
		}
	}

	version, err := parseTreeVersion()
	if err != nil {
		return "", wrap(ErrUnsupportedStatement, "cannot render canonical SQL: %v", err)
	}

	out, err := pg_query.Deparse(&pg_query.ParseResult{
		Version: version,
		Stmts: []*pg_query.RawStmt{{
			Stmt: &pg_query.Node{Node: &pg_query.Node_GrantStmt{GrantStmt: stmt}},
		}},
	})
	if err != nil {
		return "", wrap(ErrUnsupportedStatement, "cannot render canonical SQL: %v", err)
	}
	return out, nil
}

var (
	treeVersionOnce sync.Once
	treeVersion     int32
	treeVersionErr  error
)

// parseTreeVersion returns the parse tree version of the embedded parser,
// which Deparse requires on hand-built trees.
func parseTreeVersion() (int32, error) {
	treeVersionOnce.Do(func() {
		tree, err := pg_query.Parse("SELECT 1")
		if err != nil {
			treeVersionErr = err
			return
		}
		treeVersion = tree.GetVersion()
	})
	return treeVersion, treeVersionErr
}

// grantsAll reports whether the rule's privileges are the full set for its
// object type (or, with columns, the full column set). SQL renders that as ALL
// rather than an explicit list, so the statement means "everything this server
// version defines" and stays valid on servers older than PostgreSQL 17 (where
// MAINTAIN does not exist). Rules that name every privilege explicitly and
// rules written with ALL are equal and render identically.
func (r Rule) grantsAll() bool {
	full := allPrivileges[r.ObjectType]
	if len(r.Columns) > 0 {
		full = allColumnPrivileges
	}
	if len(r.Privileges) != len(full) {
		return false
	}
	for _, p := range full {
		found := false
		for _, have := range r.Privileges {
			if have == p {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// hasNUL reports whether any identifier in the rule contains a NUL byte.
func (r Rule) hasNUL() bool {
	names := []string{r.Grantee, r.AllInSchema}
	names = append(names, r.Columns...)
	for _, o := range r.Objects {
		names = append(names, o.Schema, o.Name)
	}
	for _, n := range names {
		if strings.ContainsRune(n, 0) {
			return true
		}
	}
	return false
}

func protoObjectType(t ObjectType) pg_query.ObjectType {
	switch t {
	case ObjectDatabase:
		return pg_query.ObjectType_OBJECT_DATABASE
	case ObjectSchema:
		return pg_query.ObjectType_OBJECT_SCHEMA
	case ObjectTable:
		return pg_query.ObjectType_OBJECT_TABLE
	case ObjectSequence:
		return pg_query.ObjectType_OBJECT_SEQUENCE
	default:
		return pg_query.ObjectType_OBJECT_TYPE_UNDEFINED
	}
}

func objectNode(t ObjectType, obj ObjectName) *pg_query.Node {
	switch t {
	case ObjectTable, ObjectSequence:
		return &pg_query.Node{Node: &pg_query.Node_RangeVar{RangeVar: &pg_query.RangeVar{
			Schemaname:     obj.Schema,
			Relname:        obj.Name,
			Inh:            true,
			Relpersistence: "p",
		}}}
	default:
		return pg_query.MakeStrNode(obj.Name)
	}
}

// check verifies that a Rule satisfies the support matrix, so SQL never
// renders a statement Parse would reject.
func (r Rule) check() error {
	if r.hasNUL() {
		return wrap(ErrInvalidInput, "rule contains a NUL byte in an identifier")
	}
	if r.Action != ActionGrant && r.Action != ActionRevoke {
		return wrap(ErrUnsupportedStatement, "rule has no action")
	}
	if r.Grantee == "" {
		return wrap(ErrUnsupportedOption, "rule has no grantee")
	}
	if _, ok := allPrivileges[r.ObjectType]; !ok {
		return wrap(ErrUnsupportedObject, "rule has object type %s", r.ObjectType)
	}

	switch {
	case r.AllInSchema != "":
		if r.ObjectType != ObjectTable && r.ObjectType != ObjectSequence {
			return wrap(ErrUnsupportedObject, "ALL %ss IN SCHEMA", r.ObjectType)
		}
		if len(r.Objects) > 0 {
			return wrap(ErrUnsupportedObject, "rule names both objects and ALL IN SCHEMA")
		}
		if len(r.Columns) > 0 {
			return wrap(ErrUnsupportedOption, "column lists cannot be combined with ALL TABLES IN SCHEMA")
		}
	case len(r.Objects) == 0:
		return wrap(ErrUnsupportedObject, "rule names no %s", r.ObjectType)
	default:
		for _, obj := range r.Objects {
			if obj.Name == "" {
				return wrap(ErrUnsupportedObject, "rule has an empty %s name", r.ObjectType)
			}
			if obj.Schema != "" && r.ObjectType != ObjectTable && r.ObjectType != ObjectSequence {
				return wrap(ErrUnsupportedObject, "%s names cannot be schema-qualified", r.ObjectType)
			}
		}
	}

	if len(r.Privileges) == 0 {
		return wrap(ErrUnsupportedPrivilege, "rule has no privileges")
	}
	if len(r.Columns) > 0 && r.ObjectType != ObjectTable {
		return wrap(ErrUnsupportedOption, "column lists are only valid on TABLE, not %s", r.ObjectType)
	}
	for _, priv := range r.Privileges {
		if len(r.Columns) > 0 {
			if !columnPrivilegeAllowed(priv) {
				return wrap(ErrUnsupportedPrivilege, "%s as a column privilege (allowed: SELECT, INSERT, UPDATE, REFERENCES)", priv)
			}
			continue
		}
		if !privilegeAllowed(r.ObjectType, priv) {
			return wrap(ErrUnsupportedPrivilege, "%s on %s", priv, r.ObjectType)
		}
	}
	for _, col := range r.Columns {
		if col == "" {
			return wrap(ErrUnsupportedOption, "rule has an empty column name")
		}
	}
	return nil
}

// DatabaseRule builds the Rule for one legacy privileges/databases entry
// (GRANT <privilege> ON DATABASE <database> TO <principal>, or the REVOKE
// form). The text is built with quoted identifiers and then sent through
// Validate, so the privilege string cannot add statements, change the object,
// or change the grantee.
func DatabaseRule(action Action, privilege, database, principal string) (Rule, error) {
	var text string
	switch action {
	case ActionGrant:
		text = "GRANT " + privilege + " ON DATABASE " + QuoteIdentifier(database) + " TO " + QuoteIdentifier(principal)
	case ActionRevoke:
		text = "REVOKE " + privilege + " ON DATABASE " + QuoteIdentifier(database) + " FROM " + QuoteIdentifier(principal)
	default:
		return Rule{}, wrap(ErrActionNotAllowed, "unknown action")
	}

	rule, err := Validate(text, principal, action)
	if err != nil {
		return Rule{}, err
	}
	if rule.ObjectType != ObjectDatabase || len(rule.Objects) != 1 || rule.Objects[0].Name != database {
		return Rule{}, wrap(ErrUnsupportedObject, "privilege %q must apply only to database %q", privilege, database)
	}
	return rule, nil
}

// QuoteIdentifier double-quotes name for use in SQL text.
func QuoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
