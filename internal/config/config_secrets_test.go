package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// segredosProibidos são credenciais reais do W_Access que já vazaram para
// arquivos versionados. Nenhum arquivo distribuído pode contê-las.
var segredosProibidos = []string{
	"db_W-X-S@Wellcare924_",
	"172.16.17.67",
	"172.20.112.1",
}

var arquivosVersionados = []string{
	"../../configs/config.yaml",
	"../../configs/config.local.yaml",
	"../../assets/migrations/V001_create_emulator_schema.sql",
}

func TestArquivosVersionadosNaoContemCredenciais(t *testing.T) {
	for _, arquivo := range arquivosVersionados {
		conteudo, err := os.ReadFile(arquivo)
		if err != nil {
			t.Fatalf("lendo %s: %v", arquivo, err)
		}
		for _, segredo := range segredosProibidos {
			if strings.Contains(string(conteudo), segredo) {
				t.Errorf("%s contém credencial proibida %q", filepath.Base(arquivo), segredo)
			}
		}
	}
}

func TestConfigYamlUsaPorta7070ELocalhost(t *testing.T) {
	cfg, err := Load("../../configs/config.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 7070 {
		t.Errorf("server.port = %d, quero 7070", cfg.Server.Port)
	}
	if cfg.ServiceDB.Host != "127.0.0.1" {
		t.Errorf("service_db.host = %q, quero \"127.0.0.1\"", cfg.ServiceDB.Host)
	}
}
