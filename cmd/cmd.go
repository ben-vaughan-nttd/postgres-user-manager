package cmd

import (
	"fmt"

	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/config"
	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/database"
	"github.com/ben-vaughan-nttd/postgres-user-manager/internal/structs"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const (
	appName        = "postgres-user-manager"
	appDescription = "A tool for managing PostgreSQL users and privileges"
)

var (
	configPath string
	dryRun     bool
	verbose    bool
	logger     *logrus.Logger
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   appName,
	Short: appDescription,
	Long: `PostgreSQL User Manager is a CLI tool for managing PostgreSQL users, groups, and privileges.
	
It provides idempotent operations for creating, modifying, and removing users and groups
based on a JSON configuration file. Supports both traditional password authentication 
and AWS RDS IAM database authentication.

Environment Variables:
  POSTGRES_HOST         - Database host (default: localhost)
  POSTGRES_PORT         - Database port (default: 5432)
  POSTGRES_DB           - Database name (default: postgres)
  POSTGRES_USER         - Database username (default: postgres)
  POSTGRES_SSLMODE      - SSL mode (default: require for IAM, prefer for password)
  
Authentication Options:
  Password Authentication:
    POSTGRES_PASSWORD   - Database password (required)
    POSTGRES_IAM_AUTH   - Set to false (default)
  
  IAM Authentication (AWS RDS Aurora):
    POSTGRES_IAM_AUTH   - Set to true
    POSTGRES_IAM_TOKEN  - IAM auth token (optional, can be auto-generated)
    AWS_REGION          - AWS region (required for IAM auth)
    AWS_ACCESS_KEY_ID   - AWS credentials (if not using instance profile)
    AWS_SECRET_ACCESS_KEY - AWS credentials (if not using instance profile)`,
}

// syncCmd represents the sync command
var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronize database state with configuration",
	Long:  `Synchronize the PostgreSQL database state with the configuration file. This will create users, groups, and grant privileges as defined in the configuration.`,
	RunE:  runSync,
}

// createUserCmd represents the create-user command
var createUserCmd = &cobra.Command{
	Use:   "create-user [username]",
	Short: "Create a single user",
	Args:  cobra.ExactArgs(1),
	RunE:  runCreateUser,
}

// dropUserCmd represents the drop-user command
var dropUserCmd = &cobra.Command{
	Use:   "drop-user [username]",
	Short: "Drop a single user",
	Args:  cobra.ExactArgs(1),
	RunE:  runDropUser,
}

// listUsersCmd represents the list-users command
var listUsersCmd = &cobra.Command{
	Use:   "list-users",
	Short: "List all database users",
	RunE:  runListUsers,
}

// validateCmd represents the validate command
var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration file",
	RunE:  runValidate,
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "./config.json", "path to configuration file")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "show what would be done without executing")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	// Add subcommands
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(createUserCmd)
	rootCmd.AddCommand(dropUserCmd)
	rootCmd.AddCommand(listUsersCmd)
	rootCmd.AddCommand(validateCmd)

	// User creation flags
	createUserCmd.Flags().StringP("password", "p", "", "user password (not used for IAM auth)")
	createUserCmd.Flags().StringSliceP("groups", "g", []string{}, "groups to add user to")
	createUserCmd.Flags().StringSlice("privileges", []string{}, "privileges to grant")
	createUserCmd.Flags().StringSlice("databases", []string{}, "databases to grant privileges on")
	createUserCmd.Flags().String("auth-method", "password", "authentication method: 'password' or 'iam'")
	createUserCmd.Flags().String("iam-role", "", "IAM role ARN for IAM authentication")
	createUserCmd.Flags().Bool("can-login", true, "whether user can login")
	createUserCmd.Flags().Int("connection-limit", 0, "maximum connections (0 = unlimited)")
	createUserCmd.Flags().String("description", "", "user description")
}

// initConfig initializes the logger and configuration
func initConfig() {
	logger = logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	if verbose {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		logger.SetLevel(logrus.InfoLevel)
	}
}

// Execute executes the root command
func Execute() error {
	return rootCmd.Execute()
}

// runSync handles the sync command
func runSync(cmd *cobra.Command, args []string) error {
	logger.Info("Starting sync operation")

	// Load configuration
	configManager := config.NewManager(logger)
	cfg, err := configManager.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Get database connection
	dbConn, err := configManager.GetDatabaseConnection()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	// Initialize database manager
	dbManager, err := database.NewManager(dbConn, logger, dryRun)
	if err != nil {
		return fmt.Errorf("failed to initialize database manager: %w", err)
	}
	defer dbManager.Close()

	// Sync configuration
	result, err := dbManager.SyncConfiguration(cfg)
	if err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	// Report results
	logger.WithFields(logrus.Fields{
		"users_created":  len(result.UsersCreated),
		"users_modified": len(result.UsersModified),
		"users_removed":  len(result.UsersRemoved),
		"groups_created": len(result.GroupsCreated),
		"errors":         len(result.Errors),
	}).Info("Sync completed")

	// Report errors
	for _, err := range result.Errors {
		logger.Error(err)
	}

	if len(result.Errors) > 0 {
		return fmt.Errorf("sync completed with %d errors", len(result.Errors))
	}

	return nil
}

