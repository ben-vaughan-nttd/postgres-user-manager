package permissions

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

const principal = "app_user"

func mustParse(t *testing.T, sql string) Rule {
	t.Helper()
	rule, err := Parse(sql)
	if err != nil {
		t.Fatalf("Parse(%q): %v", sql, err)
	}
	return rule
}

func TestParsePositive(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want Rule
	}{
		{
			name: "database connect",
			sql:  "GRANT CONNECT ON DATABASE mydb TO app_user",
			want: Rule{Action: ActionGrant, ObjectType: ObjectDatabase, Objects: []ObjectName{{Name: "mydb"}}, Privileges: []Privilege{PrivConnect}, Grantee: "app_user"},
		},
		{
			name: "database temp alias and sort",
			sql:  "GRANT TEMP, CREATE, CONNECT ON DATABASE mydb TO app_user",
			want: Rule{Action: ActionGrant, ObjectType: ObjectDatabase, Objects: []ObjectName{{Name: "mydb"}}, Privileges: []Privilege{PrivConnect, PrivCreate, PrivTemporary}, Grantee: "app_user"},
		},
		{
			name: "database all",
			sql:  "GRANT ALL PRIVILEGES ON DATABASE mydb TO app_user",
			want: Rule{Action: ActionGrant, ObjectType: ObjectDatabase, Objects: []ObjectName{{Name: "mydb"}}, Privileges: []Privilege{PrivConnect, PrivCreate, PrivTemporary}, Grantee: "app_user"},
		},
		{
			name: "schema usage create",
			sql:  "GRANT USAGE, CREATE ON SCHEMA app TO app_user",
			want: Rule{Action: ActionGrant, ObjectType: ObjectSchema, Objects: []ObjectName{{Name: "app"}}, Privileges: []Privilege{PrivCreate, PrivUsage}, Grantee: "app_user"},
		},
		{
			name: "schema all",
			sql:  "GRANT ALL ON SCHEMA app TO app_user",
			want: Rule{Action: ActionGrant, ObjectType: ObjectSchema, Objects: []ObjectName{{Name: "app"}}, Privileges: []Privilege{PrivCreate, PrivUsage}, Grantee: "app_user"},
		},
		{
			name: "table schema qualified",
			sql:  "GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE app.orders TO app_user",
			want: Rule{Action: ActionGrant, ObjectType: ObjectTable, Objects: []ObjectName{{Schema: "app", Name: "orders"}}, Privileges: []Privilege{PrivDelete, PrivInsert, PrivSelect, PrivUpdate}, Grantee: "app_user"},
		},
		{
			name: "table without TABLE keyword",
			sql:  "GRANT SELECT ON orders TO app_user",
			want: Rule{Action: ActionGrant, ObjectType: ObjectTable, Objects: []ObjectName{{Name: "orders"}}, Privileges: []Privilege{PrivSelect}, Grantee: "app_user"},
		},
		{
			name: "table every privilege",
			sql:  "GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER, MAINTAIN ON TABLE t TO app_user",
			want: Rule{Action: ActionGrant, ObjectType: ObjectTable, Objects: []ObjectName{{Name: "t"}}, Privileges: []Privilege{PrivDelete, PrivInsert, PrivMaintain, PrivReferences, PrivSelect, PrivTrigger, PrivTruncate, PrivUpdate}, Grantee: "app_user"},
		},
		{
			name: "table all includes maintain",
			sql:  "GRANT ALL ON TABLE t TO app_user",
			want: Rule{Action: ActionGrant, ObjectType: ObjectTable, Objects: []ObjectName{{Name: "t"}}, Privileges: []Privilege{PrivDelete, PrivInsert, PrivMaintain, PrivReferences, PrivSelect, PrivTrigger, PrivTruncate, PrivUpdate}, Grantee: "app_user"},
		},
		{
			name: "table multiple objects sorted and deduplicated",
			sql:  "GRANT SELECT ON TABLE b, a, b TO app_user",
			want: Rule{Action: ActionGrant, ObjectType: ObjectTable, Objects: []ObjectName{{Name: "a"}, {Name: "b"}}, Privileges: []Privilege{PrivSelect}, Grantee: "app_user"},
		},
		{
			name: "column privileges",
			sql:  "GRANT SELECT (b, a), UPDATE (a, b) ON TABLE t TO app_user",
			want: Rule{Action: ActionGrant, ObjectType: ObjectTable, Objects: []ObjectName{{Name: "t"}}, Privileges: []Privilege{PrivSelect, PrivUpdate}, Columns: []string{"a", "b"}, Grantee: "app_user"},
		},
		{
			name: "column level all",
			sql:  "GRANT ALL (a) ON TABLE t TO app_user",
			want: Rule{Action: ActionGrant, ObjectType: ObjectTable, Objects: []ObjectName{{Name: "t"}}, Privileges: []Privilege{PrivInsert, PrivReferences, PrivSelect, PrivUpdate}, Columns: []string{"a"}, Grantee: "app_user"},
		},
		{
			name: "column level all privileges keyword",
			sql:  "GRANT ALL PRIVILEGES (a, b) ON t TO app_user",
			want: Rule{Action: ActionGrant, ObjectType: ObjectTable, Objects: []ObjectName{{Name: "t"}}, Privileges: []Privilege{PrivInsert, PrivReferences, PrivSelect, PrivUpdate}, Columns: []string{"a", "b"}, Grantee: "app_user"},
		},
		{
			name: "all tables in schema",
			sql:  "GRANT SELECT ON ALL TABLES IN SCHEMA app TO app_user",
			want: Rule{Action: ActionGrant, ObjectType: ObjectTable, AllInSchema: "app", Privileges: []Privilege{PrivSelect}, Grantee: "app_user"},
		},
		{
			name: "sequence",
			sql:  "GRANT USAGE, SELECT ON SEQUENCE app.seq TO app_user",
			want: Rule{Action: ActionGrant, ObjectType: ObjectSequence, Objects: []ObjectName{{Schema: "app", Name: "seq"}}, Privileges: []Privilege{PrivSelect, PrivUsage}, Grantee: "app_user"},
		},
		{
			name: "sequence all",
			sql:  "GRANT ALL ON SEQUENCE seq TO app_user",
			want: Rule{Action: ActionGrant, ObjectType: ObjectSequence, Objects: []ObjectName{{Name: "seq"}}, Privileges: []Privilege{PrivSelect, PrivUpdate, PrivUsage}, Grantee: "app_user"},
		},
		{
			name: "all sequences in schema",
			sql:  "GRANT USAGE ON ALL SEQUENCES IN SCHEMA app TO app_user",
			want: Rule{Action: ActionGrant, ObjectType: ObjectSequence, AllInSchema: "app", Privileges: []Privilege{PrivUsage}, Grantee: "app_user"},
		},
		{
			name: "revoke",
			sql:  "REVOKE SELECT ON TABLE t FROM app_user",
			want: Rule{Action: ActionRevoke, ObjectType: ObjectTable, Objects: []ObjectName{{Name: "t"}}, Privileges: []Privilege{PrivSelect}, Grantee: "app_user"},
		},
		{
			name: "revoke restrict",
			sql:  "REVOKE SELECT ON TABLE t FROM app_user RESTRICT",
			want: Rule{Action: ActionRevoke, ObjectType: ObjectTable, Objects: []ObjectName{{Name: "t"}}, Privileges: []Privilege{PrivSelect}, Grantee: "app_user"},
		},
		{
			name: "unquoted identifiers are lower-cased",
			sql:  "GRANT SELECT ON TABLE App.Orders TO App_User",
			want: Rule{Action: ActionGrant, ObjectType: ObjectTable, Objects: []ObjectName{{Schema: "app", Name: "orders"}}, Privileges: []Privilege{PrivSelect}, Grantee: "app_user"},
		},
		{
			name: "quoted identifiers keep exact case",
			sql:  `GRANT SELECT ON TABLE "App"."Orders" TO "App_User"`,
			want: Rule{Action: ActionGrant, ObjectType: ObjectTable, Objects: []ObjectName{{Schema: "App", Name: "Orders"}}, Privileges: []Privilege{PrivSelect}, Grantee: "App_User"},
		},
		{
			name: "quoted identifier with special characters",
			sql:  `GRANT CONNECT ON DATABASE "my db; DROP" TO "a""b"`,
			want: Rule{Action: ActionGrant, ObjectType: ObjectDatabase, Objects: []ObjectName{{Name: "my db; DROP"}}, Privileges: []Privilege{PrivConnect}, Grantee: `a"b`},
		},
		{
			name: "trailing semicolon and lower-case keywords",
			sql:  "grant select on table t to app_user;",
			want: Rule{Action: ActionGrant, ObjectType: ObjectTable, Objects: []ObjectName{{Name: "t"}}, Privileges: []Privilege{PrivSelect}, Grantee: "app_user"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.sql)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.sql, err)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("Parse(%q)\n got: %+v\nwant: %+v", tt.sql, got, tt.want)
			}
		})
	}
}

