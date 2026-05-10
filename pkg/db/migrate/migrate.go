package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
)

type Config struct {
	Driver string
	DSN    string
	Dir    string
	FS     fs.FS
}

type StatusInfo struct {
	Total    int
	Applied  int
	Pending  int
	Versions []Migration
}

type Migration struct {
	Version string
	Name    string
	Up      string
	Down    string
	Applied bool
}

func Create(dir, name string) ([]string, error) {
	if dir == "" {
		dir = filepath.Join("db", "migrations")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	migrations, err := diskMigrations(dir)
	if err != nil {
		return nil, err
	}
	next := 1
	for _, m := range migrations {
		if n, err := strconv.Atoi(m.Version); err == nil && n >= next {
			next = n + 1
		}
	}
	clean := slug(name)
	up := filepath.Join(dir, fmt.Sprintf("%03d_%s.up.sql", next, clean))
	down := filepath.Join(dir, fmt.Sprintf("%03d_%s.down.sql", next, clean))
	if err := os.WriteFile(up, []byte("-- up\n"), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(down, []byte("-- down\n"), 0o644); err != nil {
		return nil, err
	}
	return []string{up, down}, nil
}

func Up(dir, dsn string) error { return UpWith(Config{Driver: "sqlite", DSN: dsn, Dir: dir}) }
func Down(dir, dsn string, step int) error {
	return DownWith(Config{Driver: "sqlite", DSN: dsn, Dir: dir}, step)
}
func Status(dir, dsn string) (string, error) {
	info, err := StatusWith(Config{Driver: "sqlite", DSN: dsn, Dir: dir})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d migrations, %d applied, %d pending", info.Total, info.Applied, info.Pending), nil
}

func UpWith(cfg Config) error {
	db, err := open(cfg.Driver, cfg.DSN)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err := ensure(db); err != nil {
		return err
	}
	migrations, err := loadMigrations(cfg)
	if err != nil {
		return err
	}
	for _, m := range migrations {
		if applied(db, m.Version) {
			continue
		}
		if err := execMigration(context.Background(), db, m.Up); err != nil {
			return fmt.Errorf("apply migration %s_%s: %w", m.Version, m.Name, err)
		}
		if _, err = db.Exec("insert into schema_migrations(version, name, applied_at) values(?,?,?)", m.Version, m.Name, time.Now().UTC()); err != nil {
			return err
		}
	}
	return nil
}

func DownWith(cfg Config, step int) error {
	if step <= 0 {
		step = 1
	}
	db, err := open(cfg.Driver, cfg.DSN)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err := ensure(db); err != nil {
		return err
	}
	migrations, err := loadMigrations(cfg)
	if err != nil {
		return err
	}
	byVersion := map[string]Migration{}
	for _, m := range migrations {
		byVersion[m.Version] = m
	}
	versions, err := appliedVersions(db, step)
	if err != nil {
		return err
	}
	for _, v := range versions {
		m, ok := byVersion[v]
		if !ok || strings.TrimSpace(m.Down) == "" {
			continue
		}
		if err := execMigration(context.Background(), db, m.Down); err != nil {
			return fmt.Errorf("rollback migration %s: %w", v, err)
		}
		if _, err = db.Exec("delete from schema_migrations where version=?", v); err != nil {
			return err
		}
	}
	return nil
}

func StatusWith(cfg Config) (StatusInfo, error) {
	db, err := open(cfg.Driver, cfg.DSN)
	if err != nil {
		return StatusInfo{}, err
	}
	defer func() { _ = db.Close() }()
	if err := ensure(db); err != nil {
		return StatusInfo{}, err
	}
	migrations, err := loadMigrations(cfg)
	if err != nil {
		return StatusInfo{}, err
	}
	appliedSet, err := appliedSet(db)
	if err != nil {
		return StatusInfo{}, err
	}
	info := StatusInfo{Total: len(migrations)}
	for _, m := range migrations {
		m.Applied = appliedSet[m.Version]
		if m.Applied {
			info.Applied++
		}
		info.Versions = append(info.Versions, m)
	}
	info.Pending = info.Total - info.Applied
	return info, nil
}

func open(driver, dsn string) (*sql.DB, error) {
	if driver == "" {
		driver = "sqlite"
	}
	if dsn == "" && (driver == "sqlite" || driver == "sqlite3") {
		dsn = "goflex.db"
	}
	switch driver {
	case "sqlite", "sqlite3":
		return sql.Open("sqlite3", dsn)
	case "postgres", "postgresql":
		return sql.Open("pgx", dsn)
	case "mysql":
		return sql.Open("mysql", dsn)
	default:
		return nil, fmt.Errorf("unsupported driver %s", driver)
	}
}

func ensure(db *sql.DB) error {
	_, err := db.Exec("create table if not exists schema_migrations(version text primary key, name text, applied_at timestamp)")
	if err != nil {
		return err
	}
	// Older bootstrap versions only had version/applied_at. Ignore errors when the
	// column already exists or the dialect does not support this form.
	_, _ = db.Exec("alter table schema_migrations add column name text")
	return nil
}

func execMigration(ctx context.Context, db *sql.DB, sqlText string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if strings.TrimSpace(sqlText) != "" {
		if _, err = tx.Exec(sqlText); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func loadMigrations(cfg Config) ([]Migration, error) {
	if cfg.FS != nil {
		return fsMigrations(cfg.FS)
	}
	return diskMigrations(cfg.Dir)
}

func diskMigrations(dir string) ([]Migration, error) {
	if dir == "" {
		dir = filepath.Join("db", "migrations")
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		return nil, err
	}
	migrations := make([]Migration, 0, len(files))
	for _, up := range files {
		m, ok := parseMigrationName(filepath.Base(up))
		if !ok {
			continue
		}
		upBytes, err := os.ReadFile(up)
		if err != nil {
			return nil, err
		}
		downPath := filepath.Join(dir, fmt.Sprintf("%s_%s.down.sql", m.Version, m.Name))
		downBytes, err := os.ReadFile(downPath)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		m.Up = string(upBytes)
		m.Down = string(downBytes)
		migrations = append(migrations, m)
	}
	sortMigrations(migrations)
	return migrations, nil
}

func fsMigrations(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.Glob(fsys, "*.up.sql")
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		// Common when passing an embed.FS rooted at the app instead of at the migrations dir.
		entries, err = fs.Glob(fsys, "**/*.up.sql")
		if err != nil {
			return nil, err
		}
	}
	migrations := make([]Migration, 0, len(entries))
	for _, up := range entries {
		m, ok := parseMigrationName(path.Base(up))
		if !ok {
			continue
		}
		upBytes, err := fs.ReadFile(fsys, up)
		if err != nil {
			return nil, err
		}
		downFile := path.Join(path.Dir(up), fmt.Sprintf("%s_%s.down.sql", m.Version, m.Name))
		downBytes, err := fs.ReadFile(fsys, downFile)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		m.Up = string(upBytes)
		m.Down = string(downBytes)
		migrations = append(migrations, m)
	}
	sortMigrations(migrations)
	return migrations, nil
}

var migrationNameRE = regexp.MustCompile(`^(\d+)_([a-z0-9_]+)\.(up|down)\.sql$`)

func parseMigrationName(name string) (Migration, bool) {
	parts := migrationNameRE.FindStringSubmatch(name)
	if len(parts) != 4 {
		return Migration{}, false
	}
	return Migration{Version: parts[1], Name: parts[2]}, true
}

func slug(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "migration"
	}
	return out
}

func sortMigrations(migrations []Migration) {
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
}

func applied(db *sql.DB, v string) bool {
	var x string
	return db.QueryRow("select version from schema_migrations where version=?", v).Scan(&x) == nil
}

func appliedSet(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query("select version from schema_migrations")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

func appliedVersions(db *sql.DB, step int) ([]string, error) {
	rows, err := db.Query("select version from schema_migrations order by version desc limit ?", step)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var versions []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}
