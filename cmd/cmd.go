package cmd

import (
	"errors"
	"fmt"
	"io"

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

type userManager interface {
	Close() error
	CreateUser(user *structs.UserConfig) error
	AddUserToGroup(username, groupName string) error
	GrantPrivileges(target string, privileges []string, databases []string) error
	DropUser(username string) error
	SyncConfiguration(cfg *structs.Config) (*structs.SyncResult, error)
}

type cliDeps struct {
	isTerminal func(io.Reader) bool
	newManager func(conn *structs.DatabaseConnection, logger *logrus.Logger, dryRun bool) (userManager, error)
	logOut     io.Writer
}

func defaultCLIDeps() cliDeps {
	return cliDeps{
		isTerminal: isInteractiveTerminal,
		newManager: func(conn *structs.DatabaseConnection, logger *logrus.Logger, dryRun bool) (userManager, error) {
			return database.NewManager(conn, logger, dryRun)
		},
	}
}

type cliApp struct {
	configPath string
	dryRun     bool
	verbose    bool
	logger     *logrus.Logger
	deps       cliDeps
}

func newRootCommand(deps cliDeps) *cobra.Command {
	if deps.isTerminal == nil {
		deps.isTerminal = isInteractiveTerminal
	}
	if deps.newManager == nil {
		deps.newManager = defaultCLIDeps().newManager
	}

	app := &cliApp{
		deps: deps,
	}

	rootCmd := &cobra.Command{
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
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			app.initLogger(cmd)
		},
	}

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&app.configPath, "config", "c", "./config.json", "path to configuration file")
	rootCmd.PersistentFlags().BoolVar(&app.dryRun, "dry-run", false, "show what would be done without executing")
	rootCmd.PersistentFlags().BoolVarP(&app.verbose, "verbose", "v", false, "verbose output")

	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Synchronize database state with configuration",
		Long:  `Synchronize the PostgreSQL database state with the configuration file. This will create users, groups, and grant privileges as defined in the configuration.`,
		RunE:  app.runSync,
	}

	createUserCmd := &cobra.Command{
		Use:   "create-user [username]",
		Short: "Create a single user",
		Args:  cobra.ExactArgs(1),
		RunE:  app.runCreateUser,
	}

	// User creation flags
	createUserCmd.Flags().Bool("password-stdin", false, "read one password from standard input (cannot be used with --auth-method iam)")
	createUserCmd.Flags().StringSliceP("groups", "g", []string{}, "groups to add user to")
	createUserCmd.Flags().StringSlice("privileges", []string{}, "privileges to grant")
	createUserCmd.Flags().StringSlice("databases", []string{}, "databases to grant privileges on")
	createUserCmd.Flags().String("auth-method", "password", "authentication method: 'password' or 'iam'")
	createUserCmd.Flags().String("iam-role", "", "IAM role ARN for IAM authentication")
	createUserCmd.Flags().Bool("can-login", true, "whether user can login")
	createUserCmd.Flags().Int("connection-limit", 0, "maximum connections (0 = unlimited)")
	createUserCmd.Flags().String("description", "", "user description")

	dropUserCmd := &cobra.Command{
		Use:   "drop-user [username]",
		Short: "Drop a single user",
		Args:  cobra.ExactArgs(1),
		RunE:  app.runDropUser,
	}

	listUsersCmd := &cobra.Command{
		Use:   "list-users",
		Short: "List all database users",
		RunE:  app.runListUsers,
	}

	validateCmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration file",
		RunE:  app.runValidate,
	}

	// Add subcommands
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(createUserCmd)
	rootCmd.AddCommand(dropUserCmd)
	rootCmd.AddCommand(listUsersCmd)
	rootCmd.AddCommand(validateCmd)

	return rootCmd
}

// initLogger initializes the logger for a command invocation.
func (a *cliApp) initLogger(cmd *cobra.Command) {
	a.logger = logrus.New()
	a.logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	if a.deps.logOut != nil {
		a.logger.SetOutput(a.deps.logOut)
	} else {
		a.logger.SetOutput(cmd.ErrOrStderr())
	}

	if a.verbose {
		a.logger.SetLevel(logrus.DebugLevel)
	} else {
		a.logger.SetLevel(logrus.InfoLevel)
	}
}

// Execute executes the root command.
func Execute() error {
	return newRootCommand(defaultCLIDeps()).Execute()
}