func TestParseNegative(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want error
	}{
		// Statement types.
		{"role grant", "GRANT admins TO app_user", ErrUnsupportedStatement},
		{"role revoke", "REVOKE admins FROM app_user", ErrUnsupportedStatement},
		{"do block", "DO $$ BEGIN PERFORM 1; END $$", ErrUnsupportedStatement},
		{"copy", "COPY t TO PROGRAM 'id'", ErrUnsupportedStatement},
		{"call", "CALL do_things()", ErrUnsupportedStatement},
		{"select", "SELECT 1", ErrUnsupportedStatement},
		{"alter default privileges", "ALTER DEFAULT PRIVILEGES GRANT SELECT ON TABLES TO app_user", ErrUnsupportedStatement},
		{"drop", "DROP TABLE t", ErrUnsupportedStatement},
		// Empty / syntax.
		{"empty", "", ErrSyntax},
		{"whitespace", "   \n\t", ErrSyntax},
		{"comment only", "-- nothing", ErrSyntax},
		{"garbage", "GRANT GRANT GRANT", ErrSyntax},
		// Unsupported objects.
		{"function", "GRANT EXECUTE ON FUNCTION f() TO app_user", ErrUnsupportedObject},
		{"all functions", "GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA app TO app_user", ErrUnsupportedObject},
		{"procedure", "GRANT EXECUTE ON PROCEDURE p() TO app_user", ErrUnsupportedObject},
		{"routine", "GRANT EXECUTE ON ROUTINE r() TO app_user", ErrUnsupportedObject},
		{"type", "GRANT USAGE ON TYPE ty TO app_user", ErrUnsupportedObject},
		{"domain", "GRANT USAGE ON DOMAIN d TO app_user", ErrUnsupportedObject},
		{"language", "GRANT USAGE ON LANGUAGE plpgsql TO app_user", ErrUnsupportedObject},
		{"large object", "GRANT SELECT ON LARGE OBJECT 1234 TO app_user", ErrUnsupportedObject},
		{"foreign server", "GRANT USAGE ON FOREIGN SERVER s TO app_user", ErrUnsupportedObject},
		{"foreign data wrapper", "GRANT USAGE ON FOREIGN DATA WRAPPER w TO app_user", ErrUnsupportedObject},
		{"tablespace", "GRANT CREATE ON TABLESPACE ts TO app_user", ErrUnsupportedObject},
		{"parameter", "GRANT SET ON PARAMETER work_mem TO app_user", ErrUnsupportedObject},
		{"database privilege on all tables in schema", "GRANT CONNECT ON ALL TABLES IN SCHEMA a TO app_user", ErrUnsupportedPrivilege},
		{"nul byte", "GRANT CONNECT ON DATABASE d TO app_user\x00; DROP TABLE t", ErrInvalidInput},
		{"nul byte in identifier", "GRANT CONNECT ON DATABASE \"d\x00x\" TO app_user", ErrInvalidInput},
		{"all tables in multiple schemas", "GRANT SELECT ON ALL TABLES IN SCHEMA a, b TO app_user", ErrUnsupportedObject},
		{"database qualified table", "GRANT SELECT ON TABLE db.app.t TO app_user", ErrUnsupportedObject},
		// Privileges.
		{"unknown privilege", "GRANT FOO ON DATABASE d TO app_user", ErrUnsupportedPrivilege},
		{"table privilege on database", "GRANT SELECT ON DATABASE d TO app_user", ErrUnsupportedPrivilege},
		{"connect on table", "GRANT CONNECT ON TABLE t TO app_user", ErrUnsupportedPrivilege},
		{"maintain on sequence", "GRANT MAINTAIN ON SEQUENCE s TO app_user", ErrUnsupportedPrivilege},
		{"delete on sequence", "GRANT DELETE ON SEQUENCE s TO app_user", ErrUnsupportedPrivilege},
		{"temporary on schema", "GRANT TEMPORARY ON SCHEMA s TO app_user", ErrUnsupportedPrivilege},
		{"delete as column privilege", "GRANT DELETE (a) ON TABLE t TO app_user", ErrUnsupportedPrivilege},
		{"truncate as column privilege", "GRANT TRUNCATE (a) ON TABLE t TO app_user", ErrUnsupportedPrivilege},
		// Options and grantees.
		{"public grantee", "GRANT SELECT ON TABLE t TO PUBLIC", ErrUnsupportedOption},
		{"current_user grantee", "GRANT SELECT ON TABLE t TO CURRENT_USER", ErrUnsupportedOption},
		{"session_user grantee", "GRANT SELECT ON TABLE t TO SESSION_USER", ErrUnsupportedOption},
		{"current_role grantee", "GRANT SELECT ON TABLE t TO CURRENT_ROLE", ErrUnsupportedOption},
		{"multiple grantees", "GRANT SELECT ON TABLE t TO app_user, other_user", ErrUnsupportedOption},
		{"granted by", "GRANT SELECT ON TABLE t TO app_user GRANTED BY admin", ErrUnsupportedOption},
		{"with grant option", "GRANT SELECT ON TABLE t TO app_user WITH GRANT OPTION", ErrUnsupportedOption},
		{"grant option for", "REVOKE GRANT OPTION FOR SELECT ON TABLE t FROM app_user", ErrUnsupportedOption},
		{"revoke cascade", "REVOKE SELECT ON TABLE t FROM app_user CASCADE", ErrUnsupportedOption},
		{"column list on database", "GRANT CONNECT (a) ON DATABASE d TO app_user", ErrUnsupportedOption},
		{"column list on sequence", "GRANT USAGE (a) ON SEQUENCE s TO app_user", ErrUnsupportedOption},
		{"column list on all tables", "GRANT SELECT (a) ON ALL TABLES IN SCHEMA s TO app_user", ErrUnsupportedOption},
		{"mixed column and table privileges", "GRANT SELECT (a), UPDATE ON TABLE t TO app_user", ErrUnsupportedOption},
		{"different column lists", "GRANT SELECT (a), UPDATE (b) ON TABLE t TO app_user", ErrUnsupportedOption},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.sql)
			if err == nil {
				t.Fatalf("Parse(%q) succeeded, want %v", tt.sql, tt.want)
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("Parse(%q) error = %v, want errors.Is %v", tt.sql, err, tt.want)
			}
			if !strings.Contains(err.Error(), "README") {
				t.Errorf("error %q does not point to the README matrix", err)
			}
		})
	}
}

