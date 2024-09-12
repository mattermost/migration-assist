package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/spf13/cobra"
	"github.com/testcontainers/testcontainers-go"

	module "github.com/testcontainers/testcontainers-go/modules/mysql"

	"github.com/mattermost/migration-assist/internal/git"
	"github.com/mattermost/migration-assist/internal/logger"
	"github.com/mattermost/migration-assist/internal/store"
	"github.com/mattermost/migration-assist/queries"
	"github.com/mattermost/morph/sources/file"
)

func SourceCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "mysql",
		Short:   "Checks the MySQL database schema whether it is ready for the migration",
		RunE:    runSourceCheckCmdF,
		Example: "  Rmigration-assist mysql \"root:mostest@tcp(localhost:3306)/mattermost_test\" \\\n--fix-unicode",
		Args:    cobra.MinimumNArgs(1),
	}

	// Optional flags
	cmd.Flags().Bool("fix-artifacts", false, "Removes the artifacts from older versions of Mattermost")
	cmd.Flags().Bool("fix-varchar", false, "Removes the rows with varchar overflow")
	cmd.Flags().Bool("fix-unicode", false, "Removes the unsupported unicode characters from MySQL tables")
	cmd.Flags().Bool("full-schema-check", false, "Checks the MySQL schema to determine whether it's in desired state")
	cmd.Flags().Bool("save-diff", false, "Writes diffs to files")
	cmd.Flags().String("migrations-dir", "", "Migrations directory (should be used if mattermost-version is not supplied)")
	cmd.Flags().String("mattermost-version", "v9.7", "Mattermost version to be cloned to run migrations")
	cmd.Flags().String("output", "mysql.output", "Output file for the applied migrations, postgres subcommands will use this file to apply the migrations")

	return cmd
}

func runSourceCheckCmdF(cmd *cobra.Command, args []string) error {
	baseLogger := logger.NewLogger(os.Stderr, logger.Options{Timestamps: true})
	var verboseLogger logger.LogInterface

	verbose, _ := cmd.Flags().GetBool("verbose")
	if verbose {
		verboseLogger = baseLogger
	} else {
		verboseLogger = logger.NewNopLogger()
	}

	mysqlDB, err := store.NewStore("mysql", args[0])
	if err != nil {
		return err
	}
	defer mysqlDB.Close()

	baseLogger.Println("pinging mysql...")
	err = mysqlDB.Ping()
	if err != nil {
		return fmt.Errorf("could not ping mysql: %w", err)
	}
	baseLogger.Println("connected to mysql successfully...")

	applied, err := mysqlDB.GetAppliedMigrations(cmd.Context())
	if err != nil {
		return fmt.Errorf("could not get applied migrations: %w", err)
	}
	mysqlConfig := store.DBConfig{
		AppliedMigrations: applied,
	}

	b, err := json.MarshalIndent(mysqlConfig, "", "    ")
	if err != nil {
		return fmt.Errorf("could not marshal mysql config: %w", err)
	}

	outputFile, _ := cmd.Flags().GetString("output")
	if _, err = os.Stat(outputFile); err == nil || os.IsExist(err) {
		if ConfirmationPrompt("Output file already exists, do you want to overwrite it?") {
			if err = os.Remove(outputFile); err != nil {
				return fmt.Errorf("could not remove output file: %w", err)
			}
		} else {
			baseLogger.Println("Output file already exists, will not overwrite it.")
		}
	}
	if err = os.WriteFile(outputFile, b, 0666); err != nil {
		return fmt.Errorf("could not write to output file: %w", err)
	}

	fullSchema, _ := cmd.Flags().GetBool("full-schema-check")
	if fullSchema {
		mmVersion, _ := cmd.Flags().GetString("mattermost-version")
		v, err2 := semver.ParseTolerant(mmVersion)
		if err2 != nil {
			return fmt.Errorf("could not parse version: %w", err2)
		}

		tempDir, err3 := os.MkdirTemp("", "mattermost")
		if err3 != nil {
			return fmt.Errorf("could not create temp directory: %w", err3)
		}

		migrationsDir, _ := cmd.Flags().GetString("migrations-dir")
		saveDiff, _ := cmd.Flags().GetBool("save-diff")

		err = runFullSchemaCheck(mysqlDB, migrationsDir, tempDir, v, baseLogger, verboseLogger, saveDiff)
		if err != nil {
			return fmt.Errorf("error during full schema check: %w", err)
		}
	}

	// create procedures
	cleanUpFn, err := createProcedures(mysqlDB, baseLogger)
	if err != nil {
		return fmt.Errorf("error during creating procedures for mysql: %w", err)
	}
	defer cleanUpFn()

	// run MySQL schema checks
	fixArtifacts, _ := cmd.Flags().GetBool("fix-artifacts")

	err = runChecksForMySQL(mysqlDB, "artifacts", fixArtifacts, baseLogger, verboseLogger)
	if err != nil {
		return fmt.Errorf("error during running artifact checks for mysql: %w", err)
	}

	fixUnicode, _ := cmd.Flags().GetBool("fix-unicode")

	err = runChecksForMySQL(mysqlDB, "unicode", fixUnicode, baseLogger, verboseLogger)
	if err != nil {
		return fmt.Errorf("error during running unicode checks for mysql: %w", err)
	}

	fixVarchar, _ := cmd.Flags().GetBool("fix-varchar")

	err = runChecksForMySQL(mysqlDB, "varchar", fixVarchar, baseLogger, verboseLogger)
	if err != nil {
		return fmt.Errorf("error during running varchar checks for mysql: %w", err)
	}

	err = runChecksForMySQL(mysqlDB, "varchar-extended", fixVarchar, baseLogger, verboseLogger)
	if err != nil {
		return fmt.Errorf("error during running varchar checks for mysql: %w", err)
	}

	return nil
}

