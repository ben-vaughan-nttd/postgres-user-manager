package events

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/structs"
	"github.com/sirupsen/logrus"
)

// EventHandler handles AWS Cognito events for future integration
type EventHandler struct {
	logger *logrus.Logger
}

// NewEventHandler creates a new event handler
func NewEventHandler(logger *logrus.Logger) *EventHandler {
	return &EventHandler{
		logger: logger,
	}
}

// ProcessEvent processes an incoming event and returns corresponding user configuration
func (h *EventHandler) ProcessEvent(eventData []byte) (*structs.UserConfig, error) {
	h.logger.Debug("Processing incoming event")

	var event structs.EventPayload
	if err := json.Unmarshal(eventData, &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event: %w", err)
	}

	h.logger.WithFields(logrus.Fields{
		"event_type": event.EventType,
		"user_id":    event.UserID,
		"username":   event.Username,
	}).Info("Processing event")

	// Convert Cognito event to user configuration
	userConfig := &structs.UserConfig{
		Username:    event.Username,
		Groups:      event.Groups,
		Enabled:     true,
		Description: fmt.Sprintf("User created from Cognito event at %s", event.Timestamp.Format(time.RFC3339)),
	}

	// Handle different event types
	switch event.EventType {
	case "PostConfirmation_ConfirmSignUp":
		h.logger.Info("Handling user signup confirmation")
		// User has been confirmed, create PostgreSQL user
		
	case "GroupMembership_GroupAdded":
		h.logger.Info("Handling group membership addition")
		// User added to group, update PostgreSQL roles
		
	case "GroupMembership_GroupRemoved":
		h.logger.Info("Handling group membership removal")
		// User removed from group, update PostgreSQL roles
		
	case "UserMigration_Authentication":
		h.logger.Info("Handling user migration")
		// User migration event
		
	default:
		h.logger.WithField("event_type", event.EventType).Warn("Unknown event type")
		return nil, fmt.Errorf("unknown event type: %s", event.EventType)
	}

	return userConfig, nil
}

// MapCognitoGroupsToRoles maps Cognito groups to PostgreSQL roles
func (h *EventHandler) MapCognitoGroupsToRoles(groups []string) []string {
	// This function will be implemented to map Cognito groups to PostgreSQL roles
	// For now, it returns the groups as-is
	h.logger.WithField("groups", groups).Debug("Mapping Cognito groups to PostgreSQL roles")
	
	roleMapping := map[string]string{
		"Admins":     "admin_group",
		"Users":      "app_group",
		"ReadOnly":   "read_only",
		"Developers": "dev_group",
	}

	var roles []string
	for _, group := range groups {
		if role, exists := roleMapping[group]; exists {
			roles = append(roles, role)
		} else {
			// If no mapping exists, use the group name as-is (sanitized)
			roles = append(roles, group)
		}
	}

	return roles
}

// SanitizeUsername ensures the username is valid for PostgreSQL
func (h *EventHandler) SanitizeUsername(username string) string {
	// Implement username sanitization for PostgreSQL compatibility
	// For now, return as-is
	return username
}

// ValidateEvent validates that an event payload is properly formatted
func (h *EventHandler) ValidateEvent(event *structs.EventPayload) error {
	if event.EventType == "" {
		return fmt.Errorf("event type is required")
	}
	
	if event.Username == "" {
		return fmt.Errorf("username is required")
	}
	
	if event.UserID == "" {
		return fmt.Errorf("user ID is required")
	}
	
	return nil
}