func TestParseInjection(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want error
	}{
		{"semicolon chained", "GRANT CONNECT ON DATABASE d TO app_user; DROP TABLE users", ErrMultipleStatements},
		{"semicolon chained grants", "GRANT CONNECT ON DATABASE d TO app_user; GRANT CONNECT ON DATABASE e TO app_user", ErrMultipleStatements},
		{"comment hides second statement start", "GRANT CONNECT ON DATABASE d TO app_user; -- x\nDROP TABLE users", ErrMultipleStatements},
		{"block comment between statements", "GRANT CONNECT ON DATABASE d TO app_user;/* x */DROP TABLE users", ErrMultipleStatements},
		{"dollar quoted do", "DO $$ BEGIN EXECUTE 'DROP TABLE users'; END $$", ErrUnsupportedStatement},
		{"dollar quoted after grant", "GRANT CONNECT ON DATABASE d TO app_user; DO $$ BEGIN END $$", ErrMultipleStatements},
		{"unterminated block comment", "GRANT CONNECT ON DATABASE d TO app_user /*", ErrSyntax},
		{"unterminated quote", `GRANT CONNECT ON DATABASE "d TO app_user`, ErrSyntax},
		{"unterminated string", "GRANT CONNECT ON DATABASE d TO app_user; SELECT '", ErrSyntax},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.sql)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Parse(%q) error = %v, want errors.Is %v", tt.sql, err, tt.want)
			}
		})
	}
}