// runSync handles the sync command.
func (a *cliApp) runSync(cmd *cobra.Command, args []string) error {
	a.logger.Info("Starting sync operation")

	// Load configuration
	configManager := config.NewManager(a.logger)
	cfg, err := configManager.LoadConfig(a.configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Get database connection
	dbConn, err := configManager.GetDatabaseConnection()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	// Initialize database manager
	dbManager, err := a.deps.newManager(dbConn, a.logger, a.dryRun)
	if err != nil {
		return fmt.Errorf("failed to initialize database manager: %w", err)
	}
	defer func() {
		if closeErr := dbManager.Close(); closeErr != nil {
			a.logger.WithError(closeErr).Warn("Failed to close database manager")
		}
	}()

	// Sync configuration
	result, err := dbManager.SyncConfiguration(cfg)
	if err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	// Report results
	a.logger.WithFields(logrus.Fields{
		"users_created":  len(result.UsersCreated),
		"users_modified": len(result.UsersModified),
		"users_removed":  len(result.UsersRemoved),
		"groups_created": len(result.GroupsCreated),
		"errors":         len(result.Errors),
	}).Info("Sync completed")

	// Report errors
	for _, err := range result.Errors {
		a.logger.Error(err)
	}

	if len(result.Errors) > 0 {
		return fmt.Errorf("sync completed with %d errors", len(result.Errors))
	}

	return nil
}

// runCreateUser handles the create-user command.
func (a *cliApp) runCreateUser(cmd *cobra.Command, args []string) error {
	username := args[0]
	passwordStdin, _ := cmd.Flags().GetBool("password-stdin")
	groups, _ := cmd.Flags().GetStringSlice("groups")
	privileges, _ := cmd.Flags().GetStringSlice("privileges")
	databases, _ := cmd.Flags().GetStringSlice("databases")
	authMethod, _ := cmd.Flags().GetString("auth-method")
	iamRole, _ := cmd.Flags().GetString("iam-role")
	canLogin, _ := cmd.Flags().GetBool("can-login")
	connectionLimit, _ := cmd.Flags().GetInt("connection-limit")
	description, _ := cmd.Flags().GetString("description")

	a.logger.WithFields(logrus.Fields{
		"username":    username,
		"auth_method": authMethod,
	}).Info("Creating user")

	// 1. Validate authentication method
	if authMethod != "password" && authMethod != "iam" {
		return fmt.Errorf("invalid auth-method: %s (must be 'password' or 'iam')", authMethod)
	}

	// 2. Validate IAM-specific requirements before reading standard input
	if authMethod == "iam" {
		if passwordStdin {
			return errors.New("--password-stdin cannot be used with --auth-method iam")
		}
	} else if iamRole != "" {
		a.logger.Warn("IAM role specified for password authentication user - IAM role will be ignored")
	}

	// 3. Preflight: validate privileges before reading standard input or connecting to the database
	preflight := &structs.UserConfig{Username: username, Privileges: privileges, Databases: databases}
	if err := config.ValidateUser(preflight); err != nil {
		return fmt.Errorf("invalid privileges: %w", err)
	}

	// 4. Read password from standard input if requested
	var password string
	if passwordStdin {
		var err error
		password, err = readPasswordFromStdin(cmd.InOrStdin(), a.deps.isTerminal)
		if err != nil {
			return err
		}
	}

	// Get database connection
	configManager := config.NewManager(a.logger)
	dbConn, err := configManager.GetDatabaseConnection()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	// Initialize database manager
	dbManager, err := a.deps.newManager(dbConn, a.logger, a.dryRun)
	if err != nil {
		return fmt.Errorf("failed to initialize database manager: %w", err)
	}
	defer func() {
		if closeErr := dbManager.Close(); closeErr != nil {
			a.logger.WithError(closeErr).Warn("Failed to close database manager")
		}
	}()

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
			a.logger.WithError(err).Warnf("Failed to add user to group %s", group)
		}
	}

	if len(privileges) > 0 && len(databases) > 0 {
		if err := dbManager.GrantPrivileges(username, privileges, databases); err != nil {
			a.logger.WithError(err).Warn("Failed to grant privileges")
		}
	}

	a.logger.WithFields(logrus.Fields{
		"username":    username,
		"auth_method": authMethod,
	}).Info("User created successfully")
	return nil
}

// runDropUser handles the drop-user command.
func (a *cliApp) runDropUser(cmd *cobra.Command, args []string) error {
	username := args[0]

	a.logger.WithField("username", username).Info("Dropping user")

	// Get database connection
	configManager := config.NewManager(a.logger)
	dbConn, err := configManager.GetDatabaseConnection()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	// Initialize database manager
	dbManager, err := a.deps.newManager(dbConn, a.logger, a.dryRun)
	if err != nil {
		return fmt.Errorf("failed to initialize database manager: %w", err)
	}
	defer func() {
		if closeErr := dbManager.Close(); closeErr != nil {
			a.logger.WithError(closeErr).Warn("Failed to close database manager")
		}
	}()

	// Drop user
	if err := dbManager.DropUser(username); err != nil {
		return fmt.Errorf("failed to drop user: %w", err)
	}

	a.logger.WithField("username", username).Info("User dropped successfully")
	return nil
}

// runListUsers handles the list-users command.
func (a *cliApp) runListUsers(cmd *cobra.Command, args []string) error {
	a.logger.Info("Listing users")

	// Get database connection
	configManager := config.NewManager(a.logger)
	dbConn, err := configManager.GetDatabaseConnection()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	// Initialize database manager
	dbManager, err := a.deps.newManager(dbConn, a.logger, a.dryRun)
	if err != nil {
		return fmt.Errorf("failed to initialize database manager: %w", err)
	}
	defer func() {
		if closeErr := dbManager.Close(); closeErr != nil {
			a.logger.WithError(closeErr).Warn("Failed to close database manager")
		}
	}()

	// This would require implementing a ListUsers method in the database manager
	// For now, we'll just indicate that this is a placeholder
	fmt.Println("User listing functionality to be implemented")

	return nil
}

// runValidate handles the validate command.
func (a *cliApp) runValidate(cmd *cobra.Command, args []string) error {
	a.logger.WithField("config", a.configPath).Info("Validating configuration")

	// Load configuration
	configManager := config.NewManager(a.logger)
	_, err := configManager.LoadConfig(a.configPath)
	if err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	a.logger.Info("Configuration is valid")
	return nil
}
