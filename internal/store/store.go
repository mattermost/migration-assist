package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"path/filepath"
	"time"

	"github.com/mattermost/morph"
	"github.com/mattermost/morph/drivers"
	"github.com/mattermost/morph/drivers/mysql"
	"github.com/mattermost/morph/drivers/postgres"
	"github.com/mattermost/morph/sources/file"

	"github.com/mattermost/migration-assist/internal/logger"
)

const (
	statementTimeoutInSeconds = 60 * 5 // 5 minutes
)

type DB struct {
	dbType       string
	databaseName string
	db           *sql.DB
	conn         *sql.Conn
}

func NewStore(dbType string, dataSource string) (*DB, error) {
	switch dbType {
	case "mysql":
		return openMySQL(dataSource)
	case "postgres":
		return openPostgres(dataSource)
	default:
		return nil, fmt.Errorf("unsupported db type: %s", dbType)
	}
}

func (db *DB) GetDB() *sql.DB {
	return db.db
}

func (db *DB) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), statementTimeoutInSeconds*time.Second)
	defer cancel()

	return db.conn.PingContext(ctx)
}

func (db *DB) Close() error {
	if db.conn != nil {
		if err := db.conn.Close(); err != nil {
			return fmt.Errorf("could not close connection: %w", err)
		}
	}

	if db.db != nil {
		if err := db.db.Close(); err != nil {
			return fmt.Errorf("could not close DB: %w", err)
		}
	}

	return nil
}

func (db *DB) RunSelectCountQuery(ctx context.Context, query string) (int, error) {
	var count int
	err := db.conn.QueryRowContext(ctx, query).Scan(&count)

	return count, err
}

func (db *DB) ExecQuery(ctx context.Context, query string) error {
	_, err := db.conn.ExecContext(ctx, query)

	return err
}

// RunMigrations will run all of the migrations within a directory,
func (db *DB) RunEmbeddedMigrations(assets embed.FS, dir string, logger logger.LogInterface) error {
	queries, err := assets.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, query := range queries {
		b, err := assets.ReadFile(filepath.Join("post-migrate", query.Name()))
		if err != nil {
			return fmt.Errorf("could not read embedded sql file: %w", err)
		}

		logger.Printf("applying %s\n", query.Name())
		err = db.ExecQuery(context.TODO(), string(b))
		if err != nil {
			return fmt.Errorf("error during running post-migrate queries: %w", err)
		}
	}

	return nil
}

// RunMigrations will run the migrations form a given directory with morph
func (db *DB) RunMigrations(dir string) error {
	var driver drivers.Driver
	var err error
	switch db.dbType {
	case "mysql":
		driver, err = mysql.WithInstance(db.db)
		if err != nil {
			return fmt.Errorf("could not initialize driver: %w", err)
		}
	case "postgres":
		driver, err = postgres.WithInstance(db.db)
		if err != nil {
			return fmt.Errorf("could not initialize driver: %w", err)
		}
	default:
		return fmt.Errorf("unsupported db type: %s", db.dbType)
	}

	src, err := file.Open(dir)
	if err != nil {
		return fmt.Errorf("could not read migrations: %w", err)
	}

	engine, err := morph.New(context.TODO(), driver, src, morph.WithLogger(logger.NewNopLogger()))
	if err != nil {
		return fmt.Errorf("could not initialize morph: %w", err)
	}

	err = engine.ApplyAll()
	if err != nil {
		return fmt.Errorf("could not apply migrations: %w", err)
	}

	return nil
}