func createProcedures(db *store.DB, baseLogger logger.LogInterface) (func(), error) {
	assets := queries.Assets()

	procedures, err := assets.ReadDir("procedures")
	if err != nil {
		return nil, err
	}

	for _, procedure := range procedures {
		if !strings.HasPrefix(procedure.Name(), "create") {
			continue
		}
		b, err := assets.ReadFile(filepath.Join("procedures", procedure.Name()))
		if err != nil {
			baseLogger.Printf("could not read embedded sql file: %s", err)
		}
		err = db.ExecQuery(context.TODO(), string(b))
		if err != nil {
			baseLogger.Printf("error during creating procedures: %s", err)
		}
	}

	cleanUpFn := func() {
		for _, procedure := range procedures {
			if !strings.HasPrefix(procedure.Name(), "drop") {
				continue
			}
			b, err := assets.ReadFile(filepath.Join("procedures", procedure.Name()))
			if err != nil {
				baseLogger.Printf("could not read embedded sql file: %s", err)
			}
			err = db.ExecQuery(context.TODO(), string(b))
			if err != nil {
				baseLogger.Printf("error during dropping procedures: %s", err)
			}
		}
	}

	return cleanUpFn, nil
}

func runChecksForMySQL(db *store.DB, checkType string, fix bool, baseLogger, verboseLogger logger.LogInterface) error {
	assets := queries.Assets()

	checks, err := assets.ReadDir(filepath.Join("checks", checkType))
	if err != nil {
		return err
	}

	var fixRequired, totalCheck int
	baseLogger.Printf("running checks for %s...\n", checkType)
	for _, artifact := range checks {
		if !strings.HasPrefix(artifact.Name(), "check") {
			continue
		}
		name := stripQueryName(artifact.Name())
		b, err := assets.ReadFile(filepath.Join("checks", checkType, artifact.Name()))
		if err != nil {
			return fmt.Errorf("could not read embedded sql file: %w", err)
		}
		verboseLogger.Printf("checking %s...", name)
		count, err := db.RunSelectCountQuery(context.TODO(), string(b))
		if err != nil {
			return fmt.Errorf("error during running checks: %w", err)
		}
		totalCheck++
		if count == 0 {
			verboseLogger.Printf("%s is okay", name)
			continue
		}
		fixRequired++

		baseLogger.Printf("a fix is required for: %s\n", name)
		if !fix {
			continue
		}

		fixQ, err := assets.ReadFile(filepath.Join("fixes", checkType, "fix_"+strings.TrimPrefix(artifact.Name(), "check_")))
		if err != nil {
			return fmt.Errorf("could not read embedded sql file: %w", err)
		}

		err = db.ExecQuery(context.TODO(), string(fixQ))
		if err != nil {
			return fmt.Errorf("error while trying to fix %s error: %w", name, err)
		}
		baseLogger.Println("the fix query has been executed successfully.")
		fixRequired--
	}

	if fixRequired == 0 {
		baseLogger.Printf("%d checks been made, all good for %s\n", totalCheck, checkType)
	} else {
		baseLogger.Printf("%d checks been made, %d fix(es) is required for %s\n", totalCheck, fixRequired, checkType)
	}

	return nil
}

