package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// segredosProibidos são credenciais reais do W_Access que já vazaram para
// arquivos versionados. Nenhum arquivo do repositório pode contê-las.
var segredosProibidos = []string{
	"db_W-X-S@Wellcare924_",
	"172.16.17.67",
	"172.20.112.1",
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

// TestRepositorioNaoContemCredenciaisWXS é o guarda permanente: percorre todo
// arquivo rastreado pelo git (não só os três que já vazaram) e falha se
// qualquer um contiver uma credencial real do W_Access. Isso transforma a
// limpeza pontual desta task em uma proteção contínua contra reincidência.
//
// Exceções propositais:
//   - este próprio arquivo, que precisa citar os segredos para procurá-los;
//   - docs/superpowers/, onde o spec e o plano descrevem o defeito original
//     e citam os valores antigos como parte do registro histórico.
func TestRepositorioNaoContemCredenciaisWXS(t *testing.T) {
	raiz := "../.."

	saida, err := exec.Command("git", "-C", raiz, "ls-files").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}

	const arquivoDoTeste = "internal/config/config_secrets_test.go"
	const prefixoDocsHistorico = "docs/superpowers/"

	for _, linha := range strings.Split(strings.TrimRight(string(saida), "\n"), "\n") {
		if linha == "" || linha == arquivoDoTeste || strings.HasPrefix(linha, prefixoDocsHistorico) {
			continue
		}

		conteudo, err := os.ReadFile(filepath.Join(raiz, linha))
		if err != nil {
			// Arquivo pode ter sumido entre o ls-files e a leitura, ou ser
			// binário ilegível; não é o que este teste audita.
			continue
		}

		for _, segredo := range segredosProibidos {
			if strings.Contains(string(conteudo), segredo) {
				t.Errorf("%s contém credencial proibida %q", linha, segredo)
			}
		}
	}
}
