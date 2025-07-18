package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/structs"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// Manager handles configuration loading and environment variables
type Manager struct {
	logger *logrus.Logger
}

// NewManager creates a new configuration manager
func NewManager(logger *logrus.Logger) *Manager {
	return &Manager{
		logger: logger,
	}
}

// LoadConfig reads the configuration file and returns a Config struct
func (m *Manager) LoadConfig(configPath string) (*structs.Config, error) {
	m.logger.WithField("path", configPath).Info("Loading configuration file")

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file not found: %s", configPath)
	}

	// Read the file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}

	// Parse JSON
	var config structs.Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse configuration file: %w", err)
	}

	m.logger.WithFields(logrus.Fields{
		"users":  len(config.Users),
		"groups": len(config.Groups),
	}).Info("Configuration loaded successfully")

	return &config, nil
}

// GetDatabaseConnection reads database connection details from environment variables
func (m *Manager) GetDatabaseConnection() (*structs.DatabaseConnection, error) {
	m.logger.Info("Reading database connection from environment variables")

	conn := &structs.DatabaseConnection{
		Host:      getEnvOrDefault("POSTGRES_HOST", "localhost"),
		Database:  getEnvOrDefault("POSTGRES_DB", "postgres"),
		Username:  getEnvOrDefault("POSTGRES_USER", "postgres"),
		Password:  os.Getenv("POSTGRES_PASSWORD"),
		SSLMode:   getEnvOrDefault("POSTGRES_SSLMODE", "require"), // Default to require for RDS
		IAMAuth:   getEnvOrDefault("POSTGRES_IAM_AUTH", "false") == "true",
		AWSRegion: getEnvOrDefault("AWS_REGION", "us-east-1"),
	}

	// Parse port
	portStr := getEnvOrDefault("POSTGRES_PORT", "5432")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid POSTGRES_PORT: %s", portStr)
	}
	conn.Port = port

	// Validate required fields based on authentication method
	if conn.IAMAuth {
		m.logger.Info("Using IAM authentication for database connection")
		
		// For IAM auth, we need AWS region and proper SSL
		if conn.AWSRegion == "" {
			return nil, fmt.Errorf("AWS_REGION environment variable is required for IAM authentication")
		}
		
		// Force SSL for IAM authentication
		if conn.SSLMode == "disable" {
			m.logger.Warn("Forcing SSL mode to 'require' for IAM authentication")
			conn.SSLMode = "require"
		}
		
		// IAM token can be provided or will be generated
		conn.IAMToken = os.Getenv("POSTGRES_IAM_TOKEN")
		
	} else {
		m.logger.Info("Using password authentication for database connection")
		
		// For password auth, password is required
		if conn.Password == "" {
			return nil, fmt.Errorf("POSTGRES_PASSWORD environment variable is required for password authentication")
		}
	}

	m.logger.WithFields(logrus.Fields{
		"host":      conn.Host,
		"port":      conn.Port,
		"database":  conn.Database,
		"username":  conn.Username,
		"sslmode":   conn.SSLMode,
		"iam_auth":  conn.IAMAuth,
		"aws_region": conn.AWSRegion,
	}).Info("Database connection configuration loaded")

	return conn, nil
}

// SaveConfig saves the configuration to a file
func (m *Manager) SaveConfig(config *structs.Config, configPath string) error {
	m.logger.WithField("path", configPath).Info("Saving configuration file")

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write configuration file: %w", err)
	}

	m.logger.Info("Configuration saved successfully")
	return nil
}

// InitializeViper sets up viper for configuration management
func (m *Manager) InitializeViper() {
	viper.SetEnvPrefix("PUM") // PostgreSQL User Manager
	viper.AutomaticEnv()
	
	// Set default values
	viper.SetDefault("config.path", "./config.json")
	viper.SetDefault("log.level", "info")
	viper.SetDefault("dry.run", false)
}

// getEnvOrDefault returns environment variable value or default if not set
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}