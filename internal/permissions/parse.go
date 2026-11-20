package permissions

import (
	"fmt"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// Parse parses sql with the PostgreSQL 17 grammar and checks it against the
// support matrix. It accepts exactly one GRANT or REVOKE statement on
// privileges and returns its normalized Rule. Parse does not know which role
// the statement is meant for; use Validate to check the grantee and action.
func Parse(sql string) (Rule, error) {
	if strings.ContainsRune(sql, 0) {
		return Rule{}, wrap(ErrInvalidInput, "statement contains a NUL byte at offset %d", strings.IndexByte(sql, 0))
	}
	if strings.TrimSpace(sql) == "" {
		return Rule{}, wrap(ErrSyntax, "empty statement")
	}

	tree, err := pg_query.Parse(sql)
	if err != nil {
		return Rule{}, wrap(ErrSyntax, "%v; statements are parsed with the %s grammar", err, GrammarVersion)
	}

	switch len(tree.Stmts) {
	case 0:
		return Rule{}, wrap(ErrSyntax, "empty statement")
	case 1:
	default:
		return Rule{}, wrap(ErrMultipleStatements, "found %d statements, expected exactly one", len(tree.Stmts))
	}

	node := tree.Stmts[0].GetStmt()
	grant := node.GetGrantStmt()
	if grant == nil {
		return Rule{}, wrap(ErrUnsupportedStatement, "%s is not a GRANT or REVOKE on privileges (role membership uses the groups field)", nodeName(node))
	}

	return ruleFromGrantStmt(grant)
}

// Validate parses sql like Parse, then requires the statement's action to be
// in allow and its grantee to be exactly principal. When allow is empty both
// GRANT and REVOKE are accepted.
//
// principal is the role name as stored in the database. The grantee in the
// statement is resolved with PostgreSQL identifier rules, so an unquoted
// grantee matches the lower-case role name and a quoted grantee matches the
// exact-case role name.
func Validate(sql, principal string, allow ...Action) (Rule, error) {
	rule, err := Parse(sql)
	if err != nil {
		return Rule{}, err
	}

	if len(allow) == 0 {
		allow = []Action{ActionGrant, ActionRevoke}
	}
	if !containsAction(allow, rule.Action) {
		return Rule{}, wrap(ErrActionNotAllowed, "%s is not allowed here (allowed: %s)", rule.Action, joinActions(allow))
	}

	if rule.Grantee != principal {
		return Rule{}, wrap(ErrGranteeMismatch, "statement grants to %q but the principal is %q", rule.Grantee, principal)
	}

	return rule, nil
}

func containsAction(actions []Action, a Action) bool {
	for _, candidate := range actions {
		if candidate == a {
			return true
		}
	}
	return false
}

func joinActions(actions []Action) string {
	names := make([]string, 0, len(actions))
	for _, a := range actions {
		names = append(names, a.String())
	}
	return strings.Join(names, ", ")
}

// nodeName returns a readable name for the statement node type.
func nodeName(node *pg_query.Node) string {
	if node == nil || node.GetNode() == nil {
		return "statement"
	}
	name := fmt.Sprintf("%T", node.GetNode())
	if idx := strings.LastIndex(name, "Node_"); idx >= 0 {
		name = name[idx+len("Node_"):]
	}
	return name
}

func ruleFromGrantStmt(stmt *pg_query.GrantStmt) (Rule, error) {
	rule := Rule{Action: ActionGrant}
	if !stmt.GetIsGrant() {
		rule.Action = ActionRevoke
	}

	if stmt.GetGrantOption() {
		if rule.Action == ActionGrant {
			return Rule{}, wrap(ErrUnsupportedOption, "WITH GRANT OPTION")
		}
		return Rule{}, wrap(ErrUnsupportedOption, "GRANT OPTION FOR")
	}
	if stmt.GetGrantor() != nil {
		return Rule{}, wrap(ErrUnsupportedOption, "GRANTED BY")
	}
	if stmt.GetBehavior() == pg_query.DropBehavior_DROP_CASCADE {
		return Rule{}, wrap(ErrUnsupportedOption, "CASCADE")
	}

	grantee, err := granteeFromNodes(stmt.GetGrantees())
	if err != nil {
		return Rule{}, err
	}
	rule.Grantee = grantee

	if err := applyTarget(&rule, stmt); err != nil {
		return Rule{}, err
	}
	if err := applyPrivileges(&rule, stmt); err != nil {
		return Rule{}, err
	}
	return rule, nil
}

func granteeFromNodes(nodes []*pg_query.Node) (string, error) {
	if len(nodes) != 1 {
		return "", wrap(ErrUnsupportedOption, "exactly one grantee is required, found %d", len(nodes))
	}
	spec := nodes[0].GetRoleSpec()
	if spec == nil {
		return "", wrap(ErrUnsupportedOption, "unrecognized grantee %s", nodeName(nodes[0]))
	}
	switch spec.GetRoletype() {
	case pg_query.RoleSpecType_ROLESPEC_CSTRING:
		return spec.GetRolename(), nil
	case pg_query.RoleSpecType_ROLESPEC_PUBLIC:
		return "", wrap(ErrUnsupportedOption, "grantee PUBLIC")
	case pg_query.RoleSpecType_ROLESPEC_CURRENT_USER:
		return "", wrap(ErrUnsupportedOption, "grantee CURRENT_USER")
	case pg_query.RoleSpecType_ROLESPEC_CURRENT_ROLE:
		return "", wrap(ErrUnsupportedOption, "grantee CURRENT_ROLE")
	case pg_query.RoleSpecType_ROLESPEC_SESSION_USER:
		return "", wrap(ErrUnsupportedOption, "grantee SESSION_USER")
	default:
		return "", wrap(ErrUnsupportedOption, "unrecognized grantee type %s", spec.GetRoletype())
	}
}

func objectTypeFromProto(t pg_query.ObjectType) (ObjectType, bool) {
	switch t {
	case pg_query.ObjectType_OBJECT_DATABASE:
		return ObjectDatabase, true
	case pg_query.ObjectType_OBJECT_SCHEMA:
		return ObjectSchema, true
	case pg_query.ObjectType_OBJECT_TABLE:
		return ObjectTable, true
	case pg_query.ObjectType_OBJECT_SEQUENCE:
		return ObjectSequence, true
	default:
		return 0, false
	}
}

func applyTarget(rule *Rule, stmt *pg_query.GrantStmt) error {
	objType, ok := objectTypeFromProto(stmt.GetObjtype())
	if !ok {
		return wrap(ErrUnsupportedObject, "object type %s (supported: DATABASE, SCHEMA, TABLE, SEQUENCE)", stmt.GetObjtype())
	}
	rule.ObjectType = objType

	switch stmt.GetTargtype() {
	case pg_query.GrantTargetType_ACL_TARGET_OBJECT:
		return applyObjects(rule, stmt.GetObjects())
	case pg_query.GrantTargetType_ACL_TARGET_ALL_IN_SCHEMA:
		if objType != ObjectTable && objType != ObjectSequence {
			return wrap(ErrUnsupportedObject, "ALL %ss IN SCHEMA (supported for TABLES and SEQUENCES)", objType)
		}
		schemas := stmt.GetObjects()
		if len(schemas) != 1 {
			return wrap(ErrUnsupportedObject, "ALL %ss IN SCHEMA requires exactly one schema, found %d", objType, len(schemas))
		}
		name, ok := stringValue(schemas[0])
		if !ok {
			return wrap(ErrUnsupportedObject, "unrecognized schema name %s", nodeName(schemas[0]))
		}
		rule.AllInSchema = name
		return nil
	default:
		return wrap(ErrUnsupportedStatement, "target type %s", stmt.GetTargtype())
	}
}

func applyObjects(rule *Rule, nodes []*pg_query.Node) error {
	if len(nodes) == 0 {
		return wrap(ErrUnsupportedObject, "no %s named", rule.ObjectType)
	}

	objects := make([]ObjectName, 0, len(nodes))
	for _, node := range nodes {
		switch rule.ObjectType {
		case ObjectDatabase, ObjectSchema:
			name, ok := stringValue(node)
			if !ok {
				return wrap(ErrUnsupportedObject, "unrecognized %s name %s", rule.ObjectType, nodeName(node))
			}
			objects = append(objects, ObjectName{Name: name})
		case ObjectTable, ObjectSequence:
			rv := node.GetRangeVar()
			if rv == nil {
				return wrap(ErrUnsupportedObject, "unrecognized %s name %s", rule.ObjectType, nodeName(node))
			}
			if rv.GetCatalogname() != "" {
				return wrap(ErrUnsupportedObject, "database-qualified name %q.%q.%q (use schema.name)", rv.GetCatalogname(), rv.GetSchemaname(), rv.GetRelname())
			}
			objects = append(objects, ObjectName{Schema: rv.GetSchemaname(), Name: rv.GetRelname()})
		}
	}
	rule.Objects = sortedUniqueObjects(objects)
	return nil
}

func stringValue(node *pg_query.Node) (string, bool) {
	s := node.GetString_()
	if s == nil {
		return "", false
	}
	return s.GetSval(), true
}

// applyPrivileges normalizes the privilege list and column lists.
func applyPrivileges(rule *Rule, stmt *pg_query.GrantStmt) error {
	privNodes := stmt.GetPrivileges()

	// An empty privilege list is ALL PRIVILEGES without a column list.
	if len(privNodes) == 0 {
		rule.Privileges = append([]Privilege(nil), allPrivileges[rule.ObjectType]...)
		return nil
	}

	var (
		privs      []Privilege
		columnSets [][]string
		withCols   int
	)
	for _, node := range privNodes {
		ap := node.GetAccessPriv()
		if ap == nil {
			return wrap(ErrUnsupportedPrivilege, "unrecognized privilege entry %s", nodeName(node))
		}

		cols, err := columnNames(ap.GetCols())
		if err != nil {
			return err
		}
		if len(cols) > 0 {
			withCols++
		}
		columnSets = append(columnSets, cols)

		// ALL with a column list: PrivName is empty.
		if ap.GetPrivName() == "" {
			privs = append(privs, allColumnPrivileges...)
			continue
		}

		priv, ok := privilegeNames[strings.ToUpper(ap.GetPrivName())]
		if !ok {
			return wrap(ErrUnsupportedPrivilege, "%q on %s", ap.GetPrivName(), rule.ObjectType)
		}
		privs = append(privs, priv)
	}

	if withCols > 0 {
		return applyColumnPrivileges(rule, privs, columnSets, withCols)
	}

	for _, priv := range privs {
		if !privilegeAllowed(rule.ObjectType, priv) {
			return wrap(ErrUnsupportedPrivilege, "%s on %s", priv, rule.ObjectType)
		}
	}
	rule.Privileges = sortedUniquePrivileges(privs)
	return nil
}

func columnNames(nodes []*pg_query.Node) ([]string, error) {
	cols := make([]string, 0, len(nodes))
	for _, node := range nodes {
		name, ok := stringValue(node)
		if !ok {
			return nil, wrap(ErrUnsupportedOption, "unrecognized column name %s", nodeName(node))
		}
		cols = append(cols, name)
	}
	return cols, nil
}

func applyColumnPrivileges(rule *Rule, privs []Privilege, columnSets [][]string, withCols int) error {
	if rule.ObjectType != ObjectTable {
		return wrap(ErrUnsupportedOption, "column lists are only valid on TABLE, not %s", rule.ObjectType)
	}
	if rule.AllInSchema != "" {
		return wrap(ErrUnsupportedOption, "column lists cannot be combined with ALL TABLES IN SCHEMA")
	}
	if withCols != len(columnSets) {
		return wrap(ErrUnsupportedOption, "column privileges cannot be mixed with table-level privileges in one statement")
	}

	first := sortedUniqueStrings(columnSets[0])
	for _, set := range columnSets[1:] {
		if !reflectEqualStrings(first, sortedUniqueStrings(set)) {
			return wrap(ErrUnsupportedOption, "every privilege in a statement must use the same column list")
		}
	}

	for _, priv := range privs {
		if !columnPrivilegeAllowed(priv) {
			return wrap(ErrUnsupportedPrivilege, "%s as a column privilege (allowed: SELECT, INSERT, UPDATE, REFERENCES)", priv)
		}
	}

	rule.Privileges = sortedUniquePrivileges(privs)
	rule.Columns = first
	return nil
}

func reflectEqualStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
