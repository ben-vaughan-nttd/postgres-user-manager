package config

import (
	"os"
	"testing"

	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/structs"
	"github.com/sirupsen/logrus"
)

const (
	failedCreateTempFile = "Failed to create temp file: %v"
)

func TestLoadConfig(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

	manager := NewManager(logger)

	// Create a temporary config file
	configContent := `{
		"users": [
			{
				"username": "test_user",
				"password": "test_pass",
				"groups": ["test_group"],
				"privileges": ["CONNECT"],
				"databases": ["test_db"],
				"enabled": true,
				"description": "Test user"
			}
		],
		"groups": [
			{
				"name": "test_group",
				"privileges": ["CONNECT"],
				"databases": ["test_db"],
				"description": "Test group",
				"inherit": true
			}
		]
	}`

	tmpFile, err := os.CreateTemp("", "test_config_*.json")
	if err != nil {
		t.Fatalf(failedCreateTempFile, err)
	}
	defer func() {
		if removeErr := os.Remove(tmpFile.Name()); removeErr != nil {
			t.Fatalf("Failed to remove temp file: %v", removeErr)
		}
	}()

	if _, err := tmpFile.Write([]byte(configContent)); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Test loading the config
	config, err := manager.LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify the loaded configuration
	if len(config.Users) != 1 {
		t.Errorf("Expected 1 user, got %d", len(config.Users))
	}

	if len(config.Groups) != 1 {
		t.Errorf("Expected 1 group, got %d", len(config.Groups))
	}

	user := config.Users[0]
	if user.Username != "test_user" {
		t.Errorf("Expected username 'test_user', got '%s'", user.Username)
	}

	if !user.Enabled {
		t.Error("Expected user to be enabled")
	}

	group := config.Groups[0]
	if group.Name != "test_group" {
		t.Errorf("Expected group name 'test_group', got '%s'", group.Name)
	}
}

func TestGetDatabaseConnection(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	manager := NewManager(logger)

	// Test missing password
	_, err := manager.GetDatabaseConnection()
	if err == nil {
		t.Error("Expected error for missing POSTGRES_PASSWORD")
	}

	// Set required environment variable
	t.Setenv("POSTGRES_PASSWORD", "test_password")

	conn, err := manager.GetDatabaseConnection()
	if err != nil {
		t.Fatalf("Failed to get database connection: %v", err)
	}

	if conn.Host != "localhost" {
		t.Errorf("Expected host 'localhost', got '%s'", conn.Host)
	}

	if conn.Port != 5432 {
		t.Errorf("Expected port 5432, got %d", conn.Port)
	}

	if conn.Password != "test_password" {
		t.Errorf("Expected password 'test_password', got '%s'", conn.Password)
	}
}

func TestSaveConfig(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	manager := NewManager(logger)

	// Create a test configuration
	config := &structs.Config{
		Users: []structs.UserConfig{
			{
				Username: "save_test_user",
				Enabled:  true,
			},
		},
		Groups: []structs.GroupConfig{
			{
				Name:    "save_test_group",
				Inherit: true,
			},
		},
	}

	tmpFile, err := os.CreateTemp("", "test_save_config_*.json")
	if err != nil {
		t.Fatalf(failedCreateTempFile, err)
	}
	defer func() {
		if removeErr := os.Remove(tmpFile.Name()); removeErr != nil {
			t.Fatalf("Failed to remove temp file: %v", removeErr)
		}
	}()
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Save the configuration
	err = manager.SaveConfig(config, tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Load it back and verify
	loadedConfig, err := manager.LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load saved config: %v", err)
	}

	if len(loadedConfig.Users) != 1 {
		t.Errorf("Expected 1 user in saved config, got %d", len(loadedConfig.Users))
	}

	if loadedConfig.Users[0].Username != "save_test_user" {
		t.Errorf("Expected username 'save_test_user', got '%s'", loadedConfig.Users[0].Username)
	}
}

