// Package permissions validates GRANT and REVOKE statements with the PostgreSQL
// parser (pg_query_go), checks them against a documented support matrix, and
// returns a normalized Rule. Rules compare equal when the statements they came
// from are equivalent, and Rule.SQL renders canonical SQL from the parsed AST
// so callers never execute raw input.
package permissions

import (
	"reflect"
	"sort"
)

// Action is the statement direction of a Rule.
type Action int

const (
	// ActionGrant is a GRANT statement.
	ActionGrant Action = iota + 1
	// ActionRevoke is a REVOKE statement.
	ActionRevoke
)

// String returns "GRANT" or "REVOKE".
func (a Action) String() string {
	switch a {
	case ActionGrant:
		return "GRANT"
	case ActionRevoke:
		return "REVOKE"
	default:
		return "UNKNOWN"
	}
}

// ObjectType is the kind of object a Rule targets.
type ObjectType int

const (
	// ObjectDatabase targets databases.
	ObjectDatabase ObjectType = iota + 1
	// ObjectSchema targets schemas.
	ObjectSchema
	// ObjectTable targets tables (or all tables in a schema).
	ObjectTable
	// ObjectSequence targets sequences (or all sequences in a schema).
	ObjectSequence
)

// String returns the SQL keyword for the object type.
func (o ObjectType) String() string {
	switch o {
	case ObjectDatabase:
		return "DATABASE"
	case ObjectSchema:
		return "SCHEMA"
	case ObjectTable:
		return "TABLE"
	case ObjectSequence:
		return "SEQUENCE"
	default:
		return "UNKNOWN"
	}
}

// Privilege is a normalized, upper-case privilege name. TEMP is always
// normalized to PrivTemporary.
type Privilege string

// Supported privileges.
const (
	PrivConnect    Privilege = "CONNECT"
	PrivCreate     Privilege = "CREATE"
	PrivTemporary  Privilege = "TEMPORARY"
	PrivUsage      Privilege = "USAGE"
	PrivSelect     Privilege = "SELECT"
	PrivInsert     Privilege = "INSERT"
	PrivUpdate     Privilege = "UPDATE"
	PrivDelete     Privilege = "DELETE"
	PrivTruncate   Privilege = "TRUNCATE"
	PrivReferences Privilege = "REFERENCES"
	PrivTrigger    Privilege = "TRIGGER"
	PrivMaintain   Privilege = "MAINTAIN"
)

// ObjectName is a normalized object name. Identifiers follow PostgreSQL rules:
// unquoted names are lower-cased and quoted names keep their exact case.
// Schema is empty for databases, schemas, and unqualified relations.
type ObjectName struct {
	Schema string
	Name   string
}

// Rule is the normalized form of one supported GRANT or REVOKE statement.
type Rule struct {
	Action     Action
	ObjectType ObjectType
	// Objects are sorted and de-duplicated. Empty when AllInSchema is set.
	Objects []ObjectName
	// AllInSchema is non-empty for ALL TABLES/SEQUENCES IN SCHEMA.
	AllInSchema string
	// Privileges are normalized, de-duplicated and sorted, with ALL expanded
	// to the concrete set for the object type.
	Privileges []Privilege
	// Columns is set for table column grants only; it is sorted and applies
	// to every privilege in the rule.
	Columns []string
	// Grantee is the normalized role name.
	Grantee string
}

// allPrivileges is the expansion of ALL [PRIVILEGES] per object type
// (PostgreSQL 17).
var allPrivileges = map[ObjectType][]Privilege{
	ObjectDatabase: {PrivConnect, PrivCreate, PrivTemporary},
	ObjectSchema:   {PrivCreate, PrivUsage},
	ObjectTable: {
		PrivDelete, PrivInsert, PrivMaintain, PrivReferences,
		PrivSelect, PrivTrigger, PrivTruncate, PrivUpdate,
	},
	ObjectSequence: {PrivSelect, PrivUpdate, PrivUsage},
}

// allColumnPrivileges is the expansion of ALL (columns) on a table.
var allColumnPrivileges = []Privilege{PrivInsert, PrivReferences, PrivSelect, PrivUpdate}

// privilegeNames maps accepted privilege spellings (upper-case) to the
// normalized privilege.
var privilegeNames = map[string]Privilege{
	"CONNECT":    PrivConnect,
	"CREATE":     PrivCreate,
	"TEMPORARY":  PrivTemporary,
	"TEMP":       PrivTemporary,
	"USAGE":      PrivUsage,
	"SELECT":     PrivSelect,
	"INSERT":     PrivInsert,
	"UPDATE":     PrivUpdate,
	"DELETE":     PrivDelete,
	"TRUNCATE":   PrivTruncate,
	"REFERENCES": PrivReferences,
	"TRIGGER":    PrivTrigger,
	"MAINTAIN":   PrivMaintain,
}

func privilegeAllowed(obj ObjectType, p Privilege) bool {
	for _, allowed := range allPrivileges[obj] {
		if allowed == p {
			return true
		}
	}
	return false
}

func columnPrivilegeAllowed(p Privilege) bool {
	for _, allowed := range allColumnPrivileges {
		if allowed == p {
			return true
		}
	}
	return false
}

// Inverse returns the rule with GRANT and REVOKE swapped and the same target.
// The result shares no memory with the receiver.
func (r Rule) Inverse() Rule {
	inv := r.clone()
	switch r.Action {
	case ActionGrant:
		inv.Action = ActionRevoke
	case ActionRevoke:
		inv.Action = ActionGrant
	}
	return inv
}

// Equal reports whether two rules describe the same statement.
func (r Rule) Equal(other Rule) bool {
	return reflect.DeepEqual(r.normalized(), other.normalized())
}

func (r Rule) clone() Rule {
	c := r
	c.Objects = append([]ObjectName(nil), r.Objects...)
	c.Privileges = append([]Privilege(nil), r.Privileges...)
	c.Columns = append([]string(nil), r.Columns...)
	return c
}

// normalized returns a copy with empty slices as nil so DeepEqual treats nil
// and empty alike.
func (r Rule) normalized() Rule {
	c := r.clone()
	if len(c.Objects) == 0 {
		c.Objects = nil
	}
	if len(c.Privileges) == 0 {
		c.Privileges = nil
	}
	if len(c.Columns) == 0 {
		c.Columns = nil
	}
	return c
}

func sortedUniquePrivileges(in []Privilege) []Privilege {
	seen := make(map[Privilege]bool, len(in))
	out := make([]Privilege, 0, len(in))
	for _, p := range in {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sortedUniqueStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func sortedUniqueObjects(in []ObjectName) []ObjectName {
	seen := make(map[ObjectName]bool, len(in))
	out := make([]ObjectName, 0, len(in))
	for _, o := range in {
		if !seen[o] {
			seen[o] = true
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Schema != out[j].Schema {
			return out[i].Schema < out[j].Schema
		}
		return out[i].Name < out[j].Name
	})
	return out
}
