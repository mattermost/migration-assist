package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/mattermost/migration-assist/internal/git"
	"github.com/mattermost/migration-assist/internal/logger"
	"github.com/mattermost/migration-assist/internal/pgloader"
	"github.com/mattermost/migration-assist/internal/store"
	"github.com/mattermost/migration-assist/queries"
	"github.com/mattermost/morph/sources"
	"github.com/mattermost/morph/sources/file"
	"github.com/spf13/cobra"
)

func TargetCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "postgres",
		Short: "Checks the Postgres database schema whether it is ready for the migration",
		RunE:  runTargetCheckCmdF,
		Example: "  migration-assist postgres \"postgres://mmuser:mostest@localhost:8765/mattermost_test?sslmode=disable\" \\\n--run-migrations" +
			"\n\n--mattermost-version, --migrations-dir, --applied-migrations are mutually exclusive. Use only one of these flags.\n",
		Args: cobra.MinimumNArgs(1),
	}

	amCmd := RunAfterMigration()
	cmd.AddCommand(amCmd)

	// Optional flags
	cmd.Flags().Bool("run-migrations", false, "Runs migrations for Postgres schema")
	cmd.Flags().String("mattermost-version", "", "Mattermost version to be cloned to run migrations (example: \"v8.1\")")
	cmd.Flags().String("migrations-dir", "", "Migrations directory (should be used if the migrations are already cloned separately)")
	cmd.Flags().String("applied-migrations", "", "File containing the list of applied migrations (example: \"mysql.output\")")
	cmd.Flags().String("git", "git", "git binary to be executed if the repository will be cloned (ie. --mattermost-version is supplied)")
	cmd.Flags().Bool("check-schema-owner", true, "Check if the schema owner is the same as the user running the migration")
	cmd.Flags().Bool("check-tables-empty", true, "Check if tables are empty before running migrations")
	cmd.PersistentFlags().String("schema", "public", "the default schema to be used for the session")

	return cmd
}

func RunAfterMigration() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "post-migrate",
		Short:   "Creates indexes after the migration is completed",
		RunE:    runPostMigrateCmdF,
		Example: "  migration-assist postgres post-migrate \"postgres://mmuser:mostest@localhost:8765/mattermost_test?sslmode=disable\"",
		Args:    cobra.MinimumNArgs(1),
	}

	cmd.Flags().Bool("create-indexes", false, "Creates Fulltext indexes after the migration is completed.")

	return cmd
}

func runTargetCheckCmdF(cmd *cobra.Command, args []string) error {
	baseLogger := logger.NewLogger(os.Stderr, logger.Options{Timestamps: true})
	var verboseLogger logger.LogInterface

	verbose, _ := cmd.Flags().GetBool("verbose")
	if verbose {
		verboseLogger = baseLogger
	} else {
		verboseLogger = logger.NewNopLogger()
	}

	postgresDB, err := store.NewStore("postgres", args[0])
	if err != nil {
		return err
	}
	defer postgresDB.Close()

	baseLogger.Println("pinging postgres...")
	err = postgresDB.Ping()
	if err != nil {
		return fmt.Errorf("could not ping postgres: %w", err)
	}
	baseLogger.Println("connected to postgres successfully.")

	var params pgloader.Parameters
	err = pgloader.ParsePostgres(&params, args[0])
	if err != nil {
		return fmt.Errorf("could not parse postgres connection string: %w", err)
	}

	checkSchema, _ := cmd.Flags().GetBool("check-schema-owner")
	if checkSchema {
		err = postgresDB.CheckPostgresSchemaOwnership(cmd.Context(), "public", params.PGUser)
		if err != nil {
			return fmt.Errorf("could not check schema owner: %w", err)
		}

		baseLogger.Println("schema owner check passed.")
	}

	// check if tables are empty
	checkTablesEmpty, _ := cmd.Flags().GetBool("check-tables-empty")
	if checkTablesEmpty {
		baseLogger.Println("checking if tables are empty...")
		tables, err2 := postgresDB.CheckIfPostgresTablesEmpty(cmd.Context())
		if err2 != nil {
			return fmt.Errorf("could not check if tables are empty: %w", err2)
		}
		for _, table := range tables {
			baseLogger.Printf("table %s is not empty\n", table)
		}
	}

	runMigrations, _ := cmd.Flags().GetBool("run-migrations")
	if !runMigrations {
		return nil
	}

	mysqlMigrations, _ := cmd.Flags().GetString("applied-migrations")
	mmVersion, _ := cmd.Flags().GetString("mattermost-version")
	migrationDir, _ := cmd.Flags().GetString("migrations-dir")

	src, err := determineSource(mysqlMigrations, migrationDir, mmVersion, baseLogger, verboseLogger)
	if err != nil {
		return fmt.Errorf("could not determine source: %w", err)
	}

	// run the migrations
	baseLogger.Println("running migrations..")
	err = postgresDB.RunMigrations(src)
	if err != nil {
		return fmt.Errorf("could not run migrations: %w", err)
	}

	baseLogger.Println("migrations applied.")

	return nil
}

