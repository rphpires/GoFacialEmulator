package trace

import (
	"os"
	"path/filepath"
	"testing"
)

// TestInitCreatesLogsFolder garante que o tracer grava em "logs/", que é o
// caminho documentado no MANUAL.md e criado pelos pacotes de instalação.
func TestInitCreatesLogsFolder(t *testing.T) {
	dir := t.TempDir()

	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	tr := &Tracer{}
	if err := tr.init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	t.Cleanup(tr.Close)

	if FOLDER_NAME != "logs" {
		t.Errorf("FOLDER_NAME = %q, quero %q", FOLDER_NAME, "logs")
	}

	logFile := filepath.Join(dir, "logs", "trace.log")
	if _, err := os.Stat(logFile); err != nil {
		t.Errorf("esperava %s criado, erro: %v", logFile, err)
	}

	if _, err := os.Stat(filepath.Join(dir, "traces")); err == nil {
		t.Error("pasta traces/ não deveria mais ser criada")
	}
}
