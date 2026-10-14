package devicelog

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func optsDeTeste(t *testing.T) Options {
	t.Helper()
	return Options{Dir: t.TempDir(), MaxSize: 1 << 20, MaxFiles: 8}
}

func infoDeTeste() Info {
	return Info{ID: 1, Name: "Portaria A", Type: "Hikvision", Port: 4001}
}

func ler(t *testing.T, caminho string) string {
	t.Helper()
	b, err := os.ReadFile(caminho)
	if err != nil {
		t.Fatalf("lendo %s: %v", caminho, err)
	}
	return string(b)
}

func TestDesligadoNaoCriaArquivo(t *testing.T) {
	o := optsDeTeste(t)
	l := New(o, infoDeTeste(), nil)
	l.Printf("IN", "nada")
	if fs := o.Files(1); len(fs) != 0 {
		t.Fatalf("desligado criou arquivos: %v", fs)
	}
}

func TestLigarGravaCabecalhoELinha(t *testing.T) {
	o := optsDeTeste(t)
	l := New(o, infoDeTeste(), nil)
	if err := l.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	l.Printf("IN", "ola %d", 42)

	got := ler(t, o.FilePath(1, 0))
	if !strings.HasPrefix(got, `# device 1 "Portaria A" Hikvision porta 4001 — aberto `) {
		t.Errorf("cabeçalho errado:\n%s", got)
	}
	linha := regexp.MustCompile(`(?m)^\d{4}-\d\d-\d\d \d\d:\d\d:\d\d\.\d{3} IN ola 42$`)
	if !linha.MatchString(got) {
		t.Errorf("linha IN ausente:\n%s", got)
	}
	if !strings.Contains(got, " LOG ligado") {
		t.Errorf("linha LOG ligado ausente:\n%s", got)
	}
}

func TestLigarDuasVezesNaoDuplicaLinhaLog(t *testing.T) {
	o := optsDeTeste(t)
	l := New(o, infoDeTeste(), nil)
	_ = l.SetEnabled(true)
	_ = l.SetEnabled(true)
	if n := strings.Count(ler(t, o.FilePath(1, 0)), "LOG ligado"); n != 1 {
		t.Errorf("LOG ligado %d vezes, quero 1", n)
	}
}

func TestRotacaoMantemNoMaximoMaxFiles(t *testing.T) {
	o := Options{Dir: t.TempDir(), MaxSize: 300, MaxFiles: 3}
	l := New(o, infoDeTeste(), nil)
	_ = l.SetEnabled(true)
	for i := 0; i < 60; i++ {
		l.Printf("IN", "linha %03d %s", i, strings.Repeat("x", 30))
	}

	fs := o.Files(1)
	if len(fs) != 3 {
		t.Fatalf("arquivos = %v, quero 3", fs)
	}
	if _, err := os.Stat(o.FilePath(1, 3)); !os.IsNotExist(err) {
		t.Errorf("device-1.3.log não deveria existir")
	}
	for _, f := range fs {
		if !strings.HasPrefix(ler(t, f), "# device 1 ") {
			t.Errorf("%s sem cabeçalho", f)
		}
	}
	// O ativo tem a última linha; o .1 tem linhas mais antigas que o ativo.
	if !strings.Contains(ler(t, o.FilePath(1, 0)), "linha 059") {
		t.Errorf("ativo sem a última linha")
	}
	if strings.Contains(ler(t, o.FilePath(1, 1)), "linha 059") {
		t.Errorf(".1 não pode ter a última linha")
	}
}

func TestDesligarApagaArquivos(t *testing.T) {
	o := Options{Dir: t.TempDir(), MaxSize: 300, MaxFiles: 3}
	l := New(o, infoDeTeste(), nil)
	_ = l.SetEnabled(true)
	for i := 0; i < 30; i++ {
		l.Printf("IN", "linha %s", strings.Repeat("y", 40))
	}
	if err := l.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	if fs := o.Files(1); len(fs) != 0 {
		t.Fatalf("sobrou: %v", fs)
	}
	l.Printf("IN", "depois de desligar")
	if fs := o.Files(1); len(fs) != 0 {
		t.Fatalf("gravou depois de desligar: %v", fs)
	}
}

func TestNilLoggerNaoEntraEmPanico(t *testing.T) {
	var l *Logger
	l.Printf("IN", "x")
	_ = l.SetEnabled(true)
	l.SetInfo(Info{})
	if l.Enabled() {
		t.Error("nil não pode estar ligado")
	}
}

func TestConcorrenciaSemLinhaIntercalada(t *testing.T) {
	o := optsDeTeste(t)
	l := New(o, infoDeTeste(), nil)
	_ = l.SetEnabled(true)

	var wg sync.WaitGroup
	for g := 0; g < 20; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				l.Printf("IN", "g%02d-%03d %s", g, i, strings.Repeat("z", 50))
			}
		}(g)
	}
	wg.Wait()

	ok := regexp.MustCompile(`^(# device .*|\d{4}-\d\d-\d\d \d\d:\d\d:\d\d\.\d{3} (LOG ligado|IN g\d\d-\d{3} z{50}))$`)
	linhas := strings.Split(strings.TrimRight(ler(t, o.FilePath(1, 0)), "\n"), "\n")
	if len(linhas) != 1+1+20*50 {
		t.Errorf("%d linhas, quero %d", len(linhas), 1+1+20*50)
	}
	for _, ln := range linhas {
		if !ok.MatchString(ln) {
			t.Fatalf("linha corrompida: %q", ln)
		}
	}
}

func TestFalhaAoAbrirAvisaUmaVez(t *testing.T) {
	base := t.TempDir()
	arquivo := filepath.Join(base, "nao-e-pasta")
	if err := os.WriteFile(arquivo, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	erros := 0
	l := New(Options{Dir: filepath.Join(arquivo, "devices"), MaxSize: 1 << 20, MaxFiles: 8},
		infoDeTeste(), func(error) { erros++ })
	_ = l.SetEnabled(true)
	l.Printf("IN", "a")
	l.Printf("IN", "b")
	if erros != 1 {
		t.Errorf("onError chamado %d vezes, quero 1", erros)
	}
}

func TestWriteZipTemTodosOsArquivos(t *testing.T) {
	o := Options{Dir: t.TempDir(), MaxSize: 300, MaxFiles: 3}
	l := New(o, infoDeTeste(), nil)
	_ = l.SetEnabled(true)
	for i := 0; i < 30; i++ {
		l.Printf("IN", "linha %s", strings.Repeat("w", 40))
	}

	var buf bytes.Buffer
	if err := l.WriteZip(&buf); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var nomes []string
	for _, f := range zr.File {
		nomes = append(nomes, f.Name)
	}
	if got := fmt.Sprint(nomes); got != "[device-1.log device-1.1.log device-1.2.log]" {
		t.Errorf("zip = %s", got)
	}
}

func TestWriteZipSemArquivos(t *testing.T) {
	l := New(optsDeTeste(t), infoDeTeste(), nil)
	if err := l.WriteZip(&bytes.Buffer{}); err != ErrNoLogs {
		t.Errorf("err = %v, quero ErrNoLogs", err)
	}
}
