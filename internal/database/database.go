package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/structs"
	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/sirupsen/logrus"
)

// Manager handles database operations
type Manager struct {
	db     *sql.DB
	logger *logrus.Logger
	dryRun bool
}

// NewManager creates a new database manager with support for IAM authentication
func NewManager(conn *structs.DatabaseConnection, logger *logrus.Logger, dryRun bool) (*Manager, error) {
	var connStr string
	
	if conn.IAMAuth {
		// For IAM authentication, use the IAM token as password
		// Note: In a real implementation, you'd generate the IAM token using AWS SDK
		logger.Info("Setting up database connection with IAM authentication")
		
		password := conn.IAMToken
		if password == "" {
			// In production, you would generate the IAM token here using AWS SDK
			// For now, we'll use a placeholder to indicate IAM auth is being used
			logger.Warn("IAM token not provided - in production this would be generated using AWS SDK")
			password = "PLACEHOLDER_IAM_TOKEN"
		}
		
		connStr = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			conn.Host, conn.Port, conn.Username, password, conn.Database, conn.SSLMode)
	} else {
		// Traditional password authentication
		logger.Info("Setting up database connection with password authentication")
		connStr = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			conn.Host, conn.Port, conn.Username, conn.Password, conn.Database, conn.SSLMode)
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// Test the connection (skip ping for dry run mode to avoid auth issues during development)
	if !dryRun {
		if err := db.Ping(); err != nil {
			return nil, fmt.Errorf("failed to ping database: %w", err)
		}
		logger.Info("Database connection established successfully")
	} else {
		logger.Info("Database connection configured (skipping ping in dry-run mode)")
	}

	return &Manager{
		db:     db,
		logger: logger,
		dryRun: dryRun,
	}, nil
}

// Close closes the database connection
func (m *Manager) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

// CreateUser creates a new database user with support for IAM authentication
func (m *Manager) CreateUser(user *structs.UserConfig) error {
	m.logger.WithFields(logrus.Fields{
		"username":    user.Username,
		"auth_method": user.AuthMethod,
	}).Info("Creating user")

	// Check if user already exists
	exists, err := m.UserExists(user.Username)
	if err != nil {
		return fmt.Errorf("failed to check if user exists: %w", err)
	}

	if exists {
		m.logger.WithField("username", user.Username).Info("User already exists, skipping creation")
		return nil
	}

	// Build CREATE USER query based on authentication method
	query := m.buildCreateUserQuery(user)

	if m.dryRun {
		m.logger.WithField("query", query).Info("DRY RUN: Would execute query")
		return nil
	}

	if _, err := m.db.Exec(query); err != nil {
		return fmt.Errorf("failed to create user %s: %w", user.Username, err)
	}

	// For IAM authentication, grant rds_iam role
	if user.AuthMethod == "iam" {
		if err := m.grantRDSIAMRole(user.Username); err != nil {
			return fmt.Errorf("failed to grant rds_iam role to user %s: %w", user.Username, err)
		}
	}

	m.logger.WithField("username", user.Username).Info("User created successfully")
	return nil
}

// buildCreateUserQuery builds the appropriate CREATE USER query based on auth method
func (m *Manager) buildCreateUserQuery(user *structs.UserConfig) string {
	query := fmt.Sprintf("CREATE USER %s", m.quoteIdentifier(user.Username))
	
	// Set authentication method specific options
	switch user.AuthMethod {
	case "iam":
		// For IAM authentication, no password is needed
		// The user will authenticate using AWS IAM
		m.logger.WithField("username", user.Username).Info("Creating user for IAM authentication (no password)")
		
	default:
		// Traditional password authentication
		if user.Password != "" {
			query += fmt.Sprintf(" WITH PASSWORD '%s'", m.escapeString(user.Password))
		}
	}
	
	// Add LOGIN/NOLOGIN based on CanLogin setting
	if user.CanLogin {
		query += " LOGIN"
	} else {
		query += " NOLOGIN"
	}
	
	// Set connection limit if specified
	if user.ConnectionLimit != 0 {
		if user.ConnectionLimit == -1 {
			query += " CONNECTION LIMIT -1" // Unlimited
		} else {
			query += fmt.Sprintf(" CONNECTION LIMIT %d", user.ConnectionLimit)
		}
	}
	
	return query
}

// grantRDSIAMRole grants the rds_iam role to a user for IAM authentication
func (m *Manager) grantRDSIAMRole(username string) error {
	m.logger.WithField("username", username).Info("Granting rds_iam role for IAM authentication")
	
	query := fmt.Sprintf("GRANT rds_iam TO %s", m.quoteIdentifier(username))
	
	if m.dryRun {
		m.logger.WithField("query", query).Info("DRY RUN: Would execute query")
		return nil
	}

	if _, err := m.db.Exec(query); err != nil {
		return fmt.Errorf("failed to grant rds_iam role: %w", err)
	}
	
	m.logger.WithField("username", username).Info("Successfully granted rds_iam role")
	return nil
}

