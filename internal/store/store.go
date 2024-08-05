package store

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/mattermost/morph"
	"github.com/mattermost/morph/drivers"
	"github.com/mattermost/morph/drivers/mysql"
	"github.com/mattermost/morph/drivers/postgres"
	"github.com/mattermost/morph/models"
	"github.com/mattermost/morph/sources"
	"github.com/mattermost/morph/sources/embedded"

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

type DBConfig struct {
	AppliedMigrations []int `json:"applied_migrations"`
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
func (db *DB) RunMigrations(src sources.Source) error {
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

func CreateSourceFromEbmedded(assets embed.FS, dir string, versions []int) (sources.Source, error) {
	dirEntries, err := assets.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	migrationsShouldBeApplied := make(map[int]string)
	for _, v := range dirEntries {
		m, err2 := models.NewMigration(io.NopCloser(bytes.NewReader(nil)), v.Name())
		if err2 != nil {
			return nil, fmt.Errorf("could not parse migration: %w", err2)
		}

		if m.Direction != models.Up {
			continue
		}

		migrationsShouldBeApplied[int(m.Version)] = v.Name()
	}

	assetNames := make([]string, len(versions))
	for i, v := range versions {
		assetNames[i] = migrationsShouldBeApplied[v]
	}

	res := embedded.Resource(assetNames, func(name string) ([]byte, error) {
		return assets.ReadFile(filepath.Join(dir, name))
	})

	src, err := embedded.WithInstance(res)
	if err != nil {
		return nil, err
	}

	return src, nil
}

func (db *DB) GetAppliedMigrations(ctx context.Context) ([]int, error) {
	rows, err := db.conn.QueryContext(ctx, "SELECT version FROM db_migrations ORDER BY version ASC")
	if err != nil {
		return nil, fmt.Errorf("could not get applied migrations: %w", err)
	}
	defer rows.Close()

	var versions []int
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("could not scan version: %w", err)
		}

		versions = append(versions, version)
	}

	return versions, nil
}
