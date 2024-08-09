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
		Use:     "postgres",
		Short:   "Checks the Postgres database schema whether it is ready for the migration",
		RunE:    runTargetCheckCmdF,
		Example: "  migration-assist postgres \"postgres://mmuser:mostest@localhost:8765/mattermost_test?sslmode=disable\" \\\n--run-migrations",
		Args:    cobra.MinimumNArgs(1),
	}

	amCmd := RunAfterMigration()
	cmd.AddCommand(amCmd)

	// Optional flags
	cmd.Flags().Bool("run-migrations", false, "Runs migrations for Postgres schema")
	cmd.Flags().String("mattermost-version", "", "Mattermost version to be cloned to run migrations (example: v8.1)")
	cmd.Flags().String("migrations-dir", "", "Migrations directory (should be used if mattermost-version is not supplied)")
	cmd.Flags().String("git", "git", "git binary to be executed if the repository will be cloned")
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

	mysqlMigrations := "mysql.output"
	if len(args) > 1 {
		mysqlMigrations = args[1]
	}

	var src sources.Source

	// download required migrations if necessary
	migrationDir, _ := cmd.Flags().GetString("migrations-dir")
	if _, err = os.Stat(mysqlMigrations); !os.IsNotExist(err) {
		baseLogger.Printf("loading migrations from the %s file\n", mysqlMigrations)
		// load migrations from the applied migrations file
		var cfg store.DBConfig
		f, err2 := os.Open(mysqlMigrations)
		if err2 != nil {
			return fmt.Errorf("could not open file: %w", err2)
		}
		defer f.Close()

		err = json.NewDecoder(f).Decode(&cfg)
		if err != nil {
			return fmt.Errorf("could not decode file: %w", err)
		}

		src, err = store.CreateSourceFromEbmedded(queries.Assets(), "migrations/postgres", cfg.AppliedMigrations)
		if err != nil {
			return fmt.Errorf("could not create source from embedded: %w", err)
		}
	} else if migrationDir == "" {
		mmVersion, _ := cmd.Flags().GetString("mattermost-version")
		if mmVersion == "" {
			return fmt.Errorf("--mattermost-version needs to be supplied to run migrations")
		}
		v, err2 := semver.ParseTolerant(mmVersion)
		if err2 != nil {
			return fmt.Errorf("could not parse mattermost version: %w", err2)
		}

		tempDir, err3 := os.MkdirTemp("", "mattermost")
		if err3 != nil {
			return fmt.Errorf("could not create temp directory: %w", err3)
		}

		baseLogger.Printf("cloning %s@%s\n", "repository", v.String())
		err = git.CloneMigrations(git.CloneOptions{
			TempRepoPath: tempDir,
			Output:       "postgres",
			DriverType:   "postgres",
			Version:      v,
		}, verboseLogger)
		if err != nil {
			return fmt.Errorf("error during cloning migrations: %w", err)
		}

		src, err = file.Open("postgres")
		if err != nil {
			return fmt.Errorf("could not read migrations: %w", err)
		}
	} else {
		src, err = file.Open(migrationDir)
		if err != nil {
			return fmt.Errorf("could not read migrations: %w", err)
		}
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

	postgresDB, err := store.NewStore("postgres", args[0])
	if err != nil {
		return err
	}
	defer postgresDB.Close()

	err = postgresDB.CheckPostgresDefaultSchema(c.Context(), schema, baseLogger)
	if err != nil {
		return fmt.Errorf("could not check default schema: %w", err)
	}

	baseLogger.Println("running migrations..")

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
