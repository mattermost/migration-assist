package main

import (
	"fmt"
	"os"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"github.com/spf13/cobra"

	"github.com/mattermost/migration-assist/cmd/migration-assist/commands"
)

var root = &cobra.Command{
	Use:           "migration-assist",
	Short:         "A helper tool to assist a migration from MySQL to Postgres for Mattermost",
	Version:       "v0.1.0",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func main() {
	root.PersistentFlags().Bool("verbose", false, "Becomes verbose")

	root.AddCommand(
		commands.SourceCheckCmd(),
		commands.TargetCheckCmd(),
		commands.GeneratePgloaderConfigCmd(),
		commands.VersionCmd(),
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "An Error Occurred: %s\n", err.Error())
		os.Exit(1)
	}
}
