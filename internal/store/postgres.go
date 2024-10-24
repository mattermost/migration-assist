package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"slices"

	"github.com/mattermost/migration-assist/internal/logger"
)

var (
	ignoredTablesForEmptyCheck = map[string]bool{"db_migrations": true, "systems": true, "config_migrations": true, "db_lock": true}
)

func openPostgres(dataSource string) (*DB, error) {
	db, err := sql.Open("postgres", dataSource)
	if err != nil {
		return nil, fmt.Errorf("failed to open connection with the database: %w", err)
	}

	dbName, err := extractPostgresDatabaseNameFromURL(dataSource)
	if err != nil {
		return nil, fmt.Errorf("could not parse database name: %w", err)
	}

	conn, err := db.Conn(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to grab connection to the database: %w", err)
	}

	return &DB{
		dbType:       "postgres",
		db:           db,
		conn:         conn,
		databaseName: dbName,
	}, nil
}

func extractPostgresDatabaseNameFromURL(conn string) (string, error) {
	uri, err := url.Parse(conn)
	if err != nil {
		return "", err
	}

	return uri.Path[1:], nil
}

func (db *DB) CheckPostgresDefaultSchema(ctx context.Context, schema string, logger logger.LogInterface) error {
	rows, err := db.db.QueryContext(ctx, "SHOW search_path")
	if err != nil {
		return fmt.Errorf("could not determine the search_path: %w", err)
	}
	defer rows.Close()

	var schemas []string
	for rows.Next() {
		var s string
		err := rows.Scan(&s)
		if err != nil {
			return fmt.Errorf("could not scan the schema for search_path: %w", err)
		}
		schemas = append(schemas, s)
	}
	slices.Sort(schemas)

	schemaSetting := fmt.Sprintf("%q, %s", "$user", schema)
	if len(schemas) == 0 {
		return fmt.Errorf("no value available for search_path")
	} else if _, ok := slices.BinarySearch(schemas, schemaSetting); !ok {
		logger.Printf("could not find the default schema %q in search_path, consider setting it from the postgresql console\n", schema)
		err := db.ExecQuery(ctx, fmt.Sprintf("SELECT pg_catalog.set_config('search_path', '%s', false)", schemaSetting))
		if err != nil {
			return fmt.Errorf("could not set search_path for the session: %w", err)
		}
		logger.Printf("search_path is set to %q for the currrent session\n", schema)
	}

	return nil
}

func (db *DB) CheckPostgresSchemaOwnership(ctx context.Context, schema, user string) error {
	cnt, err := db.RunSelectCountQuery(ctx, fmt.Sprintf(`SELECT COUNT(*)
		FROM information_schema.schemata
		WHERE schema_name = '%s'
		AND schema_owner = '%s'`, schema, user))
	if err != nil {
		return fmt.Errorf("could not fetch schema information: %w", err)
	}

	if cnt == 0 {
		return fmt.Errorf("the user %q is not owner of the %q schema", user, schema)
	}

	return nil
}

func (db *DB) CheckIfPostgresTablesEmpty(ctx context.Context) ([]string, error) {
	var tables []string
	rows, err := db.db.QueryContext(ctx, "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public'")
	if err != nil {
		return nil, fmt.Errorf("could not fetch tables from the database: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var table string
		err := rows.Scan(&table)
		if err != nil {
			return nil, fmt.Errorf("could not scan the table name: %w", err)
		}
		if _, ok := ignoredTablesForEmptyCheck[table]; ok {
			continue
		}
		tables = append(tables, table)
	}

	tablesWithData := []string{}
	for _, table := range tables {
		var count int
		err := db.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count)
		if err != nil {
			return nil, fmt.Errorf("could not fetch count from the table %s: %w", table, err)
		}
		if count == 0 {
			continue
		}
		tablesWithData = append(tablesWithData, table)
	}

	return tablesWithData, nil
}
