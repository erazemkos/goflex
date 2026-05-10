package migrate

import (
	"database/sql"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

//go:embed testdata/embed/*.sql
var embedded embed.FS

func TestCreateSequentialMigrationFiles(t *testing.T) {
	dir := t.TempDir()
	files, err := Create(dir, "Add Todos")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || filepath.Base(files[0]) != "001_add_todos.up.sql" || filepath.Base(files[1]) != "001_add_todos.down.sql" {
		t.Fatalf("files=%v", files)
	}
	files, err = Create(dir, "Add Todos")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(files[0]) != "002_add_todos.up.sql" {
		t.Fatalf("second files=%v", files)
	}
}

func TestApplyRollbackStatusAndIdempotency(t *testing.T) {
	dir := t.TempDir()
	writeMigration(t, dir, "001_create_users", "create table users (id integer primary key, email text);", "drop table users;")
	writeMigration(t, dir, "002_create_todos", "create table todos (id integer primary key, title text);", "drop table todos;")
	dsn := filepath.Join(t.TempDir(), "app.db")
	if err := Up(dir, dsn); err != nil {
		t.Fatal(err)
	}
	if err := Up(dir, dsn); err != nil {
		t.Fatalf("idempotent up failed: %v", err)
	}
	info, err := StatusWith(Config{Driver: "sqlite", DSN: dsn, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if info.Total != 2 || info.Applied != 2 || info.Pending != 0 {
		t.Fatalf("status=%+v", info)
	}
	status, err := Status(dir, dsn)
	if err != nil || !strings.Contains(status, "2 applied") {
		t.Fatalf("status string=%q err=%v", status, err)
	}
	assertTable(t, dsn, "users", true)
	assertTable(t, dsn, "todos", true)

	if err := Down(dir, dsn, 1); err != nil {
		t.Fatal(err)
	}
	assertTable(t, dsn, "users", true)
	assertTable(t, dsn, "todos", false)
	info, err = StatusWith(Config{Driver: "sqlite", DSN: dsn, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if info.Applied != 1 || info.Pending != 1 {
		t.Fatalf("after rollback status=%+v", info)
	}
}

func TestEmbeddedMigrations(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "embedded.db")
	sub, err := fsSub(embedded, "testdata/embed")
	if err != nil {
		t.Fatal(err)
	}
	cwd := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(old) }()
	if err := UpWith(Config{Driver: "sqlite", DSN: dsn, FS: sub}); err != nil {
		t.Fatal(err)
	}
	assertTable(t, dsn, "widgets", true)
	assertTable(t, dsn, "gadgets", true)
	if err := DownWith(Config{Driver: "sqlite", DSN: dsn, FS: sub}, 2); err != nil {
		t.Fatal(err)
	}
	assertTable(t, dsn, "widgets", false)
	assertTable(t, dsn, "gadgets", false)
}

func TestUnsupportedDriver(t *testing.T) {
	if err := UpWith(Config{Driver: "oracle", DSN: "x"}); err == nil {
		t.Fatal("want unsupported driver error")
	}
}

func writeMigration(t *testing.T, dir, name, up, down string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name+".up.sql"), []byte(up), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".down.sql"), []byte(down), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertTable(t *testing.T, dsn, table string, exists bool) {
	t.Helper()
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var name string
	err = db.QueryRow("select name from sqlite_master where type='table' and name=?", table).Scan(&name)
	if exists && err != nil {
		t.Fatalf("expected table %s: %v", table, err)
	}
	if !exists && err == nil {
		t.Fatalf("did not expect table %s", table)
	}
}

func fsSub(fsys fs.FS, dir string) (fs.FS, error) { return fs.Sub(fsys, dir) }
