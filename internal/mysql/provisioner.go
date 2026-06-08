// Package mysql provisions a dedicated database and least-privilege user for
// each WordPress host on the shared MySQL server.
package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Config describes how the operator reaches the admin MySQL connection.
type Config struct {
	Host     string
	Port     string
	User     string // admin user (e.g. root) capable of CREATE USER / GRANT
	Password string
}

// DSN returns a go-sql-driver DSN for the admin connection (no default schema).
func (c Config) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/?parseTime=true&multiStatements=false",
		c.User, c.Password, c.Host, c.Port)
}

// Provisioner manages databases and users on the shared server.
type Provisioner struct {
	cfg Config
}

// New returns a Provisioner. The connection is opened lazily per operation so a
// transiently-unavailable MySQL does not crash the operator at startup.
func New(cfg Config) *Provisioner { return &Provisioner{cfg: cfg} }

func (p *Provisioner) open(ctx context.Context) (*sql.DB, error) {
	db, err := sql.Open("mysql", p.cfg.DSN())
	if err != nil {
		return nil, err
	}
	db.SetConnMaxLifetime(time.Minute)
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	return db, nil
}

// EnsureDatabase creates (idempotently) the database, the dedicated user, sets
// its password, and grants it privileges scoped to ONLY that database. Calling
// it repeatedly is safe; the password is reset to the supplied value so it stays
// in sync with the per-site Secret.
func (p *Provisioner) EnsureDatabase(ctx context.Context, dbName, user, host, password string) error {
	if err := validateIdent(dbName); err != nil {
		return err
	}
	if err := validateIdent(user); err != nil {
		return err
	}
	db, err := p.open(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	// MySQL does NOT support bound parameters (?) in CREATE USER / ALTER USER —
	// they cannot be prepared. The password is escaped and embedded directly.
	// Identifiers (dbName/user) are allow-list validated above; host is operator
	// controlled. Errors use a label so the password is never logged.
	escPass := escapeString(password)
	stmts := []struct{ sql, label string }{
		{fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", dbName), "create database"},
		{fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%s' IDENTIFIED BY '%s'", user, host, escPass), "create user"},
		{fmt.Sprintf("ALTER USER '%s'@'%s' IDENTIFIED BY '%s'", user, host, escPass), "alter user"},
		{fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%s'", dbName, user, host), "grant"},
		{"FLUSH PRIVILEGES", "flush privileges"},
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s.sql); err != nil {
			return fmt.Errorf("mysql %s: %w", s.label, err)
		}
	}
	return nil
}

// DropDatabase removes the database and the dedicated user. Used on site
// deletion when the finalizer requests data removal.
func (p *Provisioner) DropDatabase(ctx context.Context, dbName, user, host string) error {
	if err := validateIdent(dbName); err != nil {
		return err
	}
	if err := validateIdent(user); err != nil {
		return err
	}
	db, err := p.open(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	stmts := []string{
		fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dbName),
		fmt.Sprintf("DROP USER IF EXISTS '%s'@'%s'", user, host),
		"FLUSH PRIVILEGES",
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("mysql exec %q: %w", s, err)
		}
	}
	return nil
}

// escapeString escapes a value for safe inclusion inside a single-quoted MySQL
// string literal (used for the password in CREATE/ALTER USER, which cannot take
// bound parameters).
func escapeString(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\'', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case 0:
			b.WriteString(`\0`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// validateIdent guards against SQL injection via identifiers (which cannot be
// passed as bound parameters). Names come from a sanitised, allow-listed set.
func validateIdent(s string) error {
	if s == "" || len(s) > 64 {
		return fmt.Errorf("invalid identifier %q", s)
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
		if !ok {
			return fmt.Errorf("invalid character in identifier %q", s)
		}
	}
	return nil
}
