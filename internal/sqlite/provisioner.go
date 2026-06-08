// Package sqlite is a local-dev replacement for the MySQL provisioner. Instead
// of a shared MySQL server it materialises one SQLite database file per site, so
// the operator/API can run on a laptop with no MySQL. It satisfies the same
// controller.Database interface as internal/mysql.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // pure-Go driver, registers "sqlite"
)

// Provisioner creates per-site SQLite databases under a directory.
type Provisioner struct {
	dir string
}

// New ensures the directory exists and returns a Provisioner.
func New(dir string) (*Provisioner, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create sqlite dir: %w", err)
	}
	return &Provisioner{dir: dir}, nil
}

func (p *Provisioner) path(dbName string) (string, error) {
	if dbName == "" || strings.ContainsAny(dbName, `/\.`) {
		return "", fmt.Errorf("invalid sqlite database name %q", dbName)
	}
	return filepath.Join(p.dir, dbName+".db"), nil
}

// EnsureDatabase creates the per-site SQLite file and records the "owner". SQLite
// has no users/grants, so user/host/password are stored as metadata only — they
// stay meaningful when the same code path runs against MySQL in production.
func (p *Provisioner) EnsureDatabase(ctx context.Context, dbName, user, host, password string) error {
	path, err := p.path(dbName)
	if err != nil {
		return err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS _wpmgr_meta (k TEXT PRIMARY KEY, v TEXT)`); err != nil {
		return err
	}
	for k, v := range map[string]string{"db_user": user, "db_host": host} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO _wpmgr_meta(k, v) VALUES(?, ?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`,
			k, v); err != nil {
			return err
		}
	}
	_ = password // not used by SQLite; the per-site Secret still carries it
	return nil
}

// DropDatabase removes the per-site SQLite file.
func (p *Provisioner) DropDatabase(_ context.Context, dbName, _ /*user*/, _ /*host*/ string) error {
	path, err := p.path(dbName)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
