// Package mysql provisions a dedicated database and least-privilege user for
// each WordPress host on the shared MySQL server.
package mysql

import (
	"context"
	"database/sql"
	"fmt"
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

	stmts := []string{
		fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", dbName),
		fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%s' IDENTIFIED BY ?", user, host),
		fmt.Sprintf("ALTER USER '%s'@'%s' IDENTIFIED BY ?", user, host),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%s'", dbName, user, host),
		"FLUSH PRIVILEGES",
	}
	for i, s := range stmts {
		var execErr error
		// Statements 1 and 2 (0-indexed) carry the password parameter.
		if i == 1 || i == 2 {
			_, execErr = db.ExecContext(ctx, s, password)
		} else {
			_, execErr = db.ExecContext(ctx, s)
		}
		if execErr != nil {
			return fmt.Errorf("mysql exec %q: %w", s, execErr)
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