// revokeRDSIAMRole revokes the rds_iam role from a user
func (m *Manager) revokeRDSIAMRole(username string) error {
	m.logger.WithField("username", username).Info("Revoking rds_iam role")
	
	query := fmt.Sprintf("REVOKE rds_iam FROM %s", m.quoteIdentifier(username))
	
	if m.dryRun {
		m.logger.WithField("query", query).Info("DRY RUN: Would execute query")
		return nil
	}

	if _, err := m.db.Exec(query); err != nil {
		return fmt.Errorf("failed to revoke rds_iam role: %w", err)
	}
	
	m.logger.WithField("username", username).Info("Successfully revoked rds_iam role")
	return nil
}

// DropUser removes a database user
func (m *Manager) DropUser(username string) error {
	m.logger.WithField("username", username).Info("Dropping user")

	// Check if user exists
	exists, err := m.UserExists(username)
	if err != nil {
		return fmt.Errorf("failed to check if user exists: %w", err)
	}

	if !exists {
		m.logger.WithField("username", username).Info("User does not exist, skipping deletion")
		return nil
	}

	query := fmt.Sprintf("DROP USER %s", m.quoteIdentifier(username))

	if m.dryRun {
		m.logger.WithField("query", query).Info("DRY RUN: Would execute query")
		return nil
	}

	if _, err := m.db.Exec(query); err != nil {
		return fmt.Errorf("failed to drop user %s: %w", username, err)
	}

	m.logger.WithField("username", username).Info("User dropped successfully")
	return nil
}

// CreateGroup creates a new database role/group
func (m *Manager) CreateGroup(group *structs.GroupConfig) error {
	m.logger.WithField("group", group.Name).Info("Creating group")

	// Check if group already exists
	exists, err := m.GroupExists(group.Name)
	if err != nil {
		return fmt.Errorf("failed to check if group exists: %w", err)
	}

	if exists {
		m.logger.WithField("group", group.Name).Info("Group already exists, skipping creation")
		return nil
	}

	// Build CREATE ROLE query
	query := fmt.Sprintf("CREATE ROLE %s", m.quoteIdentifier(group.Name))
	
	if group.Inherit {
		query += " INHERIT"
	} else {
		query += " NOINHERIT"
	}

	if m.dryRun {
		m.logger.WithField("query", query).Info("DRY RUN: Would execute query")
		return nil
	}

	if _, err := m.db.Exec(query); err != nil {
		return fmt.Errorf("failed to create group %s: %w", group.Name, err)
	}

	m.logger.WithField("group", group.Name).Info("Group created successfully")
	return nil
}

// GrantPrivileges grants privileges to a user or group
func (m *Manager) GrantPrivileges(target string, privileges []string, databases []string) error {
	m.logger.WithFields(logrus.Fields{
		"target":     target,
		"privileges": privileges,
		"databases":  databases,
	}).Info("Granting privileges")

	for _, db := range databases {
		for _, priv := range privileges {
			query := fmt.Sprintf("GRANT %s ON DATABASE %s TO %s", 
				priv, m.quoteIdentifier(db), m.quoteIdentifier(target))

			if m.dryRun {
				m.logger.WithField("query", query).Info("DRY RUN: Would execute query")
				continue
			}

			if _, err := m.db.Exec(query); err != nil {
				return fmt.Errorf("failed to grant %s on %s to %s: %w", priv, db, target, err)
			}
		}
	}

	m.logger.WithField("target", target).Info("Privileges granted successfully")
	return nil
}

// RevokePrivileges revokes privileges from a user or group
func (m *Manager) RevokePrivileges(target string, privileges []string, databases []string) error {
	m.logger.WithFields(logrus.Fields{
		"target":     target,
		"privileges": privileges,
		"databases":  databases,
	}).Info("Revoking privileges")

	for _, db := range databases {
		for _, priv := range privileges {
			query := fmt.Sprintf("REVOKE %s ON DATABASE %s FROM %s", 
				priv, m.quoteIdentifier(db), m.quoteIdentifier(target))

			if m.dryRun {
				m.logger.WithField("query", query).Info("DRY RUN: Would execute query")
				continue
			}

			if _, err := m.db.Exec(query); err != nil {
				return fmt.Errorf("failed to revoke %s on %s from %s: %w", priv, db, target, err)
			}
		}
	}

	m.logger.WithField("target", target).Info("Privileges revoked successfully")
	return nil
}

