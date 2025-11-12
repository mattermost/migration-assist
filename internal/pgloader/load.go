package pgloader

import (
	"embed"
	"fmt"
	"io"
	"net/url"
	"os"
	"text/template"

	"github.com/go-sql-driver/mysql"
	"github.com/mattermost/migration-assist/internal/logger"
	"github.com/mattermost/migration-assist/internal/store"
)

//go:embed templates
var assets embed.FS

type Parameters struct {
	MySQLUser     string
	MySQLPassword string
	MySQLAddress  string
	SourceSchema  string

	PGUser       string
	PGPassword   string
	PGAddress    string
	TargetSchema string

	RemoveNullCharacters bool
	SearchPath           string
}

type PgLoaderConfig struct {
	MySQLDSN    string
	PostgresDSN string

	RemoveNullCharacters bool
}

func GenerateConfigurationFile(output, product string, config PgLoaderConfig, baseLogger logger.LogInterface) error {
	var f string
	switch product {
	case "boards":
		f = "boards"
	case "playbooks":
		f = "playbooks"
	case "calls":
		f = "calls"
	default:
		f = "config"
	}
	bytes, err := assets.ReadFile(fmt.Sprintf("templates/%s.tmpl", f))
	if err != nil {
		return fmt.Errorf("could not read configuration template: %w", err)
	}

	templ, err := template.New("cfg").Parse(string(bytes))
	if err != nil {
		return fmt.Errorf("could not parse template: %w", err)
	}

	params := Parameters{
		RemoveNullCharacters: config.RemoveNullCharacters,
	}
	err = parseMySQL(&params, config.MySQLDSN)
	if err != nil {
		return fmt.Errorf("could not parse mysql DSN: %w", err)
	}

	err = ParsePostgres(&params, config.PostgresDSN)
	if err != nil {
		return fmt.Errorf("could not parse postgres DSN: %w", err)
	}

	postgresDB, err := store.NewStore("postgres", config.PostgresDSN)
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

	row := postgresDB.GetDB().QueryRow("SHOW SEARCH_PATH")
	if row.Err() != nil {
		return fmt.Errorf("could not query search path: %w", err)
	}
	err = row.Scan(&params.SearchPath)
	if err != nil {
		return fmt.Errorf("could not query scan search path: %w", err)
	}

	var writer io.Writer
	switch output {
	case "":
		writer = os.Stdout
	default:
		f, err2 := os.Create(output)
		if err2 != nil {
			return err2
		}
		defer f.Close()
		writer = f
	}

	err = templ.Execute(writer, params)
	if err != nil {
		return fmt.Errorf("error during executing the template: %w", err)
	}

	return nil
}

func parseMySQL(params *Parameters, dsn string) error {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("could not parse MySQL DSN: %w", err)
	}

	params.MySQLAddress = cfg.Addr
	params.MySQLUser = cfg.User
	params.MySQLPassword = cfg.Passwd
	params.SourceSchema = cfg.DBName

	return nil
}

func ParsePostgres(params *Parameters, dsn string) error {
	uri, err := url.Parse(dsn)
	if err != nil {
		return fmt.Errorf("could not parse PostgreSQL DSN: %w", err)
	}

	if uri.Scheme != "postgres" && uri.Scheme != "postgresql" && uri.Scheme != "pgsql" {
		return fmt.Errorf("invalid scheme: expected postgres or postgresql, got %s", uri.Scheme)
	}

	params.PGUser = uri.User.Username()
	params.PGPassword, _ = uri.User.Password()

	host := uri.Hostname()
	port := uri.Port()
	if port == "" {
		port = "5432" // default PostgreSQL port
	}
	params.PGAddress = fmt.Sprintf("%s:%s", host, port)

	// Remove leading slash from path
	dbName := uri.Path
	if len(dbName) > 0 && dbName[0] == '/' {
		dbName = dbName[1:]
	}
	params.TargetSchema = dbName

	if dbName == "" {
		return fmt.Errorf("database name is empty, please provide a valid database name")
	}

	return nil
}
