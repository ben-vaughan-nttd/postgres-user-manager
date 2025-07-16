package structs

import (
	"encoding/json"
	"testing"
	"time"
)

func TestUserConfigValidation(t *testing.T) {
	tests := []struct {
		name     string
		user     UserConfig
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid password user",
			user: UserConfig{
				Username:   "test_user",
				Password:   "secure_pass",
				AuthMethod: "password",
				Enabled:    true,
				CanLogin:   true,
			},
			wantErr: false,
		},
		{
			name: "valid IAM user",
			user: UserConfig{
				Username:   "iam_user",
				AuthMethod: "iam",
				IAMRole:    "arn:aws:iam::123456789012:role/TestRole",
				Enabled:    true,
				CanLogin:   true,
			},
			wantErr: false,
		},
		{
			name: "user with connection limit",
			user: UserConfig{
				Username:        "limited_user",
				AuthMethod:      "password",
				ConnectionLimit: 10,
				Enabled:         true,
				CanLogin:        true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling/unmarshaling
			data, err := json.Marshal(tt.user)
			if err != nil {
				t.Fatalf("Failed to marshal user: %v", err)
			}

			var unmarshaled UserConfig
			if err := json.Unmarshal(data, &unmarshaled); err != nil {
				t.Fatalf("Failed to unmarshal user: %v", err)
			}

			if unmarshaled.Username != tt.user.Username {
				t.Errorf("Username mismatch: got %s, want %s", unmarshaled.Username, tt.user.Username)
			}

			if unmarshaled.AuthMethod != tt.user.AuthMethod {
				t.Errorf("AuthMethod mismatch: got %s, want %s", unmarshaled.AuthMethod, tt.user.AuthMethod)
			}
		})
	}
}

func TestGroupConfigValidation(t *testing.T) {
	tests := []struct {
		name  string
		group GroupConfig
	}{
		{
			name: "basic group",
			group: GroupConfig{
				Name:        "test_group",
				Privileges:  []string{"CONNECT", "CREATE"},
				Databases:   []string{"test_db"},
				Description: "Test group",
				Inherit:     true,
			},
		},
		{
			name: "group without inheritance",
			group: GroupConfig{
				Name:        "noinherit_group",
				Privileges:  []string{"CONNECT"},
				Databases:   []string{"test_db"},
				Description: "No inherit group",
				Inherit:     false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.group)
			if err != nil {
				t.Fatalf("Failed to marshal group: %v", err)
			}

			var unmarshaled GroupConfig
			if err := json.Unmarshal(data, &unmarshaled); err != nil {
				t.Fatalf("Failed to unmarshal group: %v", err)
			}

			if unmarshaled.Name != tt.group.Name {
				t.Errorf("Name mismatch: got %s, want %s", unmarshaled.Name, tt.group.Name)
			}

			if unmarshaled.Inherit != tt.group.Inherit {
				t.Errorf("Inherit mismatch: got %v, want %v", unmarshaled.Inherit, tt.group.Inherit)
			}
		})
	}
}

func TestDatabaseConnectionValidation(t *testing.T) {
	tests := []struct {
		name string
		conn DatabaseConnection
	}{
		{
			name: "password connection",
			conn: DatabaseConnection{
				Host:     "localhost",
				Port:     5432,
				Database: "test",
				Username: "user",
				Password: "pass",
				SSLMode:  "require",
				IAMAuth:  false,
			},
		},
		{
			name: "IAM connection",
			conn: DatabaseConnection{
				Host:      "rds-host",
				Port:      5432,
				Database:  "test",
				Username:  "user",
				SSLMode:   "require",
				IAMAuth:   true,
				AWSRegion: "us-east-1",
				IAMToken:  "test-token",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.conn.Host == "" {
				t.Error("Host should not be empty")
			}

			if tt.conn.Port <= 0 {
				t.Error("Port should be positive")
			}

			if tt.conn.IAMAuth && tt.conn.AWSRegion == "" {
				t.Error("AWSRegion should be set for IAM auth")
			}

			if !tt.conn.IAMAuth && tt.conn.Password == "" {
				t.Error("Password should be set for non-IAM auth")
			}
		})
	}
}

func TestOperationResult(t *testing.T) {
	result := OperationResult{
		Operation: "CREATE_USER",
		Target:    "test_user",
		Success:   true,
		Message:   "User created successfully",
		Error:     nil,
	}

	if result.Operation != "CREATE_USER" {
		t.Errorf("Expected operation CREATE_USER, got %s", result.Operation)
	}

	if !result.Success {
		t.Error("Expected success to be true")
	}

	if result.Error != nil {
		t.Errorf("Expected no error, got %v", result.Error)
	}
}

func TestSyncResult(t *testing.T) {
	result := SyncResult{
		UsersCreated:   []string{"user1", "user2"},
		UsersModified:  []string{"user3"},
		UsersRemoved:   []string{"user4"},
		GroupsCreated:  []string{"group1"},
		GroupsModified: []string{"group2"},
		GroupsRemoved:  []string{"group3"},
		Errors:         []error{},
	}

	if len(result.UsersCreated) != 2 {
		t.Errorf("Expected 2 users created, got %d", len(result.UsersCreated))
	}

	if len(result.Errors) != 0 {
		t.Errorf("Expected no errors, got %d", len(result.Errors))
	}
}

func TestEventPayload(t *testing.T) {
	now := time.Now()
	event := EventPayload{
		EventType: "PostConfirmation_ConfirmSignUp",
		UserID:    "123456",
		Username:  "test_user",
		Groups:    []string{"group1", "group2"},
		Metadata:  map[string]interface{}{"key": "value"},
		Timestamp: now,
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal event: %v", err)
	}

	var unmarshaled EventPayload
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal event: %v", err)
	}

	if unmarshaled.EventType != event.EventType {
		t.Errorf("EventType mismatch: got %s, want %s", unmarshaled.EventType, event.EventType)
	}

	if len(unmarshaled.Groups) != len(event.Groups) {
		t.Errorf("Groups length mismatch: got %d, want %d", len(unmarshaled.Groups), len(event.Groups))
	}
}

func TestConfigCompleteStructure(t *testing.T) {
	config := Config{
		Users: []UserConfig{
			{
				Username:        "test_user",
				Password:        "pass",
				Groups:          []string{"group1"},
				Privileges:      []string{"CONNECT"},
				Databases:       []string{"db1"},
				Enabled:         true,
				Description:     "Test user",
				AuthMethod:      "password",
				CanLogin:        true,
				ConnectionLimit: 10,
			},
		},
		Groups: []GroupConfig{
			{
				Name:        "group1",
				Privileges:  []string{"CONNECT"},
				Databases:   []string{"db1"},
				Description: "Test group",
				Inherit:     true,
			},
		},
	}

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	var unmarshaled Config
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	if len(unmarshaled.Users) != 1 {
		t.Errorf("Expected 1 user, got %d", len(unmarshaled.Users))
	}

	if len(unmarshaled.Groups) != 1 {
		t.Errorf("Expected 1 group, got %d", len(unmarshaled.Groups))
	}

	user := unmarshaled.Users[0]
	if user.Username != "test_user" {
		t.Errorf("Expected username test_user, got %s", user.Username)
	}

	if user.ConnectionLimit != 10 {
		t.Errorf("Expected connection limit 10, got %d", user.ConnectionLimit)
	}
}
