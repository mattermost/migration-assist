package commands

import (
	"fmt"
	"os"

	"github.com/mattermost/migration-assist/internal/logger"
	"github.com/mattermost/migration-assist/internal/pgloader"
	"github.com/spf13/cobra"
)

func GeneratePgloaderConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pgloader",
		Short:   "Generates a pgLoader configuration from DSN values",
		RunE:    genPgloaderCmdFn(""),
		Example: "  migration-assist pgloader \\\n--postgres=\"postgres://mmuser:mostest@localhost:8765/mattermost_test?sslmode=disable\" \\\n--mysql=\"root:mostest@tcp(localhost:3306)/mattermost_test\"",
	}

	cmd.AddCommand(GeneratePgloaderConfigBoardsCmd())
	cmd.AddCommand(GeneratePgloaderConfigPlaybooksCmd())
	cmd.AddCommand(GeneratePgloaderConfigCallsCmd())

	// Required flags
	cmd.PersistentFlags().String("mysql", "", "DSN for MySQL")
	_ = cmd.MarkFlagRequired("mysql")
	cmd.PersistentFlags().String("postgres", "", "DSN for Postgres")
	_ = cmd.MarkFlagRequired("postgres")

	// Optional flags
	cmd.PersistentFlags().String("output", "", "The filename of the generated configuration")
	cmd.PersistentFlags().Bool("remove-null-chars", true, "Adds transformations to remove null characters on the fly")
	return cmd
}

func GeneratePgloaderConfigBoardsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "boards",
		Short:   "Generates a pgLoader configuration from DSN values for Boards",
		RunE:    genPgloaderCmdFn("boards"),
		Example: "  migration-assist pgloader boards \\\n--postgres=\"postgres://mmuser:mostest@localhost:8765/mattermost_test?sslmode=disable\" \\\n--mysql=\"root:mostest@tcp(localhost:3306)/mattermost_test\"",
	}

	return cmd
}

func GeneratePgloaderConfigPlaybooksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "playbooks",
		Short:   "Generates a pgLoader configuration from DSN values for Playbooks",
		RunE:    genPgloaderCmdFn("playbooks"),
		Example: "  migration-assist pgloader playbooks \\\n--postgres=\"postgres://mmuser:mostest@localhost:8765/mattermost_test?sslmode=disable\" \\\n--mysql=\"root:mostest@tcp(localhost:3306)/mattermost_test\"",
	}

	return cmd
}

func GeneratePgloaderConfigCallsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "calls",
		Short:   "Generates a pgLoader configuration from DSN values for Calls",
		RunE:    genPgloaderCmdFn("calls"),
		Example: "  migration-assist pgloader calls \\\n--postgres=\"postgres://mmuser:mostest@localhost:8765/mattermost_test?sslmode=disable\" \\\n--mysql=\"root:mostest@tcp(localhost:3306)/mattermost_test\"",
	}

	return cmd
}

func genPgloaderCmdFn(product string) func(cmd *cobra.Command, _ []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		mysqlDSN, _ := cmd.Flags().GetString("mysql")
		postgresDSN, _ := cmd.Flags().GetString("postgres")

		output, _ := cmd.Flags().GetString("output")
		removeNull, _ := cmd.Flags().GetBool("remove-null-chars")
		baseLogger := logger.NewLogger(os.Stderr, logger.Options{Timestamps: true})
		err := pgloader.GenerateConfigurationFile(output, product, pgloader.PgLoaderConfig{
			MySQLDSN:             mysqlDSN,
			PostgresDSN:          postgresDSN,
			RemoveNullCharacters: removeNull,
		}, baseLogger)
		if err != nil {
			return fmt.Errorf("could not generate config: %w", err)
		}

		return nil
	}
}