func TestDatabaseRuleInjection(t *testing.T) {
	// Privilege strings that try to smuggle extra SQL through the legacy
	// privileges/databases fields.
	privileges := []string{
		`CONNECT ON DATABASE x TO y; DROP TABLE users; --`,
		`CONNECT ON DATABASE "d" TO "app_user"; DROP TABLE users; --`,
		`CONNECT ON DATABASE x TO y --`,
		`SELECT ON TABLE t TO "app_user" --`,
		`CONNECT ON DATABASE "d" TO "app_user"; GRANT admins TO "app_user"; --`,
		`CONNECT; DO $$ BEGIN END $$; --`,
		`CONNECT TO PUBLIC; --`,
		`CONNECT ON DATABASE "d" TO PUBLIC; --`,
		`CONNECT ON DATABASE "d" TO "app_user" WITH GRANT OPTION; --`,
	}

	for _, priv := range privileges {
		t.Run(priv, func(t *testing.T) {
			rule, err := DatabaseRule(ActionGrant, priv, "d", "app_user")
			if err == nil {
				sql, _ := rule.SQL()
				t.Fatalf("DatabaseRule accepted %q -> %s", priv, sql)
			}
		})
	}

	// A privilege that is valid on its own is still rejected if it changes
	// the object or grantee.
	if _, err := DatabaseRule(ActionGrant, `SELECT ON TABLE t TO "app_user" --`, "d", "app_user"); err == nil {
		t.Fatal("expected object-type rejection")
	}
	if _, err := DatabaseRule(ActionGrant, `CONNECT ON DATABASE other TO "app_user" --`, "d", "app_user"); !errors.Is(err, ErrUnsupportedObject) {
		t.Fatalf("expected ErrUnsupportedObject, got %v", err)
	}
	if _, err := DatabaseRule(ActionGrant, `CONNECT ON DATABASE "d" TO other --`, "d", "app_user"); !errors.Is(err, ErrGranteeMismatch) {
		t.Fatalf("expected ErrGranteeMismatch, got %v", err)
	}

	// Legitimate privileges and hostile-looking database / principal names are
	// quoted, not interpreted.
	rule, err := DatabaseRule(ActionGrant, "connect", `d"; DROP TABLE users; --`, `we"ird`)
	if err != nil {
		t.Fatalf("DatabaseRule rejected quoted names: %v", err)
	}
	sql, err := rule.SQL()
	if err != nil {
		t.Fatal(err)
	}
	reparsed, err := Parse(sql)
	if err != nil {
		t.Fatalf("canonical SQL %q does not parse as a single statement: %v", sql, err)
	}
	if !reparsed.Equal(rule) || reparsed.Objects[0].Name != `d"; DROP TABLE users; --` || reparsed.Grantee != `we"ird` {
		t.Fatalf("hostile identifiers changed meaning in %s: %+v", sql, reparsed)
	}
}

