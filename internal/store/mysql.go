package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-sql-driver/mysql"
	"github.com/mattermost/migration-assist/internal/git"
	"github.com/mattermost/migration-assist/internal/logger"
)

type CreateTable struct {
	Table       string
	CreateTable string
}

func openMySQL(dataSource string) (*DB, error) {
	sanitizedDataSource, err := appendMultipleStatementsFlag(dataSource)
	if err != nil {
		return nil, fmt.Errorf("could not parse DSN: %w", err)
	}

	db, err := sql.Open("mysql", sanitizedDataSource)
	if err != nil {
		return nil, fmt.Errorf("failed to open connection with the database: %w", err)
	}

	dbName, err := extractMySQLDatabaseNameFromURL(sanitizedDataSource)
	if err != nil {
		return nil, fmt.Errorf("could not parse database name: %w", err)
	}

	conn, err := db.Conn(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to grab connection to the database: %w", err)
	}

	return &DB{
		dbType:       "mysql",
		db:           db,
		conn:         conn,
		databaseName: dbName,
	}, nil
}

func extractMySQLDatabaseNameFromURL(conn string) (string, error) {
	cfg, err := mysql.ParseDSN(conn)
	if err != nil {
		return "", err
	}

	return cfg.DBName, nil
}

func appendMultipleStatementsFlag(dataSource string) (string, error) {
	config, err := mysql.ParseDSN(dataSource)
	if err != nil {
		return "", err
	}

	if config.Params == nil {
		config.Params = map[string]string{}
	}

	config.Params["multiStatements"] = "true"
	return config.FormatDSN(), nil
}

func CompareMySQL(a, b *DB, baseLogger, verboseLogger logger.LogInterface, saveDiff bool) error {
	testConn, err := b.GetDB().Conn(context.TODO())
	if err != nil {
		return fmt.Errorf("could not grab connection from test db: %w", err)
	}

	tables := make([]string, 0)
	rows, err := testConn.QueryContext(context.TODO(), "SHOW TABLES")
	if err != nil {
		return fmt.Errorf("could notget tables test db: %w", err)
	}

	for rows.Next() {
		var table string
		if err2 := rows.Scan(&table); err2 != nil {
			return fmt.Errorf("error while scanning tables from test db: %w", err2)
		}
		tables = append(tables, table)
	}

	if err = rows.Err(); err != nil {
		return fmt.Errorf("error during query: %w", err)
	}

	actualConn, err := a.GetDB().Conn(context.TODO())
	if err != nil {
		return fmt.Errorf("could not grab connection from actual db: %w", err)
	}

	baseLogger.Println("comparing tables...")
	var diffTables int
	for _, table := range tables {
		row := testConn.QueryRowContext(context.TODO(), fmt.Sprintf("SHOW CREATE TABLE %s", table))
		var expected CreateTable
		err = row.Scan(&expected.Table, &expected.CreateTable)
		if err != nil {
			return fmt.Errorf("could not get table definition from test db: %w", err)
		}
		var actual CreateTable
		row = actualConn.QueryRowContext(context.TODO(), fmt.Sprintf("SHOW CREATE TABLE %s", table))
		err = row.Scan(&actual.Table, &actual.CreateTable)
		if err != nil {
			return fmt.Errorf("could not get table definition from actual db: %w", err)
		}

		diff := git.Diff(actual.CreateTable, expected.CreateTable)
		if diff != "" {
			diffTables++

			if !saveDiff {
				baseLogger.Printf("%s table is not as expected. Diff:\n%s\n", table, diff)
				continue
			}
			verboseLogger.Printf("%s table differs from what is expected.\n", table)

			_ = os.RemoveAll("diffs")
			err = os.MkdirAll("diffs", 0750)
			if err != nil {
				return fmt.Errorf("could not create diff directory: %w", err)
			}

			err := os.WriteFile(filepath.Join("diffs", table+".diff"), []byte(diff), 0644)
			if err != nil && !os.IsExist(err) {
				return fmt.Errorf("could not create diff directory: %w", err)
			}
		}
	}
	if diffTables == 0 {
		verboseLogger.Printf("MySQL tables are equal from what is expected.\n")
	}

	return nil
}
