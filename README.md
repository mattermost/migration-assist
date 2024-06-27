# Migration-Assist

The tool helps to automate the tasks defined in the [Migration guidelines from MySQL to PostgreSQL Documentation](https://docs.mattermost.com/deploy/postgres-migration.html)

## Install

You can use `go` to install the tool.

```
$ go install github.com/mattermost/migration-assist/cmd/migration-assist@latest
```

## Usage

```
A helper tool to assist migration from MySQL to Postgres for Mattermost

Usage:
  migration-assist [command]

Available Commands:
  pgloader      Generates a pgLoader configuration from DSN values
  mysql         Checks the MySQL database schema whether it is ready for the migration
  postgres      Checks the Postgres database schema whether it is ready for the migration
```

The tool provides 3 utility commands to smooth the migration process

### Generate pgLoader Configuration

This sub-command helps administrators by generating a pgLoader configuration. To run the command both MySQL and Postgres DSNs should be provided. The template configuration is based on [docs page](https://docs.mattermost.com/deploy/postgres-migration.html).

Example usage:

```
$ migration-assist pgloader \
--postgres="postgres://mmuser:mostest@localhost:8765/mattermost_test?sslmode=disable" \
--mysql="root:mostest@tcp(localhost:3306)/mattermost_test"
```

Available flags:

```
-h, --help          help for gen-pgloader-config
--mysql string      DSN for MySQL
--output string     The filename of the generated configuration
--postgres string   DSN for Postgres
```

### Check MySQL Schema

Runs several checks against the MySQL database and if any `--fix` flags are provided runs the necessary fixes.

Example usage:

```
$ migration-assist mysql "root:mostest@tcp(localhost:3306)/mattermost_test" \
--fix-unicode
```

Available flags:

```
--fix-artifacts   Removes the artifacts from older versions of Mattermost
--fix-unicode     Removes the unsupported unicode characters from MySQL tables
--fix-varchar     Removes the rows with varchar overflow
-h, --help        help for source-check
```

Please refer to [queries](queries) directory to see which queries will run to check or fix MySQL database.

### Check Postgres Schema

Runs a few checks against the Postgres database. The command also downloads the correct version of the Mattermost repository to prepare the target database. If the `--run-migrations` flag is provided, it will run the migrations with `morph` tooling.

Example usage:

```
migration-assist postgres "postgres://mmuser:mostest@localhost:8765/mattermost_test?sslmode=disable" \
--run-migrations
```

Available flags:

```
-h, --help                    help for target-check
--mattermost-version string   Mattermost version to be cloned to run migrations (default "v8.1")
--migrations-dir string       Migrations directory (should be used if mattermost-version is not supplied)
--run-migrations              Runs migrations for Postgres schema
```