func TestParseVersionCompatibility(t *testing.T) {
	// Syntax outside the PostgreSQL 17 grammar fails with a parse error that
	// names the grammar version.
	_, err := Parse("GRANT SELECT ON PROPERTY GRAPH g TO app_user")
	if !errors.Is(err, ErrSyntax) {
		t.Fatalf("error = %v, want errors.Is ErrSyntax", err)
	}
	if !strings.Contains(err.Error(), GrammarVersion) {
		t.Fatalf("error %q does not name the grammar version %q", err, GrammarVersion)
	}
	if GrammarVersion != "PostgreSQL 17" {
		t.Fatalf("GrammarVersion = %q, expected the pg_query_go v6 grammar", GrammarVersion)
	}
}

func TestNormalizationEquality(t *testing.T) {
	groups := map[string][]string{
		"case and whitespace": {
			"GRANT SELECT ON TABLE app.t TO app_user",
			"grant   select\non table APP.T to APP_USER",
			"GRANT SELECT ON app.t TO app_user;",
		},
		"quoted equals unquoted lower-case": {
			"GRANT SELECT ON TABLE t TO app_user",
			`GRANT SELECT ON TABLE "t" TO "app_user"`,
		},
		"ALL expansion on database": {
			"GRANT ALL ON DATABASE d TO app_user",
			"GRANT ALL PRIVILEGES ON DATABASE d TO app_user",
			"GRANT CONNECT, CREATE, TEMPORARY ON DATABASE d TO app_user",
		},
		"TEMP alias": {
			"GRANT TEMP ON DATABASE d TO app_user",
			"GRANT TEMPORARY ON DATABASE d TO app_user",
		},
		"privilege order and duplicates": {
			"GRANT SELECT, INSERT ON TABLE t TO app_user",
			"GRANT INSERT, SELECT ON TABLE t TO app_user",
			"GRANT INSERT, SELECT, INSERT ON TABLE t TO app_user",
		},
		"table ALL expansion includes MAINTAIN": {
			"GRANT ALL ON TABLE t TO app_user",
			"GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER, MAINTAIN ON TABLE t TO app_user",
		},
		"column list order": {
			"GRANT SELECT (a, b) ON TABLE t TO app_user",
			"GRANT SELECT (b, a) ON TABLE t TO app_user",
		},
		"column ALL expansion": {
			"GRANT ALL (a) ON TABLE t TO app_user",
			"GRANT SELECT (a), INSERT (a), UPDATE (a), REFERENCES (a) ON TABLE t TO app_user",
		},
		"object order": {
			"GRANT SELECT ON TABLE a, b TO app_user",
			"GRANT SELECT ON TABLE b, a TO app_user",
		},
	}

	for name, statements := range groups {
		t.Run(name, func(t *testing.T) {
			first, err := Parse(statements[0])
			if err != nil {
				t.Fatalf("Parse(%q): %v", statements[0], err)
			}
			for _, sql := range statements[1:] {
				other, err := Parse(sql)
				if err != nil {
					t.Fatalf("Parse(%q): %v", sql, err)
				}
				if !first.Equal(other) {
					t.Errorf("rules differ\n %q -> %+v\n %q -> %+v", statements[0], first, sql, other)
				}
			}
		})
	}

	t.Run("different statements stay different", func(t *testing.T) {
		a := mustParse(t, "GRANT SELECT ON TABLE t TO app_user")
		b := mustParse(t, "GRANT SELECT ON TABLE t TO other_user")
		c := mustParse(t, `GRANT SELECT ON TABLE "T" TO app_user`)
		if a.Equal(b) || a.Equal(c) {
			t.Fatal("distinct statements compared equal")
		}
	})
}