// AddUserToGroup adds a user to a group
func (m *Manager) AddUserToGroup(username, groupName string) error {
	m.logger.WithFields(logrus.Fields{
		"username": username,
		"group":    groupName,
	}).Info("Adding user to group")

	query := fmt.Sprintf("GRANT %s TO %s", m.quoteIdentifier(groupName), m.quoteIdentifier(username))

	if m.dryRun {
		m.logger.WithField("query", query).Info("DRY RUN: Would execute query")
		return nil
	}

	if _, err := m.db.Exec(query); err != nil {
		return fmt.Errorf("failed to add user %s to group %s: %w", username, groupName, err)
	}

	m.logger.WithFields(logrus.Fields{
		"username": username,
		"group":    groupName,
	}).Info("User added to group successfully")
	return nil
}

// RemoveUserFromGroup removes a user from a group
func (m *Manager) RemoveUserFromGroup(username, groupName string) error {
	m.logger.WithFields(logrus.Fields{
		"username": username,
		"group":    groupName,
	}).Info("Removing user from group")

	query := fmt.Sprintf("REVOKE %s FROM %s", m.quoteIdentifier(groupName), m.quoteIdentifier(username))

	if m.dryRun {
		m.logger.WithField("query", query).Info("DRY RUN: Would execute query")
		return nil
	}

	if _, err := m.db.Exec(query); err != nil {
		return fmt.Errorf("failed to remove user %s from group %s: %w", username, groupName, err)
	}

	m.logger.WithFields(logrus.Fields{
		"username": username,
		"group":    groupName,
	}).Info("User removed from group successfully")
	return nil
}

// UserExists checks if a user exists in the database
func (m *Manager) UserExists(username string) (bool, error) {
	query := "SELECT 1 FROM pg_user WHERE usename = $1"
	
	var exists int
	err := m.db.QueryRow(query, username).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	
	return true, nil
}

// GroupExists checks if a group/role exists in the database
func (m *Manager) GroupExists(groupName string) (bool, error) {
	query := "SELECT 1 FROM pg_roles WHERE rolname = $1"
	
	var exists int
	err := m.db.QueryRow(query, groupName).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	
	return true, nil
}

// GetUserInfo retrieves information about a database user
func (m *Manager) GetUserInfo(username string) (*structs.DatabaseUser, error) {
	user := &structs.DatabaseUser{
		Username:    username,
		LastChecked: time.Now(),
	}

	// Check if user exists
	exists, err := m.UserExists(username)
	if err != nil {
		return nil, err
	}
	user.Exists = exists

	if !exists {
		return user, nil
	}

	// Get user's groups
	groupQuery := `
		SELECT r.rolname 
		FROM pg_auth_members m 
		JOIN pg_roles r ON m.roleid = r.oid 
		JOIN pg_roles u ON m.member = u.oid 
		WHERE u.rolname = $1`
	
	rows, err := m.db.Query(groupQuery, username)
	if err != nil {
		return nil, fmt.Errorf("failed to get user groups: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var groupName string
		if err := rows.Scan(&groupName); err != nil {
			return nil, err
		}
		user.Groups = append(user.Groups, groupName)
	}

	return user, nil
}

// SyncConfiguration synchronizes the database state with the configuration
func (m *Manager) SyncConfiguration(config *structs.Config) (*structs.SyncResult, error) {
	m.logger.Info("Starting configuration synchronization")
	
	result := &structs.SyncResult{}

	// Create groups first (since users might depend on them)
	for _, group := range config.Groups {
		if err := m.CreateGroup(&group); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to create group %s: %w", group.Name, err))
			continue
		}
		result.GroupsCreated = append(result.GroupsCreated, group.Name)

		// Grant group privileges
		if err := m.GrantPrivileges(group.Name, group.Privileges, group.Databases); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to grant privileges to group %s: %w", group.Name, err))
		}
	}

	// Create and configure users
	for _, user := range config.Users {
		if !user.Enabled {
			m.logger.WithField("username", user.Username).Info("User is disabled, skipping")
			continue
		}

		if err := m.CreateUser(&user); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to create user %s: %w", user.Username, err))
			continue
		}
		result.UsersCreated = append(result.UsersCreated, user.Username)

		// Add user to groups
		for _, groupName := range user.Groups {
			if err := m.AddUserToGroup(user.Username, groupName); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("failed to add user %s to group %s: %w", user.Username, groupName, err))
			}
		}

		// Grant user privileges
		if err := m.GrantPrivileges(user.Username, user.Privileges, user.Databases); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to grant privileges to user %s: %w", user.Username, err))
		}
	}

	m.logger.WithFields(logrus.Fields{
		"users_created":  len(result.UsersCreated),
		"groups_created": len(result.GroupsCreated),
		"errors":         len(result.Errors),
	}).Info("Configuration synchronization completed")

	return result, nil
}

// Helper methods

// quoteIdentifier safely quotes database identifiers
func (m *Manager) quoteIdentifier(name string) string {
	return fmt.Sprintf(`"%s"`, strings.ReplaceAll(name, `"`, `""`))
}

// escapeString safely escapes string literals
func (m *Manager) escapeString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}