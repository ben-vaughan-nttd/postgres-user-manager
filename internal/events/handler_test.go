package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/structs"
	"github.com/sirupsen/logrus"
)

const (
	failedMarshalEvent   = "Failed to marshal event: %v"
	failedProcessEvent   = "Failed to process event: %v"
	expectedUsername     = "test_user"
	expectedMigrated     = "migrated_user"
	expectedUsernameMsg  = "Expected username %s, got %s"
)

func TestNewEventHandler(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

	handler := NewEventHandler(logger)
	if handler == nil {
		t.Fatal("Expected non-nil event handler")
	}

	if handler.logger != logger {
		t.Error("Expected logger to be set correctly")
	}
}

func TestProcessEventPostConfirmation(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	handler := NewEventHandler(logger)

	event := structs.EventPayload{
		EventType: "PostConfirmation_ConfirmSignUp",
		UserID:    "123456",
		Username:  "test_user",
		Groups:    []string{"Users", "Developers"},
		Metadata:  map[string]interface{}{"email": "test@example.com"},
		Timestamp: time.Now(),
	}

	eventData, err := json.Marshal(event)
	if err != nil {
		t.Fatalf(failedMarshalEvent, err)
	}

	userConfig, err := handler.ProcessEvent(eventData)
	if err != nil {
		t.Fatalf(failedProcessEvent, err)
	}

	if userConfig.Username != expectedUsername {
		t.Errorf(expectedUsernameMsg, expectedUsername, userConfig.Username)
	}

	if !userConfig.Enabled {
		t.Error("Expected user to be enabled")
	}

	if len(userConfig.Groups) != 2 {
		t.Errorf("Expected 2 groups, got %d", len(userConfig.Groups))
	}

	if userConfig.Description == "" {
		t.Error("Expected description to be set")
	}
}

func TestProcessEventGroupMembership(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	handler := NewEventHandler(logger)

	tests := []struct {
		name      string
		eventType string
	}{
		{
			name:      "group added",
			eventType: "GroupMembership_GroupAdded",
		},
		{
			name:      "group removed",
			eventType: "GroupMembership_GroupRemoved",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := structs.EventPayload{
				EventType: tt.eventType,
				UserID:    "123456",
				Username:  "test_user",
				Groups:    []string{"NewGroup"},
				Timestamp: time.Now(),
			}

			eventData, err := json.Marshal(event)
			if err != nil {
				t.Fatalf(failedMarshalEvent, err)
			}

			userConfig, err := handler.ProcessEvent(eventData)
			if err != nil {
				t.Fatalf(failedProcessEvent, err)
			}

			if userConfig.Username != expectedUsername {
				t.Errorf(expectedUsernameMsg, expectedUsername, userConfig.Username)
			}
		})
	}
}

func TestProcessEventUserMigration(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	handler := NewEventHandler(logger)

	event := structs.EventPayload{
		EventType: "UserMigration_Authentication",
		UserID:    "123456",
		Username:  "migrated_user",
		Groups:    []string{"MigratedUsers"},
		Timestamp: time.Now(),
	}

	eventData, err := json.Marshal(event)
	if err != nil {
		t.Fatalf(failedMarshalEvent, err)
	}

	userConfig, err := handler.ProcessEvent(eventData)
	if err != nil {
		t.Fatalf(failedProcessEvent, err)
	}

	if userConfig.Username != expectedMigrated {
		t.Errorf(expectedUsernameMsg, expectedMigrated, userConfig.Username)
	}
}

func TestProcessEventUnknownType(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	handler := NewEventHandler(logger)

	event := structs.EventPayload{
		EventType: "Unknown_Event_Type",
		UserID:    "123456",
		Username:  "test_user",
		Timestamp: time.Now(),
	}

	eventData, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal event: %v", err)
	}

	_, err = handler.ProcessEvent(eventData)
	if err == nil {
		t.Error("Expected error for unknown event type")
	}

	expectedErrMsg := "unknown event type: Unknown_Event_Type"
	if err.Error() != expectedErrMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedErrMsg, err.Error())
	}
}

func TestProcessEventInvalidJSON(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	handler := NewEventHandler(logger)

	invalidJSON := []byte(`{"invalid": json}`)

	_, err := handler.ProcessEvent(invalidJSON)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestMapCognitoGroupsToRoles(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	handler := NewEventHandler(logger)

	tests := []struct {
		name           string
		inputGroups    []string
		expectedRoles  []string
	}{
		{
			name:          "known mappings",
			inputGroups:   []string{"Admins", "Users", "ReadOnly"},
			expectedRoles: []string{"admin_group", "app_group", "read_only"},
		},
		{
			name:          "mixed known and unknown",
			inputGroups:   []string{"Admins", "CustomGroup"},
			expectedRoles: []string{"admin_group", "CustomGroup"},
		},
		{
			name:          "all unknown",
			inputGroups:   []string{"CustomGroup1", "CustomGroup2"},
			expectedRoles: []string{"CustomGroup1", "CustomGroup2"},
		},
		{
			name:          "empty groups",
			inputGroups:   []string{},
			expectedRoles: []string{},
		},
		{
			name:          "developers group",
			inputGroups:   []string{"Developers"},
			expectedRoles: []string{"dev_group"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roles := handler.MapCognitoGroupsToRoles(tt.inputGroups)

			if len(roles) != len(tt.expectedRoles) {
				t.Errorf("Expected %d roles, got %d", len(tt.expectedRoles), len(roles))
				return
			}

			for i, expectedRole := range tt.expectedRoles {
				if i >= len(roles) || roles[i] != expectedRole {
					t.Errorf("Expected role '%s' at index %d, got '%s'", expectedRole, i, roles[i])
				}
			}
		})
	}
}

func TestSanitizeUsername(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	handler := NewEventHandler(logger)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple username",
			input:    "testuser",
			expected: "testuser",
		},
		{
			name:     "username with underscore",
			input:    "test_user",
			expected: "test_user",
		},
		{
			name:     "username with numbers",
			input:    "user123",
			expected: "user123",
		},
		{
			name:     "empty username",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.SanitizeUsername(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestValidateEvent(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	handler := NewEventHandler(logger)

	validEvent := structs.EventPayload{
		EventType: "PostConfirmation_ConfirmSignUp",
		UserID:    "123456",
		Username:  expectedUsername,
	}

	// Test valid event
	err := handler.ValidateEvent(&validEvent)
	if err != nil {
		t.Errorf("Expected no error for valid event, got: %v", err)
	}

	// Test missing event type
	invalidEvent := validEvent
	invalidEvent.EventType = ""
	err = handler.ValidateEvent(&invalidEvent)
	if err == nil || err.Error() != "event type is required" {
		t.Errorf("Expected 'event type is required' error, got: %v", err)
	}

	// Test missing username
	invalidEvent = validEvent
	invalidEvent.Username = ""
	err = handler.ValidateEvent(&invalidEvent)
	if err == nil || err.Error() != "username is required" {
		t.Errorf("Expected 'username is required' error, got: %v", err)
	}

	// Test missing user ID
	invalidEvent = validEvent
	invalidEvent.UserID = ""
	err = handler.ValidateEvent(&invalidEvent)
	if err == nil || err.Error() != "user ID is required" {
		t.Errorf("Expected 'user ID is required' error, got: %v", err)
	}
}