func TestRoundTrip(t *testing.T) {
	statements := []string{
		"GRANT CONNECT, TEMP ON DATABASE mydb TO app_user",
		"GRANT ALL ON DATABASE mydb TO app_user",
		"GRANT USAGE ON SCHEMA app TO app_user",
		"GRANT SELECT, INSERT ON TABLE app.orders, app.items TO app_user",
		"GRANT ALL ON TABLE app.orders TO app_user",
		"GRANT SELECT (a, b), UPDATE (a, b) ON TABLE t TO app_user",
		"GRANT ALL (a) ON TABLE t TO app_user",
		"GRANT SELECT ON ALL TABLES IN SCHEMA app TO app_user",
		"GRANT USAGE ON SEQUENCE app.seq TO app_user",
		"GRANT ALL ON ALL SEQUENCES IN SCHEMA app TO app_user",
		`GRANT SELECT ON TABLE "App"."Orders" TO "App_User"`,
		`GRANT CONNECT ON DATABASE "my db" TO "a""b"`,
		"REVOKE SELECT ON TABLE t FROM app_user",
	}

	for _, sql := range statements {
		t.Run(sql, func(t *testing.T) {
			rule, err := Parse(sql)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			canonical, err := rule.SQL()
			if err != nil {
				t.Fatalf("SQL: %v", err)
			}
			again, err := Parse(canonical)
			if err != nil {
				t.Fatalf("Parse(canonical %q): %v", canonical, err)
			}
			if !rule.Equal(again) {
				t.Fatalf("round trip changed rule\n sql: %s\n canonical: %s\n before: %+v\n after: %+v", sql, canonical, rule, again)
			}

			again2, err := again.SQL()
			if err != nil {
				t.Fatalf("SQL (second): %v", err)
			}
			if again2 != canonical {
				t.Fatalf("canonical SQL not stable: %q vs %q", canonical, again2)
			}
		})
	}
}

func TestInverse(t *testing.T) {
	statements := []string{
		"GRANT CONNECT ON DATABASE d TO app_user",
		"GRANT USAGE ON SCHEMA s TO app_user",
		"GRANT SELECT, INSERT ON TABLE s.t TO app_user",
		"GRANT SELECT (a) ON TABLE t TO app_user",
		"GRANT SELECT ON ALL TABLES IN SCHEMA s TO app_user",
		"GRANT USAGE ON SEQUENCE s.q TO app_user",
	}

	for _, sql := range statements {
		t.Run(sql, func(t *testing.T) {
			rule, err := Validate(sql, principal, ActionGrant)
			if err != nil {
				t.Fatal(err)
			}
			inv := rule.Inverse()
			if inv.Action != ActionRevoke {
				t.Fatalf("Inverse action = %s", inv.Action)
			}
			revokeSQL, err := inv.SQL()
			if err != nil {
				t.Fatalf("Inverse().SQL(): %v", err)
			}
			if !strings.HasPrefix(revokeSQL, "REVOKE ") {
				t.Fatalf("expected REVOKE, got %q", revokeSQL)
			}
			parsed, err := Validate(revokeSQL, principal, ActionRevoke)
			if err != nil {
				t.Fatalf("Validate(%q, Revoke): %v", revokeSQL, err)
			}
			if !parsed.Equal(inv) {
				t.Fatalf("revoke round trip differs: %+v vs %+v", parsed, inv)
			}
			if !inv.Inverse().Equal(rule) {
				t.Fatal("double inverse is not the original rule")
			}
			if _, err := Validate(revokeSQL, principal, ActionGrant); !errors.Is(err, ErrActionNotAllowed) {
				t.Fatalf("revoke through grant-only validation error = %v", err)
			}
		})
	}

	t.Run("does not alias receiver", func(t *testing.T) {
		rule := mustParse(t, "GRANT SELECT, INSERT ON TABLE a, b TO app_user")
		inv := rule.Inverse()
		inv.Privileges[0] = PrivDelete
		inv.Objects[0].Name = "zzz"
		if rule.Privileges[0] == PrivDelete || rule.Objects[0].Name == "zzz" {
			t.Fatal("Inverse shares slices with the original rule")
		}
	})
}

