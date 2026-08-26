package assets

import (
	"embed"
	"io/fs"
)

//go:embed all:web all:migrations
var root embed.FS

// Templates retorna o FS com todos os templates em "web/templates/*.html".
func Templates() fs.FS { return root }

// StaticFS retorna o subsistema "web/static" para servir via http.FS.
func StaticFS() fs.FS {
	sub, err := fs.Sub(root, "web/static")
	if err != nil {
		panic(err)
	}
	return sub
}

// MigrationSQL retorna o conteudo embutido da migration V001.
func MigrationSQL() ([]byte, error) {
	return root.ReadFile("migrations/V001_create_emulator_schema.sql")
}

// MigrationFiles devolve o subsistema "migrations" para o runner de
// migração (internal/database/migrate.go). MigrationSQL continua servindo
// o validator, que só conhece o baseline.
func MigrationFiles() (fs.FS, error) {
	return fs.Sub(root, "migrations")
}