// runCreateUser handles the create-user command
func runCreateUser(cmd *cobra.Command, args []string) error {
	username := args[0]
	password, _ := cmd.Flags().GetString("password")
	groups, _ := cmd.Flags().GetStringSlice("groups")
	privileges, _ := cmd.Flags().GetStringSlice("privileges")
	databases, _ := cmd.Flags().GetStringSlice("databases")
	authMethod, _ := cmd.Flags().GetString("auth-method")
	iamRole, _ := cmd.Flags().GetString("iam-role")
	canLogin, _ := cmd.Flags().GetBool("can-login")
	connectionLimit, _ := cmd.Flags().GetInt("connection-limit")
	description, _ := cmd.Flags().GetString("description")

	logger.WithFields(logrus.Fields{
		"username":    username,
		"auth_method": authMethod,
	}).Info("Creating user")

	// Validate authentication method
	if authMethod != "password" && authMethod != "iam" {
		return fmt.Errorf("invalid auth-method: %s (must be 'password' or 'iam')", authMethod)
	}

	// Validate IAM-specific requirements
	if authMethod == "iam" {
		if password != "" {
			logger.Warn("Password specified for IAM authentication user - password will be ignored")
		}
	} else {
		if iamRole != "" {
			logger.Warn("IAM role specified for password authentication user - IAM role will be ignored")
		}
	}

	// Get database connection
	configManager := config.NewManager(logger)
	dbConn, err := configManager.GetDatabaseConnection()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	// Initialize database manager
	dbManager, err := database.NewManager(dbConn, logger, dryRun)
	if err != nil {
		return fmt.Errorf("failed to initialize database manager: %w", err)
	}
	defer dbManager.Close()

	// Create user configuration
	userConfig := &structs.UserConfig{
		Username:        username,
		Password:        password,
		Groups:          groups,
		Privileges:      privileges,
		Databases:       databases,
		Enabled:         true,
		Description:     description,
		AuthMethod:      authMethod,
		IAMRole:         iamRole,
		CanLogin:        canLogin,
		ConnectionLimit: connectionLimit,
	}

	// Create user
	if err := dbManager.CreateUser(userConfig); err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	// Add to groups and grant privileges
	for _, group := range groups {
		if err := dbManager.AddUserToGroup(username, group); err != nil {
			logger.WithError(err).Warnf("Failed to add user to group %s", group)
		}
	}

	if len(privileges) > 0 && len(databases) > 0 {
		if err := dbManager.GrantPrivileges(username, privileges, databases); err != nil {
			logger.WithError(err).Warn("Failed to grant privileges")
		}
	}

	logger.WithFields(logrus.Fields{
		"username":    username,
		"auth_method": authMethod,
	}).Info("User created successfully")
	return nil
}

// runDropUser handles the drop-user command
func runDropUser(cmd *cobra.Command, args []string) error {
	username := args[0]

	logger.WithField("username", username).Info("Dropping user")

	// Get database connection
	configManager := config.NewManager(logger)
	dbConn, err := configManager.GetDatabaseConnection()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	// Initialize database manager
	dbManager, err := database.NewManager(dbConn, logger, dryRun)
	if err != nil {
		return fmt.Errorf("failed to initialize database manager: %w", err)
	}
	defer dbManager.Close()

	// Drop user
	if err := dbManager.DropUser(username); err != nil {
		return fmt.Errorf("failed to drop user: %w", err)
	}

	logger.WithField("username", username).Info("User dropped successfully")
	return nil
}

// runListUsers handles the list-users command
func runListUsers(cmd *cobra.Command, args []string) error {
	logger.Info("Listing users")

	// Get database connection
	configManager := config.NewManager(logger)
	dbConn, err := configManager.GetDatabaseConnection()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	// Initialize database manager
	dbManager, err := database.NewManager(dbConn, logger, dryRun)
	if err != nil {
		return fmt.Errorf("failed to initialize database manager: %w", err)
	}
	defer dbManager.Close()

	// This would require implementing a ListUsers method in the database manager
	// For now, we'll just indicate that this is a placeholder
	fmt.Println("User listing functionality to be implemented")
	
	return nil
}

// runValidate handles the validate command
func runValidate(cmd *cobra.Command, args []string) error {
	logger.WithField("config", configPath).Info("Validating configuration")

	// Load configuration
	configManager := config.NewManager(logger)
	_, err := configManager.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	logger.Info("Configuration is valid")
	return nil
}