func TestValidateGranteeAndAction(t *testing.T) {
	t.Run("grantee matches", func(t *testing.T) {
		if _, err := Validate("GRANT CONNECT ON DATABASE d TO app_user", "app_user"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("unquoted grantee is lower-cased", func(t *testing.T) {
		if _, err := Validate("GRANT CONNECT ON DATABASE d TO App_User", "app_user"); err != nil {
			t.Fatal(err)
		}
		if _, err := Validate("GRANT CONNECT ON DATABASE d TO App_User", "App_User"); !errors.Is(err, ErrGranteeMismatch) {
			t.Fatalf("unquoted mixed-case grantee must not match mixed-case role, got %v", err)
		}
	})

	t.Run("quoted grantee preserves case", func(t *testing.T) {
		if _, err := Validate(`GRANT CONNECT ON DATABASE d TO "App_User"`, "App_User"); err != nil {
			t.Fatal(err)
		}
		if _, err := Validate(`GRANT CONNECT ON DATABASE d TO "App_User"`, "app_user"); !errors.Is(err, ErrGranteeMismatch) {
			t.Fatalf("quoted mixed-case grantee must not match lower-case role, got %v", err)
		}
	})

	t.Run("other grantee", func(t *testing.T) {
		_, err := Validate("GRANT CONNECT ON DATABASE d TO someone_else", "app_user")
		if !errors.Is(err, ErrGranteeMismatch) {
			t.Fatalf("error = %v", err)
		}
		if !strings.Contains(err.Error(), "someone_else") || !strings.Contains(err.Error(), "app_user") {
			t.Fatalf("error should name both roles: %v", err)
		}
	})

	t.Run("empty allow defaults to grant and revoke", func(t *testing.T) {
		if _, err := Validate("GRANT CONNECT ON DATABASE d TO app_user", "app_user"); err != nil {
			t.Fatalf("grant: %v", err)
		}
		if _, err := Validate("REVOKE CONNECT ON DATABASE d FROM app_user", "app_user"); err != nil {
			t.Fatalf("revoke: %v", err)
		}
	})

	t.Run("grant only rejects revoke", func(t *testing.T) {
		_, err := Validate("REVOKE CONNECT ON DATABASE d FROM app_user", "app_user", ActionGrant)
		if !errors.Is(err, ErrActionNotAllowed) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("revoke only rejects grant", func(t *testing.T) {
		_, err := Validate("GRANT CONNECT ON DATABASE d TO app_user", "app_user", ActionRevoke)
		if !errors.Is(err, ErrActionNotAllowed) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("parse errors propagate", func(t *testing.T) {
		if _, err := Validate("SELECT 1", "app_user"); !errors.Is(err, ErrUnsupportedStatement) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestSQLRejectsInvalidRules(t *testing.T) {
	valid := Rule{
		Action:     ActionGrant,
		ObjectType: ObjectTable,
		Objects:    []ObjectName{{Name: "t"}},
		Privileges: []Privilege{PrivSelect},
		Grantee:    "app_user",
	}
	if _, err := valid.SQL(); err != nil {
		t.Fatalf("valid rule: %v", err)
	}

	mutations := map[string]func(Rule) Rule{
		"zero rule":          func(Rule) Rule { return Rule{} },
		"no action":          func(r Rule) Rule { r.Action = 0; return r },
		"no grantee":         func(r Rule) Rule { r.Grantee = ""; return r },
		"no objects":         func(r Rule) Rule { r.Objects = nil; return r },
		"no privileges":      func(r Rule) Rule { r.Privileges = nil; return r },
		"bad privilege":      func(r Rule) Rule { r.Privileges = []Privilege{"CONNECT"}; return r },
		"injected privilege": func(r Rule) Rule { r.Privileges = []Privilege{"SELECT; DROP TABLE t"}; return r },
		"empty object name":  func(r Rule) Rule { r.Objects = []ObjectName{{}}; return r },
		"unsupported object": func(r Rule) Rule { r.ObjectType = 99; return r },
		"column on database": func(r Rule) Rule {
			r.ObjectType = ObjectDatabase
			r.Privileges = []Privilege{PrivConnect}
			r.Columns = []string{"a"}
			return r
		},
		"column privilege bad": func(r Rule) Rule { r.Privileges = []Privilege{PrivDelete}; r.Columns = []string{"a"}; return r },
		"objects and all":      func(r Rule) Rule { r.AllInSchema = "s"; return r },
		"all in schema schema": func(r Rule) Rule { r.ObjectType = ObjectSchema; r.Objects = nil; r.AllInSchema = "s"; return r },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			if _, err := mutate(valid).SQL(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestSQLQuotesHostileIdentifiers(t *testing.T) {
	rule := Rule{
		Action:     ActionGrant,
		ObjectType: ObjectTable,
		Objects:    []ObjectName{{Schema: `s"; DROP SCHEMA x; --`, Name: `t"; DROP TABLE y; --`}},
		Privileges: []Privilege{PrivSelect},
		Grantee:    `u"; DROP ROLE z; --`,
	}
	sql, err := rule.SQL()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(sql)
	if err != nil {
		t.Fatalf("canonical SQL %q does not parse as a single grant: %v", sql, err)
	}
	if !parsed.Equal(rule) {
		t.Fatalf("hostile identifiers changed meaning: %+v vs %+v", parsed, rule)
	}
}

func TestDatabaseRule(t *testing.T) {
	rule, err := DatabaseRule(ActionGrant, "CONNECT", "mydb", "app_user")
	if err != nil {
		t.Fatal(err)
	}
	want := Rule{Action: ActionGrant, ObjectType: ObjectDatabase, Objects: []ObjectName{{Name: "mydb"}}, Privileges: []Privilege{PrivConnect}, Grantee: "app_user"}
	if !reflect.DeepEqual(rule, want) {
		t.Fatalf("got %+v want %+v", rule, want)
	}

	revoke, err := DatabaseRule(ActionRevoke, "ALL", "mydb", "app_user")
	if err != nil {
		t.Fatal(err)
	}
	if revoke.Action != ActionRevoke || len(revoke.Privileges) != 3 {
		t.Fatalf("unexpected revoke rule %+v", revoke)
	}

	if _, err := DatabaseRule(ActionGrant, "SELECT", "mydb", "app_user"); !errors.Is(err, ErrUnsupportedPrivilege) {
		t.Fatalf("expected ErrUnsupportedPrivilege, got %v", err)
	}
	if _, err := DatabaseRule(0, "CONNECT", "mydb", "app_user"); !errors.Is(err, ErrActionNotAllowed) {
		t.Fatalf("expected ErrActionNotAllowed, got %v", err)
	}
}

func TestCanonicalSQLPreservesAll(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{"table all", "GRANT ALL ON TABLE t TO app_user", "GRANT ALL ON t TO app_user"},
		{"table all privileges", "GRANT ALL PRIVILEGES ON TABLE s.t TO app_user", "GRANT ALL ON s.t TO app_user"},
		{"table explicit full set", "GRANT SELECT, INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER, MAINTAIN ON TABLE t TO app_user", "GRANT ALL ON t TO app_user"},
		{"all tables in schema", "GRANT ALL ON ALL TABLES IN SCHEMA s TO app_user", "GRANT ALL ON ALL TABLES IN SCHEMA s TO app_user"},
		{"database all", "GRANT ALL ON DATABASE d TO app_user", "GRANT ALL ON DATABASE d TO app_user"},
		{"schema all", "GRANT ALL ON SCHEMA s TO app_user", "GRANT ALL ON SCHEMA s TO app_user"},
		{"sequence all", "GRANT ALL ON SEQUENCE q TO app_user", "GRANT ALL ON SEQUENCE q TO app_user"},
		{"revoke all", "REVOKE ALL ON TABLE t FROM app_user", "REVOKE ALL ON t FROM app_user"},
		{"column all", "GRANT ALL (a, b) ON TABLE t TO app_user", "GRANT ALL (a, b) ON t TO app_user"},
		{"partial table set is explicit", "GRANT SELECT, INSERT ON TABLE t TO app_user", "GRANT insert, select ON t TO app_user"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := mustParse(t, tt.sql)
			got, err := rule.SQL()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.EqualFold(got, tt.want) {
				t.Fatalf("SQL() = %q, want %q", got, tt.want)
			}
			if strings.Contains(strings.ToUpper(tt.want), "ALL") && strings.Contains(strings.ToUpper(got), "MAINTAIN") {
				t.Fatalf("canonical SQL %q expands ALL with MAINTAIN", got)
			}
			again := mustParse(t, got)
			if !again.Equal(rule) {
				t.Fatalf("round trip changed rule: %+v vs %+v", again, rule)
			}
		})
	}
}

func TestSQLRejectsNULInRule(t *testing.T) {
	rule := Rule{
		Action:     ActionGrant,
		ObjectType: ObjectTable,
		Objects:    []ObjectName{{Name: "t\x00; DROP TABLE x"}},
		Privileges: []Privilege{PrivSelect},
		Grantee:    "app_user",
	}
	if _, err := rule.SQL(); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("SQL() error = %v, want ErrInvalidInput", err)
	}
	rule.Objects[0].Name = "t"
	rule.Grantee = "app\x00user"
	if _, err := rule.SQL(); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("SQL() error = %v, want ErrInvalidInput", err)
	}
}

func TestDatabaseRuleRejectsNUL(t *testing.T) {
	for name, args := range map[string][3]string{
		"privilege": {"CONNECT\x00", "d", "app_user"},
		"database":  {"CONNECT", "d\x00x", "app_user"},
		"principal": {"CONNECT", "d", "app\x00user"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DatabaseRule(ActionGrant, args[0], args[1], args[2]); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error = %v, want ErrInvalidInput", err)
			}
		})
	}
}