func TestGetDatabaseConnectionWithIAM(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	
	manager := NewManager(logger)
	
	// Set environment variables for IAM authentication
	t.Setenv("POSTGRES_IAM_AUTH", "true")
	t.Setenv("AWS_REGION", "us-west-2")
	t.Setenv("POSTGRES_USER", "iam_user")
	t.Setenv("POSTGRES_HOST", "test.cluster-xxx.us-west-2.rds.amazonaws.com")
	
	conn, err := manager.GetDatabaseConnection()
	if err != nil {
		t.Fatalf("Failed to get IAM database connection: %v", err)
	}
	
	if !conn.IAMAuth {
		t.Error("Expected IAMAuth to be true")
	}
	
	if conn.AWSRegion != "us-west-2" {
		t.Errorf("Expected AWS region 'us-west-2', got '%s'", conn.AWSRegion)
	}
	
	if conn.Username != "iam_user" {
		t.Errorf("Expected username 'iam_user', got '%s'", conn.Username)
	}
	
	if conn.SSLMode != "require" {
		t.Errorf("Expected SSL mode 'require' for IAM, got '%s'", conn.SSLMode)
	}
}

func TestEnvironmentVariableHandling(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	manager := NewManager(logger)

	// Test with custom environment variables
	t.Setenv("POSTGRES_HOST", "custom-host")
	t.Setenv("POSTGRES_PORT", "5433")
	t.Setenv("POSTGRES_DB", "custom_db")
	t.Setenv("POSTGRES_USER", "custom_user")
	t.Setenv("POSTGRES_PASSWORD", "custom_pass")

	conn, err := manager.GetDatabaseConnection()
	if err != nil {
		t.Fatalf("Failed to get database connection: %v", err)
	}

	if conn.Host != "custom-host" {
		t.Errorf("Expected host 'custom-host', got '%s'", conn.Host)
	}

	if conn.Port != 5433 {
		t.Errorf("Expected port 5433, got %d", conn.Port)
	}

	if conn.Database != "custom_db" {
		t.Errorf("Expected database 'custom_db', got '%s'", conn.Database)
	}

	if conn.Username != "custom_user" {
		t.Errorf("Expected username 'custom_user', got '%s'", conn.Username)
	}
}

func TestLoadConfigWithInvalidFile(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	manager := NewManager(logger)

	// Test with non-existent file
	_, err := manager.LoadConfig("non_existent_file.json")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}

	// Test with invalid JSON
	tmpFile, err := os.CreateTemp("", "invalid_config_*.json")
	if err != nil {
		t.Fatalf(failedCreateTempFile, err)
	}
	defer func() {
		if removeErr := os.Remove(tmpFile.Name()); removeErr != nil {
			t.Fatalf("Failed to remove temp file: %v", removeErr)
		}
	}()

	if _, err := tmpFile.Write([]byte("invalid json content")); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	_, err = manager.LoadConfig(tmpFile.Name())
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestSaveConfigWithInvalidPath(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	manager := NewManager(logger)

	config := &structs.Config{
		Users: []structs.UserConfig{
			{
				Username: "test_user",
				Enabled:  true,
			},
		},
	}

	// Test with invalid path (directory that doesn't exist)
	err := manager.SaveConfig(config, "/non/existent/path/config.json")
	if err == nil {
		t.Error("Expected error for invalid file path")
	}
}

func TestConfigPortParsing(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	manager := NewManager(logger)

	// Test with invalid port
	t.Setenv("POSTGRES_PORT", "invalid_port")
	t.Setenv("POSTGRES_PASSWORD", "test_pass")

	_, err := manager.GetDatabaseConnection()
	if err == nil {
		t.Error("Expected error for invalid port")
	}
}

func TestNewManager(t *testing.T) {
	logger := logrus.New()

	manager := NewManager(logger)
	if manager == nil {
		t.Error("Expected non-nil manager")
	}

	if manager.logger != logger {
		t.Error("Expected logger to be set correctly")
	}
}

func TestInitializeViper(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	
	manager := NewManager(logger)
	
	// Test InitializeViper function
	manager.InitializeViper()
	
	// This function mainly sets up viper configuration
	// We can't easily test the internal state without coupling to viper internals
	// But we can ensure it doesn't panic and runs successfully
}
