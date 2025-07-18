package structs

import "time"

// Config represents the overall configuration for the user manager
type Config struct {
	Users  []UserConfig  `json:"users"`
	Groups []GroupConfig `json:"groups"`
}

// UserConfig represents a user configuration from the config file
type UserConfig struct {
	Username        string   `json:"username"`
	Password        string   `json:"password,omitempty"`        // Optional, not used for IAM auth
	Groups          []string `json:"groups"`
	Privileges      []string `json:"privileges"`
	Databases       []string `json:"databases"`
	Enabled         bool     `json:"enabled"`
	Description     string   `json:"description,omitempty"`
	AuthMethod      string   `json:"auth_method,omitempty"`     // "iam" or "password" (default: "password")
	IAMRole         string   `json:"iam_role,omitempty"`        // AWS IAM role ARN for IAM authentication
	CanLogin        bool     `json:"can_login"`                 // Whether user can login (default: true)
	ConnectionLimit int      `json:"connection_limit,omitempty"` // Max connections (default: -1, unlimited)
}

// GroupConfig represents a group/role configuration
type GroupConfig struct {
	Name        string   `json:"name"`
	Privileges  []string `json:"privileges"`
	Databases   []string `json:"databases"`
	Description string   `json:"description,omitempty"`
	Inherit     bool     `json:"inherit"`
}

// DatabaseUser represents an actual database user
type DatabaseUser struct {
	Username    string
	Groups      []string
	Privileges  []string
	Databases   []string
	Exists      bool
	LastChecked time.Time
}

// DatabaseGroup represents an actual database role/group
type DatabaseGroup struct {
	Name        string
	Privileges  []string
	Databases   []string
	Members     []string
	Exists      bool
	LastChecked time.Time
}

// OperationResult represents the result of a user management operation
type OperationResult struct {
	Operation string
	Target    string
	Success   bool
	Message   string
	Error     error
}

// SyncResult represents the result of a synchronization operation
type SyncResult struct {
	UsersCreated   []string
	UsersModified  []string
	UsersRemoved   []string
	GroupsCreated  []string
	GroupsModified []string
	GroupsRemoved  []string
	Errors         []error
}

// DatabaseConnection represents database connection configuration
type DatabaseConnection struct {
	Host          string
	Port          int
	Database      string
	Username      string
	Password      string
	SSLMode       string
	IAMAuth       bool   // Whether to use IAM authentication for connection
	AWSRegion     string // AWS region for IAM auth
	IAMToken      string // IAM auth token (if using IAM authentication)
}

// EventPayload represents a future AWS Cognito event payload
type EventPayload struct {
	EventType string                 `json:"eventType"`
	UserID    string                 `json:"userId"`
	Username  string                 `json:"username"`
	Groups    []string               `json:"groups"`
	Metadata  map[string]interface{} `json:"metadata"`
	Timestamp time.Time              `json:"timestamp"`
}