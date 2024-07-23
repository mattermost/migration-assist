package pgloader

import (
	"embed"
	"fmt"
	"io"
	"os"
	"regexp"
	"text/template"

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
	regex := regexp.MustCompile(`^(?P<user>[^:]+):(?P<password>[^@]+)@tcp\((?P<address>[^:]+):(?P<port>\d+)\)\/(?P<database>.+)$`)
	match := regex.FindStringSubmatch(dsn)

	if len(match) > 0 {
		user := match[regex.SubexpIndex("user")]
		password := match[regex.SubexpIndex("password")]
		address := match[regex.SubexpIndex("address")]
		port := match[regex.SubexpIndex("port")]
		database := match[regex.SubexpIndex("database")]

		params.MySQLAddress = fmt.Sprintf("%s:%s", address, port)
		params.MySQLUser = user
		params.MySQLPassword = password
		params.SourceSchema = database
	} else {
		return fmt.Errorf("no match found")
	}
	return nil
}

func ParsePostgres(params *Parameters, dsn string) error {
	regex := regexp.MustCompile(`^postgres:\/\/(?P<user>[^:]+):(?P<password>[^@]+)@(?P<address>[^:]+):(?P<port>\d+)\/(?P<database>[^\?]+)`)
	match := regex.FindStringSubmatch(dsn)

	if len(match) > 0 {
		user := match[regex.SubexpIndex("user")]
		password := match[regex.SubexpIndex("password")]
		address := match[regex.SubexpIndex("address")]
		port := match[regex.SubexpIndex("port")]
		database := match[regex.SubexpIndex("database")]

		params.PGAddress = fmt.Sprintf("%s:%s", address, port)
		params.PGUser = user
		params.PGPassword = password
		params.TargetSchema = database
	} else {
		return fmt.Errorf("no match found")
	}
	return nil
}