func stripQueryName(fileName string) string {
	fileName = strings.TrimPrefix(fileName, "check_")
	fileName = strings.TrimPrefix(fileName, "fix_")
	return strings.TrimSuffix(fileName, ".sql")
}

func runFullSchemaCheck(db *store.DB, migrationsDir, tempDir string, v semver.Version, baseLogger, verboseLogger logger.LogInterface, saveDiff bool) error {
	ctx := context.Background()

	var mysqlContainer *module.MySQLContainer
	var err error

	baseLogger.Println("setting up a test MySQL instance...")
	mysqlContainer, err = module.RunContainer(ctx,
		// TODO: get version from user database
		testcontainers.WithImage("mysql:8.0.36"),
		testcontainers.WithLogger(verboseLogger),
		module.WithDatabase("foo"),
		module.WithDefaultCredentials(),
	)
	if err != nil {
		log.Fatalf("failed to start container: %s", err)
	}
	defer func() {
		verboseLogger.Println("terminating test container...")

		if err2 := mysqlContainer.Terminate(ctx); err2 != nil {
			log.Fatalf("failed to terminate container: %s", err2)
		}
	}()

	connectionString, err := mysqlContainer.ConnectionString(ctx, "multiStatements=true", "tls=skip-verify")
	if err != nil {
		log.Fatalf("failed to get connection string of container: %s", err)
	}

	dir := "mysql"
	if migrationsDir == "" {
		baseLogger.Printf("cloning %s@%s\n", "repository", v.String())
		err = git.CloneMigrations(git.CloneOptions{
			TempRepoPath: tempDir,
			Output:       dir,
			DriverType:   "mysql",
			Version:      v,
		}, verboseLogger)
		if err != nil {
			return fmt.Errorf("error during cloning migrations: %w", err)
		}
	} else {
		dir = migrationsDir
	}

	// create mysql connection
	testDB, err := store.NewStore("mysql", connectionString)
	if err != nil {
		return err
	}
	defer testDB.Close()

	// run the migrations
	baseLogger.Println("running migrations...")

	src, err := file.Open(dir)
	if err != nil {
		return fmt.Errorf("could not read migrations: %w", err)
	}

	err = testDB.RunMigrations(src)
	if err != nil {
		return fmt.Errorf("could not run migrations: %w", err)
	}
	baseLogger.Println("migrations applied.")

	err = store.CompareMySQL(db, testDB, baseLogger, verboseLogger, saveDiff)
	if err != nil {
		return fmt.Errorf("failed to run schema comparison: %w", err)
	}

	return nil
}