func runPostMigrateCmdF(c *cobra.Command, args []string) error {
	baseLogger := logger.NewLogger(os.Stderr, logger.Options{Timestamps: true})
	schema, _ := c.Flags().GetString("schema")

	var params pgloader.Parameters
	err := pgloader.ParsePostgres(&params, args[0])
	if err != nil {
		return err
	}

	postgresDB, err := store.NewStore("postgres", args[0])
	if err != nil {
		return err
	}
	defer postgresDB.Close()

	err = postgresDB.CheckPostgresDefaultSchema(c.Context(), schema, baseLogger)
	if err != nil {
		return fmt.Errorf("could not check default schema: %w", err)
	}

	err = postgresDB.CheckPostgresSchemaOwnership(c.Context(), "public", params.PGUser)
	if err != nil {
		return fmt.Errorf("could not check default schema: %w", err)
	}

	baseLogger.Println("running migrations..")

	runMigration, _ := c.Flags().GetBool("create-indexes")
	if !runMigration {
		baseLogger.Println("index creation skipped.")
		return nil
	}

	err = postgresDB.RunEmbeddedMigrations(queries.Assets(), "post-migrate", baseLogger)
	if err != nil {
		if strings.Contains(err.Error(), "pq: string is too long for tsvector") {
			baseLogger.Println("Index creation failed due to content being too long for tsvector.\n" +
				"This is expected if you have a large amount of data.\n\n" +
				"Please run the migration manually and refer to the documentation page below:\n" +
				"https://docs.mattermost.com/deploy/manual-postgres-migration.html#restore-full-text-indexes")
			return nil
		}
		return fmt.Errorf("could not run migrations: %w", err)
	}

	baseLogger.Println("indexes created.")

	return nil
}

func determineSource(appliedMigrations, userSuppliedMigrations, mmVersion string, baseLogger, verboseLogger logger.LogInterface) (sources.Source, error) {
	switch {
	case appliedMigrations != "":
		baseLogger.Printf("loading migrations from the %s file\n", appliedMigrations)
		// load migrations from the applied migrations file
		var cfg store.DBConfig
		f, err2 := os.Open(appliedMigrations)
		if err2 != nil {
			return nil, fmt.Errorf("could not open file: %w", err2)
		}
		defer f.Close()

		err := json.NewDecoder(f).Decode(&cfg)
		if err != nil {
			return nil, fmt.Errorf("could not decode file: %w", err)
		}

		src, err := store.CreateSourceFromEmbedded(queries.Assets(), "migrations/postgres", cfg.AppliedMigrations)
		if err != nil {
			return nil, fmt.Errorf("could not create source from embedded: %w", err)
		}

		return src, nil
	case userSuppliedMigrations != "":
		src, err := file.Open(userSuppliedMigrations)
		if err != nil {
			return nil, fmt.Errorf("could not read migrations: %w", err)
		}

		return src, nil
	default:
		if mmVersion == "" {
			return nil, fmt.Errorf("--mattermost-version needs to be supplied to run migrations")
		}
		v, err2 := semver.ParseTolerant(mmVersion)
		if err2 != nil {
			return nil, fmt.Errorf("could not parse mattermost version: %w", err2)
		}

		tempDir, err3 := os.MkdirTemp("", "mattermost")
		if err3 != nil {
			return nil, fmt.Errorf("could not create temp directory: %w", err3)
		}

		baseLogger.Printf("cloning %s@%s\n", "repository", v.String())
		err := git.CloneMigrations(git.CloneOptions{
			TempRepoPath: tempDir,
			Output:       "postgres",
			DriverType:   "postgres",
			Version:      v,
		}, baseLogger)
		if err != nil {
			return nil, fmt.Errorf("error during cloning migrations: %w", err)
		}

		src, err := file.Open("postgres")
		if err != nil {
			return nil, fmt.Errorf("could not read migrations: %w", err)
		}

		return src, nil
	}
}
