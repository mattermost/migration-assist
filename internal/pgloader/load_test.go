package pgloader

import (
	"testing"
)

func TestParseMySQL(t *testing.T) {
	tests := []struct {
		name      string
		dsn       string
		want      Parameters
		wantError bool
	}{
		{
			name: "standard MySQL DSN",
			dsn:  "user:password@tcp(localhost:3306)/mydb",
			want: Parameters{
				MySQLUser:     "user",
				MySQLPassword: "password",
				MySQLAddress:  "localhost:3306",
				SourceSchema:  "mydb",
			},
			wantError: false,
		},
		{
			name: "MySQL DSN with IP address",
			dsn:  "root:secret@tcp(192.168.1.100:3306)/testdb",
			want: Parameters{
				MySQLUser:     "root",
				MySQLPassword: "secret",
				MySQLAddress:  "192.168.1.100:3306",
				SourceSchema:  "testdb",
			},
			wantError: false,
		},
		{
			name: "MySQL DSN with empty password",
			dsn:  "user:@tcp(localhost:3306)/mydb",
			want: Parameters{
				MySQLUser:     "user",
				MySQLPassword: "",
				MySQLAddress:  "localhost:3306",
				SourceSchema:  "mydb",
			},
			wantError: false,
		},
		{
			name: "MySQL DSN with special characters in password",
			dsn:  "user:p@ssw0rd!@#@tcp(localhost:3306)/mydb",
			want: Parameters{
				MySQLUser:     "user",
				MySQLPassword: "p@ssw0rd!@#",
				MySQLAddress:  "localhost:3306",
				SourceSchema:  "mydb",
			},
			wantError: false,
		},
		{
			name: "MySQL DSN with custom port",
			dsn:  "user:password@tcp(localhost:3307)/mydb",
			want: Parameters{
				MySQLUser:     "user",
				MySQLPassword: "password",
				MySQLAddress:  "localhost:3307",
				SourceSchema:  "mydb",
			},
			wantError: false,
		},
		{
			name: "MySQL DSN with database name containing underscore",
			dsn:  "user:password@tcp(localhost:3306)/my_db",
			want: Parameters{
				MySQLUser:     "user",
				MySQLPassword: "password",
				MySQLAddress:  "localhost:3306",
				SourceSchema:  "my_db",
			},
			wantError: false,
		},
		{
			name: "MySQL DSN with empty database name",
			dsn:  "user:password@tcp(localhost:3306)/",
			want: Parameters{
				MySQLUser:     "user",
				MySQLPassword: "password",
				MySQLAddress:  "localhost:3306",
				SourceSchema:  "",
			},
			wantError: false,
		},
		{
			name:      "invalid MySQL DSN - malformed",
			dsn:       "invalid-dsn",
			wantError: true,
		},
		{
			name: "MySQL DSN with empty string (produces default values)",
			dsn:  "",
			want: Parameters{
				MySQLUser:     "",
				MySQLPassword: "",
				MySQLAddress:  "127.0.0.1:3306", // mysql.ParseDSN defaults to localhost:3306
				SourceSchema:  "",
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var params Parameters
			err := parseMySQL(&params, tt.dsn)

			if tt.wantError {
				if err == nil {
					t.Errorf("parseMySQL() expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("parseMySQL() error = %v, want no error", err)
				return
			}

			if params.MySQLUser != tt.want.MySQLUser {
				t.Errorf("parseMySQL() MySQLUser = %v, want %v", params.MySQLUser, tt.want.MySQLUser)
			}
			if params.MySQLPassword != tt.want.MySQLPassword {
				t.Errorf("parseMySQL() MySQLPassword = %v, want %v", params.MySQLPassword, tt.want.MySQLPassword)
			}
			if params.MySQLAddress != tt.want.MySQLAddress {
				t.Errorf("parseMySQL() MySQLAddress = %v, want %v", params.MySQLAddress, tt.want.MySQLAddress)
			}
			if params.SourceSchema != tt.want.SourceSchema {
				t.Errorf("parseMySQL() SourceSchema = %v, want %v", params.SourceSchema, tt.want.SourceSchema)
			}
		})
	}
}

func TestParsePostgres(t *testing.T) {
	tests := []struct {
		name      string
		dsn       string
		want      Parameters
		wantError bool
	}{
		{
			name: "standard PostgreSQL DSN",
			dsn:  "postgres://user:password@localhost:5432/mydb",
			want: Parameters{
				PGUser:       "user",
				PGPassword:   "password",
				PGAddress:    "localhost:5432",
				TargetSchema: "mydb",
			},
			wantError: false,
		},
		{
			name: "PostgreSQL DSN with postgresql scheme",
			dsn:  "postgresql://user:password@localhost:5432/mydb",
			want: Parameters{
				PGUser:       "user",
				PGPassword:   "password",
				PGAddress:    "localhost:5432",
				TargetSchema: "mydb",
			},
			wantError: false,
		},
		{
			name: "PostgreSQL DSN without port (should default to 5432)",
			dsn:  "postgres://user:password@localhost/mydb",
			want: Parameters{
				PGUser:       "user",
				PGPassword:   "password",
				PGAddress:    "localhost:5432",
				TargetSchema: "mydb",
			},
			wantError: false,
		},
		{
			name: "PostgreSQL DSN with custom port",
			dsn:  "postgres://user:password@localhost:8765/mydb",
			want: Parameters{
				PGUser:       "user",
				PGPassword:   "password",
				PGAddress:    "localhost:8765",
				TargetSchema: "mydb",
			},
			wantError: false,
		},
		{
			name: "PostgreSQL DSN with empty password",
			dsn:  "postgres://user@localhost:5432/mydb",
			want: Parameters{
				PGUser:       "user",
				PGPassword:   "",
				PGAddress:    "localhost:5432",
				TargetSchema: "mydb",
			},
			wantError: false,
		},
		{
			name: "PostgreSQL DSN with IP address",
			dsn:  "postgres://user:password@192.168.1.100:5432/mydb",
			want: Parameters{
				PGUser:       "user",
				PGPassword:   "password",
				PGAddress:    "192.168.1.100:5432",
				TargetSchema: "mydb",
			},
			wantError: false,
		},
		{
			name: "PostgreSQL DSN with special characters in password",
			dsn:  "postgres://user:p@ssw0rd%21%40%23@localhost:5432/mydb",
			want: Parameters{
				PGUser:       "user",
				PGPassword:   "p@ssw0rd!@#",
				PGAddress:    "localhost:5432",
				TargetSchema: "mydb",
			},
			wantError: false,
		},
		{
			name: "PostgreSQL DSN with query parameters",
			dsn:  "postgres://user:password@localhost:5432/mydb?sslmode=disable",
			want: Parameters{
				PGUser:       "user",
				PGPassword:   "password",
				PGAddress:    "localhost:5432",
				TargetSchema: "mydb",
			},
			wantError: false,
		},
		{
			name: "PostgreSQL DSN with database name containing underscore",
			dsn:  "postgres://user:password@localhost:5432/my_db",
			want: Parameters{
				PGUser:       "user",
				PGPassword:   "password",
				PGAddress:    "localhost:5432",
				TargetSchema: "my_db",
			},
			wantError: false,
		},
		{
			name: "PostgreSQL DSN with multiple query parameters",
			dsn:  "postgres://user:password@localhost:5432/mydb?sslmode=disable&connect_timeout=10",
			want: Parameters{
				PGUser:       "user",
				PGPassword:   "password",
				PGAddress:    "localhost:5432",
				TargetSchema: "mydb",
			},
			wantError: false,
		},
		{
			name:      "invalid PostgreSQL DSN - wrong scheme",
			dsn:       "mysql://user:password@localhost:5432/mydb",
			wantError: true,
		},
		{
			name:      "invalid PostgreSQL DSN - missing scheme",
			dsn:       "user:password@localhost:5432/mydb",
			wantError: true,
		},
		{
			name: "PostgreSQL DSN with malformed host (url.Parse succeeds)",
			dsn:  "postgres://invalid-dsn",
			want: Parameters{
				PGUser:       "",
				PGPassword:   "",
				PGAddress:    "invalid-dsn:5432",
				TargetSchema: "",
			},
			wantError: true,
		},
		{
			name:      "invalid PostgreSQL DSN - empty string",
			dsn:       "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var params Parameters
			err := ParsePostgres(&params, tt.dsn)

			if tt.wantError {
				if err == nil {
					t.Errorf("ParsePostgres() expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ParsePostgres() error = %v, want no error", err)
				return
			}

			if params.PGUser != tt.want.PGUser {
				t.Errorf("ParsePostgres() PGUser = %v, want %v", params.PGUser, tt.want.PGUser)
			}
			if params.PGPassword != tt.want.PGPassword {
				t.Errorf("ParsePostgres() PGPassword = %v, want %v", params.PGPassword, tt.want.PGPassword)
			}
			if params.PGAddress != tt.want.PGAddress {
				t.Errorf("ParsePostgres() PGAddress = %v, want %v", params.PGAddress, tt.want.PGAddress)
			}
			if params.TargetSchema != tt.want.TargetSchema {
				t.Errorf("ParsePostgres() TargetSchema = %v, want %v", params.TargetSchema, tt.want.TargetSchema)
			}
		})
	}
}
