# Logs por emulador — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fazer o checkbox "Gravar log de eventos" gravar, por emulador, um arquivo texto com conexões, requisições recebidas (origem + objetos) e eventos enviados ao gerenciador, com link para ver o log e baixar zip na gaveta de detalhes.

**Architecture:** Pacote novo `internal/devicelog` (logger com rotação própria, sanitização de corpos, middleware gin, hook `ConnState`, registros de stream e de saída). Um `devicelog.Registry` vive no `emulator.Manager`; cada emulador recebe seu `*devicelog.Logger` (nil-safe). Dois endpoints novos em `/api/devices/:id/` servem o arquivo ativo e o zip. A UI ganha os links e libera o checkbox com o emulador rodando.

**Tech Stack:** Go 1.21 (stdlib: `archive/zip`, `mime/multipart`, `net/http`, `regexp`, `sync/atomic`), gin v1.10, JS vanilla nos templates em `assets/web`.

**Spec:** `docs/superpowers/specs/2026-10-14-logs-por-emulador-design.md`

## Global Constraints

- Pasta dos arquivos: `logs/devices/` (relativa ao diretório de trabalho do serviço, mesma raiz do `trace.log`).
- Nomes: ativo `device-<id>.log`; rotacionados `device-<id>.1.log` … `device-<id>.7.log` (`.1` o mais novo).
- Rotação: 3MB por arquivo (`3 << 20`), no máximo 8 arquivos por emulador.
- Cada linha começa com `AAAA-MM-DD HH:MM:SS.mmm TIPO`; tipos: `CONN`, `IN`, `OUT`, `CONECTADO`, `DESCONECTADO`, `START`, `STOP`, `LOG`.
- Corpos truncados em 2KB (`2048` bytes) após sanitização; base64 com 257+ caracteres vira `<base64 NKB>`; partes binárias viram `<tipo/mime NKB>`; `password`/`pwd` viram `***`.
- Corpo de requisição acima de 2MB não é copiado: registra só o tamanho.
- Campos-chave: no máximo 6 por linha, valor cortado em 40 caracteres.
- Desligar o log apaga os arquivos do emulador. Excluir o emulador também.
- O log nunca altera a resposta ao gerenciador nem derruba o emulador.
- Sem dependências novas no `go.mod`.
- O `trace.log` global e os `[REQ]/[RES]` atuais ficam como estão.
- Texto de ajuda da UI: `Últimos 8 arquivos de 3MB. Desligar apaga os arquivos.`
- Confirmação ao desligar: `Desligar apaga os logs deste emulador. Continuar?`

## Review Focus

1. **Desligar com conexão/stream aberto** — o stream do gerenciador continua; nenhuma linha nova aparece; os arquivos somem. Coberto em Task 3 (`TestDesligarComStreamAbertoNaoGrava`).
2. **Corpo grande / upload de face** — handler recebe o corpo inteiro e intacto mesmo acima de 2MB; o log mostra só o tamanho. Coberto em Task 3 (`TestGinCorpoGrandeChegaInteiroAoHandler`).
3. **ID de outro dispositivo com prefixo comum** (`device-1` vs `device-12`) — apagar/zipar o 1 não toca nos arquivos do 12. Coberto em Task 4 (`TestRemoveNaoTocaEmIDComMesmoPrefixo`).
4. **Ligar o log de um emulador parado e nunca iniciado** — o arquivo nasce com cabeçalho completo (nome, tipo, porta) vindo do banco. Coberto em Task 4 (`TestUpdateDeviceSettingsLigaComCabecalhoDoBanco`).
5. **Pasta de logs sem permissão / disco cheio** — `onError` chamado uma vez, sem pânico, chamadas seguintes viram no-op. Coberto em Task 1 (`TestFalhaAoAbrirAvisaUmaVez`).

---

## File Structure

| Arquivo | Responsabilidade |
|---|---|
| `internal/devicelog/logger.go` (novo) | `Options`, `Info`, `Logger`: arquivo, rotação, liga/desliga, `Printf`, zip |
| `internal/devicelog/sanitize.go` (novo) | `Describe`, `KeyFields`, `UserFromAuth`, `humanSize` |
| `internal/devicelog/capture.go` (novo) | `Gin()` middleware, `MarkStream`, `ConnState`, `StreamOpen/Event/Close`, `Out` |
| `internal/devicelog/registry.go` (novo) | `Registry`: um logger por dispositivo, remoção, zip, caminho ativo |
| `internal/emulator/manager.go` | campo `Logs`, `deviceLog`, `logInfo`, `UpdateDeviceSettings`, `createEmulator` |
| `internal/emulator/crud.go` | `DeleteDevice` apaga os logs |
| `internal/emulator/hikvision/emulator.go`, `handlers.go` | `dlog`, `newRouter`, `ConnState`, START/STOP, `postEvent`, stream |
| `internal/emulator/dahua/emulator.go`, `handlers.go` | `dlog`, `newRouter`, `ConnState`, START/STOP, `postNotification`, keepalive, streams |
| `internal/handlers/device_log.go` (novo) | `getDeviceLog`, `getDeviceLogZip` |
| `internal/handlers/handlers.go` | campo `logs`, rotas |
| `assets/web/templates/devices.html`, `static/js/device-drawer.js`, `static/js/devices.js`, `static/css/components.css` | links, confirmação, checkbox liberado |
| `docs/manual/conteudo/comum/logs.md` | seção do log por dispositivo |

---

### Task 1: devicelog — logger com rotação

**Files:**
- Create: `internal/devicelog/logger.go`
- Test: `internal/devicelog/logger_test.go`

**Interfaces:**
- Produces:
  - `type Options struct { Dir string; MaxSize int64; MaxFiles int }`
  - `func DefaultOptions() Options`
  - `func (o Options) FilePath(id, n int) string`
  - `func (o Options) Files(id int) []string`
  - `type Info struct { ID int; Name, Type string; Port int }`
  - `func New(opts Options, info Info, onError func(error)) *Logger`
  - `func (l *Logger) Enabled() bool`, `SetEnabled(on bool) error`, `SetInfo(info Info)`, `Printf(kind, format string, args ...interface{})`, `WriteZip(w io.Writer) error`
  - `var ErrNoLogs error`
  - internos usados pelas Tasks 3 e 4: `l.mu`, `l.writeLocked(kind, msg string)`, `l.now`, `l.conns map[string]*connInfo`, `l.streams map[string]*streamInfo`, `(o Options).removeAll(id int) error`, `writeZip(w io.Writer, files []string) error`
- Todos os métodos aceitam receptor `nil` sem pânico (viram no-op).

- [ ] **Step 1: Write the failing tests**

`internal/devicelog/logger_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/devicelog/ -run . -v`
Expected: FAIL — `undefined: Options`, `undefined: New` etc.

- [ ] **Step 3: Write the implementation**

`internal/devicelog/logger.go`:

```go
// Package devicelog grava o log de um emulador específico: conexões,
// requisições recebidas e eventos enviados ao gerenciador. Liga e desliga
// pela flag log_enabled do dispositivo; desligar apaga os arquivos.
//
// Todo método aceita receptor nil e vira no-op, para os emuladores não
// precisarem checar se receberam um logger.
package devicelog

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// ErrNoLogs indica que o dispositivo não tem nenhum arquivo de log no disco.
var ErrNoLogs = errors.New("devicelog: nenhum arquivo de log")

// Options define onde e quanto guardar.
type Options struct {
	Dir      string // pasta dos arquivos
	MaxSize  int64  // tamanho de um arquivo antes de rotacionar
	MaxFiles int    // ativo + rotacionados
}

// DefaultOptions: logs/devices, 8 arquivos de 3MB.
func DefaultOptions() Options {
	return Options{Dir: filepath.Join("logs", "devices"), MaxSize: 3 << 20, MaxFiles: 8}
}

// FilePath devolve o n-ésimo arquivo do dispositivo: 0 é o ativo, 1 o
// rotacionado mais novo.
func (o Options) FilePath(id, n int) string {
	if n == 0 {
		return filepath.Join(o.Dir, fmt.Sprintf("device-%d.log", id))
	}
	return filepath.Join(o.Dir, fmt.Sprintf("device-%d.%d.log", id, n))
}

// Files devolve os arquivos existentes do dispositivo, do mais novo ao mais
// velho.
func (o Options) Files(id int) []string {
	var out []string
	for n := 0; n < o.MaxFiles; n++ {
		p := o.FilePath(id, n)
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func (o Options) removeAll(id int) error {
	var primeiro error
	for n := 0; n < o.MaxFiles; n++ {
		err := os.Remove(o.FilePath(id, n))
		if err != nil && !errors.Is(err, os.ErrNotExist) && primeiro == nil {
			primeiro = err
		}
	}
	return primeiro
}

// Info identifica o dispositivo no cabeçalho de cada arquivo.
type Info struct {
	ID   int
	Name string
	Type string
	Port int
}

// Logger é o log de um dispositivo.
type Logger struct {
	opts    Options
	onError func(error)
	now     func() time.Time
	enabled atomic.Bool

	mu                sync.Mutex
	info              Info
	file              *os.File
	size              int64
	failed            bool
	rotateErrReported bool
	conns             map[string]*connInfo
	streams           map[string]*streamInfo
}

// New cria o logger desligado. onError recebe falhas de disco (pode ser nil).
func New(opts Options, info Info, onError func(error)) *Logger {
	if opts.MaxFiles < 2 {
		opts.MaxFiles = 2
	}
	return &Logger{
		opts:    opts,
		info:    info,
		onError: onError,
		now:     time.Now,
		conns:   map[string]*connInfo{},
		streams: map[string]*streamInfo{},
	}
}

// Enabled diz se o log está ligado.
func (l *Logger) Enabled() bool {
	return l != nil && l.enabled.Load()
}

// SetInfo atualiza nome/porta para o próximo cabeçalho.
func (l *Logger) SetInfo(info Info) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.info = info
	l.mu.Unlock()
}

// SetEnabled liga ou desliga. Ligar de novo depois de uma falha de disco
// tenta outra vez. Desligar fecha e apaga todos os arquivos.
func (l *Logger) SetEnabled(on bool) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if on {
		if l.enabled.Load() && !l.failed {
			return nil
		}
		l.failed = false
		l.enabled.Store(true)
		l.writeLocked("LOG", "ligado")
		return nil
	}

	l.enabled.Store(false)
	l.closeLocked()
	l.failed = false
	l.conns = map[string]*connInfo{}
	l.streams = map[string]*streamInfo{}
	return l.opts.removeAll(l.info.ID)
}

// Printf grava uma linha "data hora TIPO mensagem".
func (l *Logger) Printf(kind, format string, args ...interface{}) {
	if !l.Enabled() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.writeLocked(kind, fmt.Sprintf(format, args...))
}

// writeLocked exige l.mu. Re-checa enabled porque um SetEnabled(false) pode
// ter rodado entre o Enabled() do chamador e o lock.
func (l *Logger) writeLocked(kind, msg string) {
	if !l.enabled.Load() || l.failed {
		return
	}
	line := l.now().Format("2006-01-02 15:04:05.000") + " " + kind + " " + msg + "\n"

	if l.file == nil {
		if err := l.openLocked(); err != nil {
			l.fail(err)
			return
		}
	}
	if l.size > 0 && l.size+int64(len(line)) > l.opts.MaxSize {
		if err := l.rotateLocked(); err != nil {
			if l.file == nil {
				l.fail(err)
				return
			}
			// Continua no arquivo atual; tenta de novo na próxima linha.
			if !l.rotateErrReported {
				l.rotateErrReported = true
				l.report(fmt.Errorf("rotação: %w", err))
			}
		} else {
			l.rotateErrReported = false
		}
	}

	n, err := l.file.WriteString(line)
	l.size += int64(n)
	if err != nil {
		l.fail(err)
	}
}

func (l *Logger) openLocked() error {
	if err := os.MkdirAll(l.opts.Dir, 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(l.opts.FilePath(l.info.ID, 0), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	l.file, l.size = f, st.Size()
	if l.size == 0 {
		n, err := fmt.Fprintf(f, "# device %d %q %s porta %d — aberto %s\n",
			l.info.ID, l.info.Name, l.info.Type, l.info.Port,
			l.now().Format("2006-01-02 15:04:05"))
		l.size += int64(n)
		if err != nil {
			return err
		}
	}
	return nil
}

// rotateLocked apaga o mais velho, desloca .n → .n+1 e abre um ativo novo.
func (l *Logger) rotateLocked() error {
	l.closeLocked()
	id, ultimo := l.info.ID, l.opts.MaxFiles-1

	if err := os.Remove(l.opts.FilePath(id, ultimo)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return l.reopenAfter(err)
	}
	for n := ultimo - 1; n >= 0; n-- {
		err := os.Rename(l.opts.FilePath(id, n), l.opts.FilePath(id, n+1))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return l.reopenAfter(err)
		}
	}
	return l.openLocked()
}

// reopenAfter reabre o ativo depois de uma rotação que falhou, para continuar
// gravando nele.
func (l *Logger) reopenAfter(cause error) error {
	if err := l.openLocked(); err != nil {
		return err
	}
	return cause
}

func (l *Logger) closeLocked() {
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
	l.size = 0
}

// fail desiste até o próximo SetEnabled(true): uma falha de disco não pode
// virar uma linha de erro no trace a cada requisição.
func (l *Logger) fail(err error) {
	l.failed = true
	l.closeLocked()
	l.report(err)
}

func (l *Logger) report(err error) {
	if l.onError != nil {
		l.onError(fmt.Errorf("log do dispositivo %d: %w", l.info.ID, err))
	}
}

// WriteZip escreve um zip com todos os arquivos do dispositivo. Segura o
// lock para não pegar uma rotação no meio.
func (l *Logger) WriteZip(w io.Writer) error {
	if l == nil {
		return ErrNoLogs
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Sync()
	}
	return writeZip(w, l.opts.Files(l.info.ID))
}

func writeZip(w io.Writer, files []string) error {
	if len(files) == 0 {
		return ErrNoLogs
	}
	zw := zip.NewWriter(w)
	for _, p := range files {
		if err := addToZip(zw, p); err != nil {
			return err
		}
	}
	return zw.Close()
}

func addToZip(zw *zip.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	hdr, err := zip.FileInfoHeader(st)
	if err != nil {
		return err
	}
	hdr.Name = filepath.Base(path)
	hdr.Method = zip.Deflate
	dst, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, f)
	return err
}
```

Os tipos `connInfo` e `streamInfo` são definidos na Task 3. Para esta task compilar, crie já `internal/devicelog/capture.go` só com:

```go
package devicelog

import "time"

type connInfo struct {
	opened time.Time
	reqs   int
}

type streamInfo struct {
	opened time.Time
	path   string
	events int
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race ./internal/devicelog/ -v`
Expected: PASS em todos os testes.

- [ ] **Step 5: Commit**

```bash
git add internal/devicelog/
git commit -m "feat(devicelog): logger por dispositivo com rotação 8x3MB"
```

---

### Task 2: devicelog — sanitização e campos-chave

**Files:**
- Create: `internal/devicelog/sanitize.go`
- Test: `internal/devicelog/sanitize_test.go`

**Interfaces:**
- Produces:
  - `func Describe(contentType string, body []byte) string` — corpo em uma linha, sanitizado, ≤ 2KB + sufixo ` …(cortado)`.
  - `func KeyFields(contentType string, body []byte, query url.Values) string` — `k=v k=v` (máx. 6).
  - `func UserFromAuth(header string) string`
  - `func humanSize(n int) string` (interno)
  - constantes internas `maxBody = 2048`, `maxCopy = 2 << 20`
- Com `contentType` vazio e corpo começando (após `\r\n`/espaços) com `--`, o boundary é deduzido da primeira linha (formato dos streams e do push).

- [ ] **Step 1: Write the failing tests**

`internal/devicelog/sanitize_test.go`:

```go
package devicelog

import (
	"net/url"
	"strings"
	"testing"
)

func TestDescribeTrocaBase64PorTamanho(t *testing.T) {
	foto := strings.Repeat("QUJD", 1000) // 4000 caracteres base64
	got := Describe("application/json", []byte(`{"faceURL":"","img":"`+foto+`"}`))
	if strings.Contains(got, "QUJDQUJD") {
		t.Errorf("base64 não substituído: %s", got)
	}
	if !strings.Contains(got, "<base64 3KB>") {
		t.Errorf("quero <base64 3KB>: %s", got)
	}
}

func TestDescribeMascaraSenha(t *testing.T) {
	casos := map[string]struct{ ct, corpo, quero string }{
		"json": {"application/json", `{"userName":"a","password":"segredo"}`, `"password":"***"`},
		"xml":  {"application/xml", `<User><password>segredo</password></User>`, `<password>***</password>`},
		"form": {"application/x-www-form-urlencoded", `user=a&pwd=segredo`, `pwd=***`},
	}
	for nome, c := range casos {
		got := Describe(c.ct, []byte(c.corpo))
		if strings.Contains(got, "segredo") || !strings.Contains(got, c.quero) {
			t.Errorf("%s: %s", nome, got)
		}
	}
}

func TestDescribeCompactaEmUmaLinha(t *testing.T) {
	got := Describe("application/xml", []byte("<A>\n  <B>1</B>\n</A>\n"))
	if got != "<A><B>1</B></A>" {
		t.Errorf("got %q", got)
	}
}

func TestDescribeTrunca(t *testing.T) {
	got := Describe("text/plain", []byte(strings.Repeat("a ", 3000)))
	if !strings.HasSuffix(got, " …(cortado)") {
		t.Errorf("sem sufixo de corte")
	}
	if len(got) > maxBody+len(" …(cortado)") {
		t.Errorf("tamanho %d", len(got))
	}
}

func TestDescribeBinario(t *testing.T) {
	if got := Describe("image/jpeg", make([]byte, 48*1024)); got != "<image/jpeg 48KB>" {
		t.Errorf("got %q", got)
	}
	if got := Describe("", []byte{0xff, 0xd8, 0xff, 0x00}); got != "<application/octet-stream 4B>" {
		t.Errorf("got %q", got)
	}
}

const multipartPush = "--MIME_boundary\r\n" +
	"Content-Disposition: form-data; name=\"event_log\"\r\n" +
	"Content-Type: application/json\r\n\r\n" +
	`{"eventType":"AccessControllerEvent","AccessControllerEvent":{"majorEventType":5,"subEventType":75,"cardNo":"999","name":"Ana"}}` + "\r\n" +
	"--MIME_boundary\r\n" +
	"Content-Disposition: form-data; name=\"Picture\"; filename=\"Picture.jpg\"\r\n" +
	"Content-Type: image/jpeg\r\n\r\n" +
	"\xff\xd8\xff\x00\r\n" +
	"--MIME_boundary--\r\n"

func TestDescribeMultipart(t *testing.T) {
	got := Describe("multipart/form-data; boundary=MIME_boundary", []byte(multipartPush))
	if !strings.Contains(got, `[event_log] {"eventType":"AccessControllerEvent"`) {
		t.Errorf("parte JSON ausente: %s", got)
	}
	if !strings.Contains(got, "[Picture] <image/jpeg 4B>") {
		t.Errorf("parte imagem ausente: %s", got)
	}
}

func TestKeyFieldsJSONAninhado(t *testing.T) {
	got := KeyFields("application/json",
		[]byte(`{"UserInfo":{"employeeNo":"123","name":"Ana Paula","userType":"normal"}}`), nil)
	if got != `employeeNo=123 name="Ana Paula" userType=normal` {
		t.Errorf("got %q", got)
	}
}

func TestKeyFieldsXMLFormEQuery(t *testing.T) {
	if got := KeyFields("application/xml", []byte(`<CardInfo><employeeNo>7</employeeNo><cardNo>55</cardNo></CardInfo>`), nil); got != "employeeNo=7 cardNo=55" {
		t.Errorf("xml: %q", got)
	}
	if got := KeyFields("application/x-www-form-urlencoded", []byte("CardNo=9&CardName=Bia"), nil); got != "CardName=Bia CardNo=9" {
		t.Errorf("form: %q", got)
	}
	q := url.Values{"action": {"insert"}, "name": {"AccessUserCard"}, "ignorado": {"x"}}
	if got := KeyFields("", nil, q); got != "action=insert name=AccessUserCard" {
		t.Errorf("query: %q", got)
	}
}

func TestKeyFieldsLimites(t *testing.T) {
	// Ordem alfabética (maiúsculas antes): CardName vem primeiro, então o
	// valor longo entra e precisa sair cortado.
	corpo := `{"employeeNo":"1","name":"x","cardNo":"2","UserID":"3","CardName":"` + strings.Repeat("n", 60) + `","major":5,"minor":6,"searchID":"7"}`
	got := KeyFields("application/json", []byte(corpo), nil)
	if n := len(strings.Fields(got)); n != 6 {
		t.Errorf("%d campos, quero 6: %s", n, got)
	}
	if !strings.Contains(got, "CardName="+strings.Repeat("n", 40)+"…") || strings.Contains(got, strings.Repeat("n", 41)) {
		t.Errorf("valor não cortado em 40: %s", got)
	}
}

func TestKeyFieldsMultipartSemContentType(t *testing.T) {
	got := KeyFields("", []byte(multipartPush), nil)
	if got != "eventType=AccessControllerEvent cardNo=999 majorEventType=5 name=Ana subEventType=75" {
		t.Errorf("got %q", got)
	}
	// Part de stream do Dahua: começa com CRLF e não tem boundary final.
	dahua := "\r\n--myboundary\r\nContent-Type: text/plain\r\nContent-Length: 30\r\n\r\n" +
		`{"Events":[{"Code":"AccessControl","Data":{"UserID":"10"}}]}` + "\r\n"
	if got := KeyFields("", []byte(dahua), nil); got != "Code=AccessControl UserID=10" {
		t.Errorf("dahua: %q", got)
	}
}

func TestUserFromAuth(t *testing.T) {
	casos := map[string]string{
		`Digest username="admin", realm="x", nonce="y"`: "admin",
		"Basic YWRtaW46c2VncmVkbw==":                    "admin",
		"":                                              "",
		"Bearer abc":                                    "",
	}
	for h, quero := range casos {
		if got := UserFromAuth(h); got != quero {
			t.Errorf("UserFromAuth(%q) = %q, quero %q", h, got, quero)
		}
	}
}

func TestHumanSize(t *testing.T) {
	casos := map[int]string{0: "0B", 512: "512B", 1024: "1KB", 48 * 1024: "48KB", 3 << 20: "3.0MB"}
	for n, quero := range casos {
		if got := humanSize(n); got != quero {
			t.Errorf("humanSize(%d) = %q, quero %q", n, got, quero)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/devicelog/ -run 'Describe|KeyFields|UserFromAuth|HumanSize' -v`
Expected: FAIL — `undefined: Describe`.

- [ ] **Step 3: Write the implementation**

`internal/devicelog/sanitize.go`:

```go
package devicelog

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	maxBody   = 2048    // corpo gravado, depois de sanitizar
	maxCopy   = 2 << 20 // acima disto o corpo da requisição não é copiado
	maxFields = 6
	maxValue  = 40
	cutSuffix = " …(cortado)"
)

// keyFields são os campos que identificam o objeto da operação nos
// protocolos Hikvision (ISAPI) e Dahua (CGI). Comparação sem maiúsculas.
var keyFields = map[string]bool{
	"employeeno": true, "employeenostring": true, "name": true, "cardno": true,
	"userid": true, "cardname": true, "usertype": true, "major": true,
	"minor": true, "majoreventtype": true, "subeventtype": true, "serialno": true,
	"eventtype": true, "code": true, "searchid": true, "facelibtype": true,
	"fpid": true, "action": true, "recno": true, "status": true,
}

var (
	reBase64     = regexp.MustCompile(`[A-Za-z0-9+/=]{257,}`)
	reXMLPass    = regexp.MustCompile(`(?is)<(password|pwd)>[^<]*</(password|pwd)>`)
	reJSONPass   = regexp.MustCompile(`(?i)"(password|pwd)"\s*:\s*"[^"]*"`)
	reFormPass   = regexp.MustCompile(`(?i)\b(password|pwd)=[^&\s]*`)
	reXMLBetween = regexp.MustCompile(`>\s+<`)
	reSpaces     = regexp.MustCompile(`\s+`)
	reXMLField   = regexp.MustCompile(`<([A-Za-z]\w*)>([^<]{0,200})</([A-Za-z]\w*)>`)
	reDigestUser = regexp.MustCompile(`(?i)username="([^"]*)"`)
)

// Describe devolve o corpo em uma linha, sem imagens, base64 e senhas,
// cortado em maxBody bytes.
func Describe(contentType string, body []byte) string {
	if len(body) == 0 {
		return ""
	}
	mt, params := mediaType(contentType, body)
	var s string
	switch {
	case strings.HasPrefix(mt, "multipart/"):
		s = describeMultipart(body, params["boundary"])
	case isBinary(mt, body):
		if mt == "" {
			mt = "application/octet-stream"
		}
		s = fmt.Sprintf("<%s %s>", mt, humanSize(len(body)))
	default:
		s = cleanText(string(body))
	}
	return truncate(s, maxBody)
}

// mediaType lê o Content-Type ou, sem ele, reconhece um multipart pelo
// "--boundary" da primeira linha (formato dos streams e do push).
func mediaType(contentType string, body []byte) (string, map[string]string) {
	if mt, params, err := mime.ParseMediaType(contentType); err == nil {
		return mt, params
	}
	t := bytes.TrimLeft(body, "\r\n \t")
	if bytes.HasPrefix(t, []byte("--")) {
		linha := t[2:]
		if i := bytes.IndexAny(linha, "\r\n"); i >= 0 {
			linha = linha[:i]
		}
		return "multipart/mixed", map[string]string{"boundary": string(linha)}
	}
	return "", nil
}

func isBinary(mt string, body []byte) bool {
	for _, p := range []string{"image/", "video/", "audio/", "application/octet-stream"} {
		if strings.HasPrefix(mt, p) {
			return true
		}
	}
	return !utf8.Valid(body)
}

// eachPart percorre as partes de um multipart. Tolera stream sem boundary
// final: usa o que conseguiu ler da última parte.
func eachPart(body []byte, boundary string, fn func(name, ct string, data []byte)) bool {
	if boundary == "" {
		return false
	}
	r := multipart.NewReader(bytes.NewReader(bytes.TrimLeft(body, "\r\n \t")), boundary)
	achou := false
	for {
		p, err := r.NextPart()
		if err != nil {
			return achou
		}
		data, _ := io.ReadAll(p)
		achou = true
		fn(p.FormName(), p.Header.Get("Content-Type"), data)
	}
}

func describeMultipart(body []byte, boundary string) string {
	var parts []string
	ok := eachPart(body, boundary, func(name, ct string, data []byte) {
		if name == "" {
			name = "parte"
		}
		parts = append(parts, "["+name+"] "+Describe(ct, data))
	})
	if !ok {
		return fmt.Sprintf("<multipart %s>", humanSize(len(body)))
	}
	return strings.Join(parts, " ")
}

func cleanText(s string) string {
	s = reXMLPass.ReplaceAllString(s, "<$1>***</$2>")
	s = reJSONPass.ReplaceAllString(s, `"$1":"***"`)
	s = reFormPass.ReplaceAllString(s, "$1=***")
	s = reBase64.ReplaceAllStringFunc(s, func(m string) string {
		return "<base64 " + humanSize(len(m)*3/4) + ">"
	})
	s = reXMLBetween.ReplaceAllString(s, "><")
	s = reSpaces.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	corte := max
	for corte > 0 && !utf8.RuneStart(s[corte]) {
		corte--
	}
	return s[:corte] + cutSuffix
}

func humanSize(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1024:
		return fmt.Sprintf("%dKB", (n+512)/1024)
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// KeyFields extrai os campos que identificam o objeto da operação: primeiro
// da query, depois do corpo (JSON, XML, form ou multipart).
func KeyFields(contentType string, body []byte, query url.Values) string {
	c := &collector{seen: map[string]bool{}}
	c.addValues(query)
	c.fromBody(contentType, body)
	return strings.Join(c.out, " ")
}

type collector struct {
	seen map[string]bool
	out  []string
}

func (c *collector) add(k, v string) {
	lk := strings.ToLower(k)
	if !keyFields[lk] || c.seen[lk] || len(c.out) >= maxFields {
		return
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return
	}
	if utf8.RuneCountInString(v) > maxValue {
		v = string([]rune(v)[:maxValue]) + "…"
	}
	if strings.ContainsAny(v, " \"=") {
		v = strconv.Quote(v)
	}
	c.seen[lk] = true
	c.out = append(c.out, k+"="+v)
}

func (c *collector) addValues(v url.Values) {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		c.add(k, v.Get(k))
	}
}

func (c *collector) fromBody(contentType string, body []byte) {
	if len(body) == 0 || len(body) > maxCopy {
		return
	}
	mt, params := mediaType(contentType, body)
	switch {
	case strings.HasPrefix(mt, "multipart/"):
		ok := eachPart(body, params["boundary"], func(_, ct string, data []byte) {
			if !isBinary(ct, data) {
				c.fromBody(ct, data)
			}
		})
		if !ok {
			c.fromLooseJSON(body)
		}
	case mt == "application/x-www-form-urlencoded":
		if v, err := url.ParseQuery(string(body)); err == nil {
			c.addValues(v)
		}
	default:
		t := bytes.TrimSpace(body)
		if len(t) == 0 {
			return
		}
		if t[0] == '{' || t[0] == '[' {
			var v interface{}
			if json.Unmarshal(t, &v) == nil {
				c.walkJSON(v)
			}
			return
		}
		if t[0] == '<' {
			for _, m := range reXMLField.FindAllStringSubmatch(string(t), -1) {
				if m[1] == m[3] {
					c.add(m[1], m[2])
				}
			}
		}
	}
}

// fromLooseJSON é o último recurso para uma part de stream que o leitor de
// multipart não conseguiu abrir: tenta o trecho entre o primeiro '{' e o
// último '}'.
func (c *collector) fromLooseJSON(body []byte) {
	i, j := bytes.IndexByte(body, '{'), bytes.LastIndexByte(body, '}')
	if i < 0 || j <= i {
		return
	}
	var v interface{}
	if json.Unmarshal(body[i:j+1], &v) == nil {
		c.walkJSON(v)
	}
}

// walkJSON visita primeiro os escalares de cada nível (em ordem alfabética),
// depois desce nos objetos e listas.
func (c *collector) walkJSON(v interface{}) {
	switch x := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			switch val := x[k].(type) {
			case string:
				c.add(k, val)
			case float64:
				c.add(k, strconv.FormatFloat(val, 'f', -1, 64))
			case bool:
				c.add(k, strconv.FormatBool(val))
			}
		}
		for _, k := range keys {
			switch x[k].(type) {
			case map[string]interface{}, []interface{}:
				c.walkJSON(x[k])
			}
		}
	case []interface{}:
		for _, item := range x {
			c.walkJSON(item)
		}
	}
}

// UserFromAuth tira só o usuário do header Authorization (Digest ou Basic).
// Nunca devolve senha nem o header inteiro.
func UserFromAuth(header string) string {
	lower := strings.ToLower(header)
	switch {
	case strings.HasPrefix(lower, "digest "):
		if m := reDigestUser.FindStringSubmatch(header); m != nil {
			return m[1]
		}
	case strings.HasPrefix(lower, "basic "):
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(header[6:]))
		if err == nil {
			u, _, _ := strings.Cut(string(raw), ":")
			return u
		}
	}
	return ""
}
```

Nota sobre `TestKeyFieldsMultipartSemContentType`: no objeto de topo, `eventType` é o único escalar listado; depois desce em `AccessControllerEvent`, cujos escalares saem em ordem alfabética (`cardNo`, `majorEventType`, `name`, `subEventType`). No Dahua, `Events[0]` tem `Code`, depois `Data.UserID`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race ./internal/devicelog/ -v`
Expected: PASS em todos.

- [ ] **Step 5: Commit**

```bash
git add internal/devicelog/sanitize.go internal/devicelog/sanitize_test.go
git commit -m "feat(devicelog): sanitização de corpos e extração de campos-chave"
```

---

### Task 3: devicelog — middleware, conexões, streams e saídas

**Files:**
- Modify: `internal/devicelog/capture.go` (substitui o stub da Task 1)
- Test: `internal/devicelog/capture_test.go`

**Interfaces:**
- Consumes: `Logger` internos (`mu`, `writeLocked`, `now`, `conns`, `streams`), `Describe`, `KeyFields`, `UserFromAuth`, `humanSize`, `maxCopy` (Tasks 1–2).
- Produces:
  - `func (l *Logger) Gin() gin.HandlerFunc`
  - `func MarkStream(c *gin.Context)`
  - `func (l *Logger) ConnState(conn net.Conn, state http.ConnState)` — usável como `http.Server.ConnState`.
  - `func (l *Logger) StreamOpen(remote, path string)`
  - `func (l *Logger) StreamEvent(remote, summary string, size int)`
  - `func (l *Logger) StreamClose(remote string)`
  - `func (l *Logger) Out(method, url, contentType string, body []byte, status int, err error, d time.Duration)`
- Formatos de linha (o restante do plano e os testes dependem deles):
  - `IN <remote>[ user=<u>] <MÉTODO> <uri> -> <status> <N>ms (<req>/<resp>)[ | campos]` + `\n    > <corpo req>` + `\n    < <corpo resp>`
  - `CONN <remote> aberta` / `CONN <remote> fechada (<N> reqs, <dur>)` / `CONN <remote> virou stream (<N> reqs, <dur>)`
  - `CONECTADO <remote> <uri>` / `DESCONECTADO <remote> <uri> após <dur>, <N> eventos`
  - `OUT -> <remote> stream <tam>[ | campos]`
  - `OUT -> <MÉTODO> <url> -> <status|erro: msg> <N>ms[ | campos]` + `\n    > <corpo>`

- [ ] **Step 1: Write the failing tests**

`internal/devicelog/capture_test.go`:

```go
package devicelog

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func ligado(t *testing.T) (*Logger, Options) {
	t.Helper()
	o := optsDeTeste(t)
	l := New(o, infoDeTeste(), nil)
	if err := l.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	return l, o
}

func routerDeTeste(l *Logger, handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(l.Gin())
	r.Any("/*rota", handler)
	return r
}

func TestGinRegistraRequisicaoComCorpoECampos(t *testing.T) {
	l, o := ligado(t)
	var recebido string
	r := routerDeTeste(l, func(c *gin.Context) {
		b, _ := io.ReadAll(c.Request.Body)
		recebido = string(b)
		c.JSON(200, gin.H{"statusCode": 1})
	})

	corpo := `{"UserInfo":{"employeeNo":"123","name":"Ana"}}`
	req := httptest.NewRequest("PUT", "/ISAPI/AccessControl/UserInfo/Modify?format=json", strings.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", `Digest username="admin", realm="x"`)
	req.RemoteAddr = "10.0.0.5:51234"
	r.ServeHTTP(httptest.NewRecorder(), req)

	if recebido != corpo {
		t.Fatalf("handler recebeu %q", recebido)
	}
	got := ler(t, o.FilePath(1, 0))
	for _, quero := range []string{
		"IN 10.0.0.5:51234 user=admin PUT /ISAPI/AccessControl/UserInfo/Modify?format=json -> 200 ",
		"| employeeNo=123 name=Ana",
		"\n    > " + corpo,
		"\n    < {\"statusCode\":1}",
	} {
		if !strings.Contains(got, quero) {
			t.Errorf("falta %q em:\n%s", quero, got)
		}
	}
	if strings.Contains(got, "Digest") {
		t.Errorf("header Authorization vazou:\n%s", got)
	}
}

func TestGinCorpoGrandeChegaInteiroAoHandler(t *testing.T) {
	l, o := ligado(t)
	grande := bytes.Repeat([]byte("a"), maxCopy+10)
	var n int
	r := routerDeTeste(l, func(c *gin.Context) {
		b, _ := io.ReadAll(c.Request.Body)
		n = len(b)
		c.Status(200)
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/face", bytes.NewReader(grande)))

	if n != len(grande) {
		t.Fatalf("handler recebeu %d bytes, quero %d", n, len(grande))
	}
	got := ler(t, o.FilePath(1, 0))
	if !strings.Contains(got, "> <corpo 2.0MB não copiado>") {
		t.Errorf("quero só o tamanho:\n%s", got)
	}
	if strings.Contains(got, "aaaa") {
		t.Errorf("corpo grande foi gravado")
	}
}

func TestGinDesligadoNaoLeNemGrava(t *testing.T) {
	o := optsDeTeste(t)
	l := New(o, infoDeTeste(), nil)
	r := routerDeTeste(l, func(c *gin.Context) {
		b, _ := io.ReadAll(c.Request.Body)
		c.String(200, string(b))
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/x", strings.NewReader("abc")))
	if rec.Body.String() != "abc" {
		t.Fatalf("resposta %q", rec.Body.String())
	}
	if fs := o.Files(1); len(fs) != 0 {
		t.Errorf("gravou desligado: %v", fs)
	}
}

func TestGinStreamMarcadoNaoGeraLinhaIN(t *testing.T) {
	l, o := ligado(t)
	r := routerDeTeste(l, func(c *gin.Context) {
		MarkStream(c)
		c.Status(200)
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/alertStream", nil))
	if strings.Contains(ler(t, o.FilePath(1, 0)), " IN ") {
		t.Errorf("stream não pode gerar linha IN")
	}
}

type connFalsa struct {
	net.Conn
	addr string
}

func (c connFalsa) RemoteAddr() net.Addr {
	a, _ := net.ResolveTCPAddr("tcp", c.addr)
	return a
}

func TestConnStateAbreContaEFecha(t *testing.T) {
	l, o := ligado(t)
	c := connFalsa{addr: "10.0.0.5:51234"}
	l.ConnState(c, http.StateNew)
	l.ConnState(c, http.StateActive)
	l.ConnState(c, http.StateIdle)
	l.ConnState(c, http.StateActive)
	l.ConnState(c, http.StateClosed)

	got := ler(t, o.FilePath(1, 0))
	if !strings.Contains(got, "CONN 10.0.0.5:51234 aberta") {
		t.Errorf("sem aberta:\n%s", got)
	}
	if !strings.Contains(got, "CONN 10.0.0.5:51234 fechada (2 reqs, 0s)") {
		t.Errorf("sem fechada:\n%s", got)
	}
}

func TestConnStateHijackedViraStream(t *testing.T) {
	l, o := ligado(t)
	c := connFalsa{addr: "10.0.0.6:6000"}
	l.ConnState(c, http.StateNew)
	l.ConnState(c, http.StateActive)
	l.ConnState(c, http.StateHijacked)
	if !strings.Contains(ler(t, o.FilePath(1, 0)), "CONN 10.0.0.6:6000 virou stream (1 reqs, 0s)") {
		t.Errorf("sem virou stream")
	}
}

func TestStreamAbreEventoFecha(t *testing.T) {
	l, o := ligado(t)
	l.StreamOpen("10.0.0.5:6001", "/ISAPI/Event/notification/alertStream")
	l.StreamEvent("10.0.0.5:6001", "cardNo=999", 2048)
	l.StreamEvent("10.0.0.5:6001", "", 100)
	l.StreamClose("10.0.0.5:6001")

	got := ler(t, o.FilePath(1, 0))
	for _, quero := range []string{
		"CONECTADO 10.0.0.5:6001 /ISAPI/Event/notification/alertStream",
		"OUT -> 10.0.0.5:6001 stream 2KB | cardNo=999",
		"OUT -> 10.0.0.5:6001 stream 100B\n",
		"DESCONECTADO 10.0.0.5:6001 /ISAPI/Event/notification/alertStream após 0s, 2 eventos",
	} {
		if !strings.Contains(got, quero) {
			t.Errorf("falta %q em:\n%s", quero, got)
		}
	}
}

func TestDesligarComStreamAbertoNaoGrava(t *testing.T) {
	l, o := ligado(t)
	l.StreamOpen("10.0.0.5:6001", "/alertStream")
	if err := l.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	l.StreamEvent("10.0.0.5:6001", "x", 10)
	l.StreamClose("10.0.0.5:6001")
	if fs := o.Files(1); len(fs) != 0 {
		t.Errorf("gravou depois de desligar: %v", fs)
	}
}

func TestOutRegistraSucessoEErro(t *testing.T) {
	l, o := ligado(t)
	l.Out("POST", "http://10.0.0.5:8000/w-access", "application/json",
		[]byte(`{"cardNo":"999"}`), 200, nil, 30*time.Millisecond)
	l.Out("GET", "http://10.0.0.5:8000/keepalive", "", nil, 0,
		errors.New("connection refused"), 2*time.Millisecond)

	got := ler(t, o.FilePath(1, 0))
	for _, quero := range []string{
		"OUT -> POST http://10.0.0.5:8000/w-access -> 200 30ms | cardNo=999\n    > {\"cardNo\":\"999\"}",
		"OUT -> GET http://10.0.0.5:8000/keepalive -> erro: connection refused 2ms\n",
	} {
		if !strings.Contains(got, quero) {
			t.Errorf("falta %q em:\n%s", quero, got)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/devicelog/ -run 'Gin|Conn|Stream|Out|Desligar' -v`
Expected: FAIL — `l.Gin undefined`, `undefined: MarkStream`.

- [ ] **Step 3: Write the implementation**

Substitua todo o conteúdo de `internal/devicelog/capture.go`:

```go
package devicelog

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// maxCapture é quanto da resposta guardamos para descrever: o bastante para
// a sanitização reconhecer um base64 longo antes de cortar em maxBody.
const maxCapture = 64 << 10

const streamKey = "devicelog.stream"

type connInfo struct {
	opened time.Time
	reqs   int
}

type streamInfo struct {
	opened time.Time
	path   string
	events int
}

// MarkStream avisa o middleware que o handler assumiu a conexão como
// stream. O próprio handler registra CONECTADO/DESCONECTADO.
func MarkStream(c *gin.Context) {
	c.Set(streamKey, true)
}

type bodyCapture struct {
	gin.ResponseWriter
	buf bytes.Buffer
}

func (w *bodyCapture) keep(b []byte) {
	if room := maxCapture - w.buf.Len(); room > 0 {
		if len(b) > room {
			b = b[:room]
		}
		w.buf.Write(b)
	}
}

func (w *bodyCapture) Write(b []byte) (int, error) {
	w.keep(b)
	return w.ResponseWriter.Write(b)
}

func (w *bodyCapture) WriteString(s string) (int, error) {
	w.keep([]byte(s))
	return w.ResponseWriter.WriteString(s)
}

// restoredBody devolve ao handler o que o middleware já leu, seguido do
// resto do corpo original.
type restoredBody struct {
	io.Reader
	io.Closer
}

// Gin registra cada requisição recebida. Desligado, não lê nada.
func (l *Logger) Gin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !l.Enabled() {
			c.Next()
			return
		}
		inicio := time.Now()
		req := c.Request

		var reqBody []byte
		lidos, grande := 0, false
		if req.Body != nil && req.Body != http.NoBody {
			data, _ := io.ReadAll(io.LimitReader(req.Body, maxCopy+1))
			lidos, grande = len(data), len(data) > maxCopy
			req.Body = restoredBody{io.MultiReader(bytes.NewReader(data), req.Body), req.Body}
			if !grande {
				reqBody = data
			}
		}

		w := &bodyCapture{ResponseWriter: c.Writer}
		c.Writer = w
		c.Next()

		if c.GetBool(streamKey) {
			return
		}

		respSize := w.Size()
		if respSize < 0 {
			respSize = 0
		}
		tamReq := humanSize(lidos)
		if grande {
			tamReq = ">" + humanSize(maxCopy)
		}

		var b strings.Builder
		b.WriteString(req.RemoteAddr)
		if u := UserFromAuth(req.Header.Get("Authorization")); u != "" {
			b.WriteString(" user=" + u)
		}
		fmt.Fprintf(&b, " %s %s -> %d %dms (%s/%s)", req.Method, req.URL.RequestURI(),
			w.Status(), time.Since(inicio).Milliseconds(), tamReq, humanSize(respSize))
		ct := req.Header.Get("Content-Type")
		if f := KeyFields(ct, reqBody, req.URL.Query()); f != "" {
			b.WriteString(" | " + f)
		}
		switch {
		case grande:
			fmt.Fprintf(&b, "\n    > <corpo %s não copiado>", humanSize(maxCopy))
		case len(reqBody) > 0:
			if d := Describe(ct, reqBody); d != "" {
				b.WriteString("\n    > " + d)
			}
		}
		if d := Describe(w.Header().Get("Content-Type"), w.buf.Bytes()); d != "" {
			b.WriteString("\n    < " + d)
		}
		l.Printf("IN", "%s", b.String())
	}
}

func (ci *connInfo) resumo(now time.Time) string {
	if ci == nil {
		return "? reqs"
	}
	s := fmt.Sprintf("%d reqs", ci.reqs)
	if !ci.opened.IsZero() {
		s += ", " + now.Sub(ci.opened).Round(time.Second).String()
	}
	return s
}

// ConnState é o hook de http.Server: uma linha quando a conexão TCP abre e
// outra quando fecha. Dois gerenciadores no mesmo emulador aparecem aqui
// como dois endereços alternando.
func (l *Logger) ConnState(conn net.Conn, state http.ConnState) {
	if !l.Enabled() {
		return
	}
	addr := conn.RemoteAddr().String()
	l.mu.Lock()
	defer l.mu.Unlock()

	ci := l.conns[addr]
	switch state {
	case http.StateNew:
		l.conns[addr] = &connInfo{opened: l.now()}
		l.writeLocked("CONN", addr+" aberta")
	case http.StateActive:
		if ci == nil {
			ci = &connInfo{}
			l.conns[addr] = ci
		}
		ci.reqs++
	case http.StateHijacked:
		delete(l.conns, addr)
		l.writeLocked("CONN", fmt.Sprintf("%s virou stream (%s)", addr, ci.resumo(l.now())))
	case http.StateClosed:
		delete(l.conns, addr)
		l.writeLocked("CONN", fmt.Sprintf("%s fechada (%s)", addr, ci.resumo(l.now())))
	}
}

// StreamOpen marca a abertura de um stream de eventos (alertStream,
// eventManager, snapManager).
func (l *Logger) StreamOpen(remote, path string) {
	if !l.Enabled() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.streams[remote] = &streamInfo{opened: l.now(), path: path}
	l.writeLocked("CONECTADO", remote+" "+path)
}

// StreamEvent registra um evento escrito no stream (heartbeat não entra).
func (l *Logger) StreamEvent(remote, summary string, size int) {
	if !l.Enabled() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if s := l.streams[remote]; s != nil {
		s.events++
	}
	msg := fmt.Sprintf("-> %s stream %s", remote, humanSize(size))
	if summary != "" {
		msg += " | " + summary
	}
	l.writeLocked("OUT", msg)
}

// StreamClose fecha o stream com duração e total de eventos.
func (l *Logger) StreamClose(remote string) {
	if !l.Enabled() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.streams[remote]
	delete(l.streams, remote)
	if s == nil {
		l.writeLocked("DESCONECTADO", remote)
		return
	}
	l.writeLocked("DESCONECTADO", fmt.Sprintf("%s %s após %s, %d eventos",
		remote, s.path, l.now().Sub(s.opened).Round(time.Second), s.events))
}

// Out registra um envio do emulador ao gerenciador (push de evento,
// keepalive).
func (l *Logger) Out(method, url, contentType string, body []byte, status int, err error, d time.Duration) {
	if !l.Enabled() {
		return
	}
	res := fmt.Sprint(status)
	if err != nil {
		res = "erro: " + err.Error()
	}
	msg := fmt.Sprintf("-> %s %s -> %s %dms", method, url, res, d.Milliseconds())
	if f := KeyFields(contentType, body, nil); f != "" {
		msg += " | " + f
	}
	if desc := Describe(contentType, body); desc != "" {
		msg += "\n    > " + desc
	}
	l.Printf("OUT", "%s", msg)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race ./internal/devicelog/ -v`
Expected: PASS em todos (Tasks 1–3).

- [ ] **Step 5: Commit**

```bash
git add internal/devicelog/capture.go internal/devicelog/capture_test.go
git commit -m "feat(devicelog): middleware gin, conexões TCP, streams e saídas"
```

---

### Task 4: Registry e integração no Manager

**Files:**
- Create: `internal/devicelog/registry.go`
- Test: `internal/devicelog/registry_test.go`
- Modify: `internal/emulator/manager.go` (struct `Manager` ~linha 25, `NewManager` ~linha 70, `createEmulator` ~linha 553, `UpdateDeviceSettings` ~linha 877)
- Modify: `internal/emulator/crud.go` (`DeleteDevice`, depois do `tx.Commit` ~linha 315)
- Test: `internal/emulator/devicelog_test.go`

**Interfaces:**
- Consumes: `devicelog.New`, `Logger.SetEnabled/SetInfo/WriteZip`, `Options.FilePath/Files/removeAll`, `writeZip`.
- Produces:
  - `func NewRegistry(opts Options, onError func(error)) *Registry`
  - `func (r *Registry) For(info Info) *Logger`
  - `func (r *Registry) SetEnabled(info Info, on bool) error`
  - `func (r *Registry) Remove(id int) error`
  - `func (r *Registry) ActivePath(id int) string`
  - `func (r *Registry) WriteZip(id int, w io.Writer) error`
  - `Manager.Logs *devicelog.Registry` (exportado; usado pela Task 7)
  - `func (m *Manager) deviceLog(device models.Device) *devicelog.Logger` (usado pelas Tasks 5–6 em `createEmulator`)
- Registry com receptor nil: `For` devolve nil; os demais viram no-op (exceto `ActivePath`, que só é chamado com registry real).

- [ ] **Step 1: Write the failing tests**

`internal/devicelog/registry_test.go`:

```go
package devicelog

import (
	"bytes"
	"os"
	"testing"
)

func TestForDevolveOMesmoLoggerEAtualizaInfo(t *testing.T) {
	r := NewRegistry(optsDeTeste(t), nil)
	a := r.For(Info{ID: 4, Name: "velho"})
	b := r.For(Info{ID: 4, Name: "novo"})
	if a != b {
		t.Fatal("For criou dois loggers para o mesmo id")
	}
	if b.info.Name != "novo" {
		t.Errorf("info não atualizada: %+v", b.info)
	}
}

func TestRemoveApagaMesmoSemLogger(t *testing.T) {
	o := optsDeTeste(t)
	if err := os.MkdirAll(o.Dir, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(o.FilePath(5, 0), []byte("x"), 0644)
	_ = os.WriteFile(o.FilePath(5, 1), []byte("x"), 0644)
	r := NewRegistry(o, nil)
	if err := r.Remove(5); err != nil {
		t.Fatal(err)
	}
	if fs := o.Files(5); len(fs) != 0 {
		t.Errorf("sobrou: %v", fs)
	}
}

func TestRemoveNaoTocaEmIDComMesmoPrefixo(t *testing.T) {
	o := optsDeTeste(t)
	r := NewRegistry(o, nil)
	_ = r.SetEnabled(Info{ID: 1}, true)
	_ = r.SetEnabled(Info{ID: 12}, true)
	_ = r.Remove(1)
	if fs := o.Files(12); len(fs) != 1 {
		t.Errorf("arquivos do 12: %v", fs)
	}
	var buf bytes.Buffer
	if err := r.WriteZip(1, &buf); err != ErrNoLogs {
		t.Errorf("zip do 1 depois de remover: %v", err)
	}
}

func TestRegistryWriteZipSemLoggerLeDoDisco(t *testing.T) {
	o := optsDeTeste(t)
	_ = os.MkdirAll(o.Dir, 0755)
	_ = os.WriteFile(o.FilePath(8, 0), []byte("linha\n"), 0644)
	r := NewRegistry(o, nil)
	var buf bytes.Buffer
	if err := r.WriteZip(8, &buf); err != nil {
		t.Fatal(err)
	}
	if buf.Len() == 0 {
		t.Error("zip vazio")
	}
}

func TestRegistryNilNaoEntraEmPanico(t *testing.T) {
	var r *Registry
	if r.For(Info{ID: 1}) != nil {
		t.Error("For em nil deveria devolver nil")
	}
	_ = r.SetEnabled(Info{ID: 1}, true)
	_ = r.Remove(1)
}
```

`internal/emulator/devicelog_test.go`:

```go
package emulator

import (
	"os"
	"strings"
	"testing"

	"GoFacialEmulator/internal/devicelog"
	"GoFacialEmulator/internal/trace"
)

func managerComLogs(t *testing.T, db *dbFalso) (*Manager, devicelog.Options) {
	t.Helper()
	o := devicelog.Options{Dir: t.TempDir(), MaxSize: 1 << 20, MaxFiles: 8}
	return &Manager{
		ServiceDB:   db,
		EmulatorDB:  db,
		Tracer:      trace.NewTracer(),
		emulators:   map[int]Emulator{},
		watchdog:    map[int]*WatchdogInfo{},
		startErrors: map[int]string{},
		Logs:        devicelog.NewRegistry(o, nil),
	}, o
}

func TestUpdateDeviceSettingsLigaComCabecalhoDoBanco(t *testing.T) {
	db := &dbFalso{linha: linhaFalsa{valores: []interface{}{
		5, "Portaria B", "10.0.0.1", 4005, "Dahua", 1, 0, "stopped", 0, 0, 1, "manual",
	}}}
	m, o := managerComLogs(t, db)

	if err := m.UpdateDeviceSettings(5, true); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(o.FilePath(5, 0))
	if err != nil {
		t.Fatalf("arquivo não criado: %v", err)
	}
	if !strings.HasPrefix(string(b), `# device 5 "Portaria B" Dahua porta 4005`) {
		t.Errorf("cabeçalho: %s", b)
	}

	if err := m.UpdateDeviceSettings(5, false); err != nil {
		t.Fatal(err)
	}
	if fs := o.Files(5); len(fs) != 0 {
		t.Errorf("desligar não apagou: %v", fs)
	}
}

// Mesma montagem de TestDeleteDevicePurgaTodasAsTabelas (crud_test.go):
// deviceSource lê db.linha e precisa ver "manual".
func TestDeleteDeviceApagaLogs(t *testing.T) {
	m, db := managerDeTeste(&txFalsa{})
	db.linha = linhaFalsa{valores: []interface{}{SourceManual}}
	o := devicelog.Options{Dir: t.TempDir(), MaxSize: 1 << 20, MaxFiles: 8}
	m.Logs = devicelog.NewRegistry(o, nil)
	_ = m.Logs.SetEnabled(devicelog.Info{ID: 900001}, true)

	if err := m.DeleteDevice(context.Background(), 900001); err != nil {
		t.Fatal(err)
	}
	if fs := o.Files(900001); len(fs) != 0 {
		t.Errorf("DeleteDevice não apagou os logs: %v", fs)
	}
}
```

Adicione `"context"` aos imports deste arquivo.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/devicelog/ ./internal/emulator/ -run 'Registry|For|Remove|UpdateDeviceSettings|DeleteDeviceApagaLogs' -v`
Expected: FAIL — `undefined: NewRegistry`, `unknown field Logs in struct literal`.

- [ ] **Step 3: Implement the registry**

`internal/devicelog/registry.go`:

```go
package devicelog

import (
	"io"
	"sync"
)

// Registry guarda um Logger por dispositivo. Vive no Manager e sobrevive a
// stop/start do emulador: a flag é do dispositivo, não da instância.
type Registry struct {
	opts    Options
	onError func(error)

	mu      sync.Mutex
	loggers map[int]*Logger
}

func NewRegistry(opts Options, onError func(error)) *Registry {
	return &Registry{opts: opts, onError: onError, loggers: map[int]*Logger{}}
}

// For devolve o logger do dispositivo, criando se preciso, com info
// atualizada.
func (r *Registry) For(info Info) *Logger {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if l, ok := r.loggers[info.ID]; ok {
		l.SetInfo(info)
		return l
	}
	l := New(r.opts, info, r.onError)
	r.loggers[info.ID] = l
	return l
}

// SetEnabled liga ou desliga o log do dispositivo (desligar apaga).
func (r *Registry) SetEnabled(info Info, on bool) error {
	if r == nil {
		return nil
	}
	return r.For(info).SetEnabled(on)
}

// Remove desliga, apaga os arquivos e esquece o dispositivo.
func (r *Registry) Remove(id int) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	l := r.loggers[id]
	delete(r.loggers, id)
	r.mu.Unlock()
	if l != nil {
		return l.SetEnabled(false)
	}
	return r.opts.removeAll(id)
}

// ActivePath é o arquivo mais recente do dispositivo.
func (r *Registry) ActivePath(id int) string {
	return r.opts.FilePath(id, 0)
}

// WriteZip escreve todos os arquivos do dispositivo num zip.
func (r *Registry) WriteZip(id int, w io.Writer) error {
	if r == nil {
		return ErrNoLogs
	}
	r.mu.Lock()
	l := r.loggers[id]
	r.mu.Unlock()
	if l != nil {
		return l.WriteZip(w)
	}
	return writeZip(w, r.opts.Files(id))
}
```

- [ ] **Step 4: Wire into the Manager**

Em `internal/emulator/manager.go`:

1. Import: adicionar `"GoFacialEmulator/internal/devicelog"`.
2. No struct `Manager`, logo depois de `ServicePort int`:

```go
	// Logs guarda o log de cada dispositivo (checkbox "Gravar log de
	// eventos"). Exportado para os handlers servirem os arquivos.
	Logs *devicelog.Registry
```

3. Em `NewManager`, no literal `&Manager{...}`, adicionar:

```go
		Logs: devicelog.NewRegistry(devicelog.DefaultOptions(), func(err error) {
			tracer.Error("%v", err)
		}),
```

4. Logo antes de `func (m *Manager) createEmulator`, adicionar:

```go
// logInfo é o que vai no cabeçalho de cada arquivo de log do dispositivo.
func logInfo(device models.Device) devicelog.Info {
	return devicelog.Info{ID: device.ID, Name: device.Name, Type: device.Model, Port: device.Port}
}

// deviceLog devolve o logger do dispositivo com a flag do banco aplicada.
func (m *Manager) deviceLog(device models.Device) *devicelog.Logger {
	l := m.Logs.For(logInfo(device))
	if err := l.SetEnabled(device.LogEnabled == 1); err != nil {
		m.Tracer.Warning("log do dispositivo %d: %v", device.ID, err)
	}
	return l
}
```

5. Em `UpdateDeviceSettings`, entre o `if err != nil { return ... }` do `Exec` e `m.NotifyChanged(id)`:

```go
	// Vale com o emulador rodando: o logger é o mesmo que ele já usa.
	info := devicelog.Info{ID: id}
	if dev, err := m.GetDevice(id); err == nil {
		info = logInfo(dev)
	}
	if err := m.Logs.SetEnabled(info, logEnabled); err != nil {
		m.Tracer.Warning("log do dispositivo %d: %v", id, err)
	}
```

Em `internal/emulator/crud.go`, em `DeleteDevice`, logo depois do bloco `if err := tx.Commit(ctx); err != nil {...}`:

```go
	if err := m.Logs.Remove(id); err != nil {
		m.Tracer.Warning("apagando logs do dispositivo %d: %v", id, err)
	}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -race ./internal/devicelog/ ./internal/emulator/ -v 2>&1 | tail -40`
Expected: PASS em todos, incluindo os testes antigos do pacote `emulator`.

- [ ] **Step 6: Commit**

```bash
git add internal/devicelog/registry.go internal/devicelog/registry_test.go internal/emulator/manager.go internal/emulator/crud.go internal/emulator/devicelog_test.go
git commit -m "feat(devicelog): registry no Manager; checkbox liga/desliga e apaga logs"
```

---

### Task 5: Hikvision — middleware, conexões, stream e push

**Files:**
- Modify: `internal/emulator/hikvision/emulator.go` (struct ~linha 27, `NewEmulator` ~linha 67, `Start` ~linhas 146-185, `Stop` ~linha 218, `sendEventToRemoteServer` ~linha 421, `handleEventStream` ~linha 729)
- Modify: `internal/emulator/hikvision/handlers.go` (`handleGetAlertStream` ~linha 1289)
- Modify: `internal/emulator/manager.go` (`createEmulator`)
- Test: `internal/emulator/hikvision/devicelog_test.go`

**Interfaces:**
- Consumes: `devicelog.Logger` (`Gin`, `ConnState`, `Printf`, `StreamOpen/Event/Close`, `Out`), `devicelog.MarkStream`, `devicelog.KeyFields`, `Manager.deviceLog` (Task 4).
- Produces:
  - `func NewEmulator(db database.DBInterface, device models.Device, tracer *trace.Tracer, dlog *devicelog.Logger) *Emulator`
  - `func (e *Emulator) newRouter() *gin.Engine`
  - `func (e *Emulator) postEvent(remoteURL, contentType string, body []byte, timeout time.Duration) error`

- [ ] **Step 1: Write the failing tests**

`internal/emulator/hikvision/devicelog_test.go`:

```go
package hikvision

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"GoFacialEmulator/internal/devicelog"
)

func logDeTeste(t *testing.T) (*devicelog.Logger, string) {
	t.Helper()
	dir := t.TempDir()
	l := devicelog.New(devicelog.Options{Dir: dir, MaxSize: 1 << 20, MaxFiles: 8},
		devicelog.Info{ID: 7, Name: "hik", Type: "Hikvision", Port: 4007}, nil)
	if err := l.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	return l, filepath.Join(dir, "device-7.log")
}

// esperarNoLog espera (até 2s) todos os trechos aparecerem: o fechamento da
// conexão chega ao hook ConnState de forma assíncrona.
func esperarNoLog(t *testing.T, caminho string, trechos ...string) {
	t.Helper()
	limite := time.Now().Add(2 * time.Second)
	for {
		b, _ := os.ReadFile(caminho)
		faltando := ""
		for _, tr := range trechos {
			if !strings.Contains(string(b), tr) {
				faltando = tr
				break
			}
		}
		if faltando == "" {
			return
		}
		if time.Now().After(limite) {
			t.Fatalf("falta %q no log:\n%s", faltando, b)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestHikvisionLogRegistraConexaoERequisicao(t *testing.T) {
	e := newTestEmulator(t)
	dlog, caminho := logDeTeste(t)
	e.dlog = dlog

	srv := httptest.NewUnstartedServer(e.newRouter())
	srv.Config.ConnState = e.dlog.ConnState
	srv.Start()

	resp, err := http.Get(srv.URL + "/rota-inexistente?employeeNo=55")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	srv.Close()

	esperarNoLog(t, caminho,
		" aberta",
		"IN 127.0.0.1:",
		"GET /rota-inexistente?employeeNo=55 -> 404 ",
		"| employeeNo=55",
		" fechada (1 reqs, ")
}

func TestHikvisionPostEventRegistraSaida(t *testing.T) {
	alvo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer alvo.Close()

	e := newTestEmulator(t)
	dlog, caminho := logDeTeste(t)
	e.dlog = dlog

	evento := &Event{EventType: "AccessControllerEvent"}
	evento.AccessControllerEvent.CardNo = "999"
	corpo := buildPushEventMultipart(evento, []byte{0xff, 0xd8, 0xff, 0x00}, "MIME_boundary")

	if err := e.postEvent(alvo.URL+"/w-access", "multipart/form-data; boundary=MIME_boundary", corpo, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	esperarNoLog(t, caminho,
		"OUT -> POST "+alvo.URL+"/w-access -> 200 ",
		"cardNo=999",
		"[Picture] <image/jpeg 4B>")
}

func TestHikvisionSemLoggerNaoQuebra(t *testing.T) {
	e := newTestEmulator(t) // dlog nil
	srv := httptest.NewServer(e.newRouter())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/rota-inexistente")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status %d", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/emulator/hikvision/ -run 'Hikvision' -v`
Expected: FAIL — `e.dlog undefined`, `e.newRouter undefined`, `e.postEvent undefined`.

- [ ] **Step 3: Implement**

Em `internal/emulator/hikvision/emulator.go`:

1. Import `"GoFacialEmulator/internal/devicelog"`.
2. No struct `Emulator`, depois de `eventLog *eventLog`:

```go
	// dlog é o log deste dispositivo (checkbox "Gravar log de eventos").
	// Pode ser nil: todo método do devicelog vira no-op.
	dlog *devicelog.Logger
```

3. `NewEmulator` ganha o parâmetro e o campo:

```go
func NewEmulator(db database.DBInterface, device models.Device, tracer *trace.Tracer, dlog *devicelog.Logger) *Emulator {
```

e no literal: `dlog: dlog,`.

4. Em `Start`, substitua o bloco que vai de `// Configura o router (modo release para melhor performance)` até `e.SetupRoutes(router)` (inclusive) por:

```go
	router := e.newRouter()
```

e adicione o método novo (logo antes de `Start`), com o bloco movido e o middleware do log:

```go
// newRouter monta o router do emulador com os middlewares de diagnóstico.
func (e *Emulator) newRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(e.dlog.Gin())

	// Middleware de diagnóstico: loga toda requisição que entra neste emulador.
	router.Use(func(c *gin.Context) {
		e.tracer.Info("[REQ] %s %s from %s (CL=%d CT=%q)",
			c.Request.Method, c.Request.URL.Path, c.Request.RemoteAddr,
			c.Request.ContentLength, c.Request.Header.Get("Content-Type"))
		c.Next()
		e.tracer.Info("[RES] %s %s -> %d (%d bytes)",
			c.Request.Method, c.Request.URL.Path, c.Writer.Status(), c.Writer.Size())
	})

	// NoRoute: loga tentativas de rota não registrada (evita resposta silenciosa).
	router.NoRoute(func(c *gin.Context) {
		e.tracer.Error("[NOROUTE] %s %s not matched", c.Request.Method, c.Request.URL.Path)
		c.String(http.StatusNotFound, "not found: %s %s", c.Request.Method, c.Request.URL.Path)
	})

	e.SetupRoutes(router)
	return router
}
```

(confira que o conteúdo movido é idêntico ao que estava em `Start`; o único acréscimo é `router.Use(e.dlog.Gin())`).

5. No literal `e.server = &http.Server{...}` em `Start`, adicionar `ConnState: e.dlog.ConnState,`.

6. Em `Start`, logo depois de `e.running = true`:

```go
	e.dlog.Printf("START", "emulador %q na porta %d", e.device.Name, e.device.Port)
```

7. Em `Stop`, logo depois de `e.running = false`:

```go
	e.dlog.Printf("STOP", "emulador parado")
```

8. Substitua a metade final de `sendEventToRemoteServer` — do `// Faz a requisição HTTP` até o fim da função — por:

```go
	// multipart/form-data: o receptor WXS (/w-access) lê os campos via
	// request.form (Werkzeug), que só popula para form-data. x-mixed-replace
	// é o content-type do alertStream (streaming), não do POST de push — com
	// ele o WXS responde 200 mas request.form fica vazio e o evento é ignorado.
	contentType := fmt.Sprintf("multipart/form-data; boundary=%s", boundary)
	if err := e.postEvent(remoteURL, contentType, body, 5*time.Second); err != nil {
		return err
	}

	e.tracer.Info("Event sent successfully to %s", remoteURL)
	return nil
}

// postEvent faz o POST de push ao gerenciador e registra a saída no log do
// dispositivo, com sucesso ou erro.
func (e *Emulator) postEvent(remoteURL, contentType string, body []byte, timeout time.Duration) error {
	req, err := http.NewRequest("POST", remoteURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	inicio := time.Now()
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	status := 0
	if resp != nil {
		status = resp.StatusCode
		defer resp.Body.Close()
	}
	e.dlog.Out("POST", remoteURL, contentType, body, status, err, time.Since(inicio))

	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response: %s", resp.Status)
	}
	return nil
}
```

9. Em `handleEventStream`, dentro do `else if eventData != nil {`, logo depois do `if err := bufrw.Flush(); err != nil {...}` do evento (antes de `heartbeatCounter = now`):

```go
					e.dlog.StreamEvent(conn.RemoteAddr().String(),
						devicelog.KeyFields("", eventData, nil), len(eventData))
```

Em `internal/emulator/hikvision/handlers.go`, import `"GoFacialEmulator/internal/devicelog"` e, em `handleGetAlertStream`, logo depois de `defer conn.Close()`:

```go
	devicelog.MarkStream(c)
	remote := conn.RemoteAddr().String()
	e.dlog.StreamOpen(remote, c.Request.URL.RequestURI())
	defer e.dlog.StreamClose(remote)
```

Em `internal/emulator/manager.go`, `createEmulator`:

```go
func (m *Manager) createEmulator(device models.Device) (Emulator, error) {
	dlog := m.deviceLog(device)
	switch device.Model {
	case "Dahua":
		return dahua.NewEmulator(m.EmulatorDB, device, m.Tracer), nil
	case "Hikvision":
		return hikvision.NewEmulator(m.EmulatorDB, device, m.Tracer, dlog), nil
```

(o Dahua passa a receber `dlog` na Task 6; até lá o case Dahua fica como está).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go build ./... && go test -race ./internal/emulator/... -v 2>&1 | tail -40`
Expected: build OK; PASS nos três testes novos e nos antigos do pacote.

- [ ] **Step 5: Commit**

```bash
git add internal/emulator/hikvision/ internal/emulator/manager.go
git commit -m "feat(hikvision): log por dispositivo — conexões, requisições, alertStream e push"
```

---

### Task 6: Dahua — middleware, conexões, streams, push e keepalive

**Files:**
- Modify: `internal/emulator/dahua/emulator.go` (struct ~linha 23, `NewEmulator` ~linha 59, `startKeepAlive` ~linha 144, `Start` ~linhas 292-345, `Stop`, `sendOnlineEventToServer` ~linha 679, `sendTextEventToServer` ~linha 765)
- Modify: `internal/emulator/dahua/handlers.go` (`handleSnapManager` ~linha 600, `handleEventManager` ~linha 656, `handleEventStream` ~linha 697)
- Modify: `internal/emulator/manager.go` (`createEmulator`)
- Test: `internal/emulator/dahua/devicelog_test.go`

**Interfaces:**
- Consumes: mesmas da Task 5.
- Produces:
  - `func NewEmulator(db database.DBInterface, device models.Device, tracer *trace.Tracer, dlog *devicelog.Logger) *Emulator`
  - `func (e *Emulator) newRouter() *gin.Engine`
  - `func (e *Emulator) postNotification(remoteURL string, payload []byte, timeout time.Duration) error`

- [ ] **Step 1: Write the failing tests**

`internal/emulator/dahua/devicelog_test.go`:

```go
package dahua

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"GoFacialEmulator/internal/devicelog"
	"GoFacialEmulator/internal/trace"
)

func logDeTeste(t *testing.T) (*devicelog.Logger, string) {
	t.Helper()
	dir := t.TempDir()
	l := devicelog.New(devicelog.Options{Dir: dir, MaxSize: 1 << 20, MaxFiles: 8},
		devicelog.Info{ID: 9, Name: "dahua", Type: "Dahua", Port: 4009}, nil)
	if err := l.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	return l, filepath.Join(dir, "device-9.log")
}

// esperarNoLog espera (até 2s) todos os trechos aparecerem: o fechamento da
// conexão chega ao hook ConnState de forma assíncrona.
func esperarNoLog(t *testing.T, caminho string, trechos ...string) {
	t.Helper()
	limite := time.Now().Add(2 * time.Second)
	for {
		b, _ := os.ReadFile(caminho)
		faltando := ""
		for _, tr := range trechos {
			if !strings.Contains(string(b), tr) {
				faltando = tr
				break
			}
		}
		if faltando == "" {
			return
		}
		if time.Now().After(limite) {
			t.Fatalf("falta %q no log:\n%s", faltando, b)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func emuladorDahuaDeTeste(dlog *devicelog.Logger) *Emulator {
	return &Emulator{
		tracer:   trace.NewTracer(),
		stopChan: make(chan struct{}),
		configs:  newConfigStore("aa:bb:cc:dd:ee:ff"),
		eventLog: newDahuaEventLog(10),
		pushChan: make(chan []byte, 8),
		dlog:     dlog,
	}
}

func TestDahuaLogRegistraConexaoERequisicao(t *testing.T) {
	dlog, caminho := logDeTeste(t)
	e := emuladorDahuaDeTeste(dlog)

	srv := httptest.NewUnstartedServer(e.newRouter())
	srv.Config.ConnState = e.dlog.ConnState
	srv.Start()

	resp, err := http.Get(srv.URL + "/cgi-bin/inexistente.cgi?action=insert&name=AccessUserCard")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	srv.Close()

	esperarNoLog(t, caminho,
		" aberta",
		"IN 127.0.0.1:",
		"GET /cgi-bin/inexistente.cgi?action=insert&name=AccessUserCard -> 404 ",
		"| action=insert name=AccessUserCard",
		" fechada (1 reqs, ")
}

func TestDahuaPostNotificationRegistraSaida(t *testing.T) {
	alvo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer alvo.Close()

	dlog, caminho := logDeTeste(t)
	e := emuladorDahuaDeTeste(dlog)

	corpo := "--myboundary\r\nContent-Type: text/plain\r\nContent-Disposition: form-data; name=\"info\"\r\n\r\n" +
		`{"Events":[{"Code":"DoorStatus","Data":{"Status":"Open"}}]}` + "\r\n--myboundary--\r\n"
	if err := e.postNotification(alvo.URL+"/notification", []byte(corpo), 2*time.Second); err != nil {
		t.Fatal(err)
	}
	esperarNoLog(t, caminho,
		"OUT -> POST "+alvo.URL+"/notification -> 200 ",
		"Code=DoorStatus Status=Open")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/emulator/dahua/ -run 'Dahua(Log|Post)' -v`
Expected: FAIL — `unknown field dlog`, `e.newRouter undefined`, `e.postNotification undefined`.

- [ ] **Step 3: Implement**

Em `internal/emulator/dahua/emulator.go`:

1. Import `"GoFacialEmulator/internal/devicelog"`.
2. No struct `Emulator`, depois de `enrollFn func(userID string)`:

```go
	// dlog é o log deste dispositivo (checkbox "Gravar log de eventos").
	// Pode ser nil: todo método do devicelog vira no-op.
	dlog *devicelog.Logger
```

3. `NewEmulator(db database.DBInterface, device models.Device, tracer *trace.Tracer, dlog *devicelog.Logger) *Emulator` e no literal `dlog: dlog,`.

4. Em `Start`, substitua o bloco de `// Configura o router (modo release para melhor performance)` até `e.SetupRoutes(router)` (inclusive) por `router := e.newRouter()`, e crie o método antes de `Start` com o bloco movido (idêntico, mais o `Use(e.dlog.Gin())`):

```go
// newRouter monta o router do emulador com os middlewares de diagnóstico.
func (e *Emulator) newRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(e.dlog.Gin())

	// Middleware de diagnostico: loga toda requisicao que entra neste emulador
	// (mesmo par [REQ]/[RES] do Hikvision).
	router.Use(func(c *gin.Context) {
		e.tracer.Info("[REQ] %s %s?%s from %s (CL=%d CT=%q)",
			c.Request.Method, c.Request.URL.Path, c.Request.URL.RawQuery,
			c.Request.RemoteAddr, c.Request.ContentLength,
			c.Request.Header.Get("Content-Type"))
		c.Next()
		e.tracer.Info("[RES] %s %s -> %d (%d bytes)",
			c.Request.Method, c.Request.URL.Path, c.Writer.Status(), c.Writer.Size())
	})

	// NoRoute: sem isto, um endpoint que o gerenciador chama e o emulador nao
	// implementa vira 404 silencioso.
	router.NoRoute(func(c *gin.Context) {
		e.tracer.Error("[NOROUTE] %s %s?%s not matched",
			c.Request.Method, c.Request.URL.Path, c.Request.URL.RawQuery)
		c.String(http.StatusNotFound, "not found: %s %s", c.Request.Method, c.Request.URL.Path)
	})

	e.SetupRoutes(router)
	return router
}
```

5. No literal `e.server = &http.Server{...}`: `ConnState: e.dlog.ConnState,`.
6. Em `Start`, depois de `e.running = true`: `e.dlog.Printf("START", "emulador %q na porta %d", e.device.Name, e.device.Port)`.
7. Em `Stop`, depois de `e.running = false`: `e.dlog.Printf("STOP", "emulador parado")`.

8. Em `startKeepAlive`, troque:

```go
				url := e.remoteServerURL + path
				resp, err := client.Get(url)
				if err != nil {
```

por:

```go
				url := e.remoteServerURL + path
				inicio := time.Now()
				resp, err := client.Get(url)
				status := 0
				if resp != nil {
					status = resp.StatusCode
				}
				e.dlog.Out("GET", url, "", nil, status, err, time.Since(inicio))
				if err != nil {
```

9. Substitua `sendOnlineEventToServer` e `sendTextEventToServer` inteiras por:

```go
// sendOnlineEventToServer envia evento online para servidor remoto no formato Dahua
func (e *Emulator) sendOnlineEventToServer(textPart, imagePart string, imageData []byte, closingPart string, totalLength int) error {
	remoteURL := e.remoteServerURL + "/notification"
	e.tracer.Info("Sending online event to server: %s (total size: %d bytes)", remoteURL, totalLength)

	var body bytes.Buffer
	body.WriteString(textPart)
	body.WriteString(imagePart)
	body.Write(imageData)
	body.WriteString(closingPart)

	if err := e.postNotification(remoteURL, body.Bytes(), 10*time.Second); err != nil {
		return err
	}
	e.tracer.Info("Online event sent successfully to %s", remoteURL)
	return nil
}
```

```go
// sendTextEventToServer envia evento de texto simples (sem imagem) para servidor remoto
func (e *Emulator) sendTextEventToServer(body string) error {
	remoteURL := e.remoteServerURL + "/notification"
	e.tracer.Info("Sending text event to server: %s", remoteURL)

	if err := e.postNotification(remoteURL, []byte(body), 5*time.Second); err != nil {
		return err
	}
	e.tracer.Info("Text event sent successfully to %s", remoteURL)
	return nil
}

// postNotification faz o POST /notification com os headers do dispositivo
// real e registra a saída no log do dispositivo.
func (e *Emulator) postNotification(remoteURL string, payload []byte, timeout time.Duration) error {
	const contentType = "multipart/mixed; boundary=myboundary"
	req, err := http.NewRequest("POST", remoteURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Connection", "Keep-Alive")
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(payload)))

	inicio := time.Now()
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	status := 0
	if resp != nil {
		status = resp.StatusCode
		defer resp.Body.Close()
	}
	e.dlog.Out("POST", remoteURL, contentType, payload, status, err, time.Since(inicio))

	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response: %s", resp.Status)
	}
	return nil
}
```

(Antes o Content-Length do online vinha de `totalLength`; agora é `len(payload)`, que é a soma das mesmas partes. Se `strings` deixar de ser usado em `emulator.go`, o compilador avisa — remova o import só nesse caso.)

Em `internal/emulator/dahua/handlers.go`, import `"GoFacialEmulator/internal/devicelog"` e:

10. Em `handleSnapManager` e em `handleEventManager`, logo depois de `defer conn.Close()`:

```go
	devicelog.MarkStream(c)
	remote := conn.RemoteAddr().String()
	e.dlog.StreamOpen(remote, c.Request.URL.RequestURI())
	defer e.dlog.StreamClose(remote)
```

11. Em `handleEventStream`, no laço que drena `pushChan`, logo depois de `bufrw.Flush()`:

```go
					e.dlog.StreamEvent(conn.RemoteAddr().String(),
						devicelog.KeyFields("", part, nil), len(part))
```

12. Em `handleEventStream`, logo depois de `e.tracer.Info("Event sent: %d total bytes (text=%d + image=%d)", ...)`:

```go
						e.dlog.StreamEvent(conn.RemoteAddr().String(),
							devicelog.KeyFields("", textPart, nil), msgTextPart+msgImagePart)
```

Em `internal/emulator/manager.go`, `createEmulator`: `return dahua.NewEmulator(m.EmulatorDB, device, m.Tracer, dlog), nil`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go build ./... && go vet ./internal/... && go test -race ./internal/emulator/... -v 2>&1 | tail -40`
Expected: build e vet OK; PASS nos testes novos e antigos (o pacote dahua leva ~10s).

- [ ] **Step 5: Commit**

```bash
git add internal/emulator/dahua/ internal/emulator/manager.go
git commit -m "feat(dahua): log por dispositivo — conexões, requisições, streams, push e keepalive"
```

---

### Task 7: Endpoints `/log` e `/log.zip`

**Files:**
- Create: `internal/handlers/device_log.go`
- Modify: `internal/handlers/handlers.go` (struct `Handler` ~linha 26, literal em `NewHandler` ~linha 92, grupo `devices` ~linha 236)
- Test: `internal/handlers/device_log_test.go`

**Interfaces:**
- Consumes: `Manager.Logs`, `Registry.ActivePath`, `Registry.WriteZip`, `devicelog.ErrNoLogs`.
- Produces: `GET /api/devices/:id/log`, `GET /api/devices/:id/log.zip`; métodos `(h *Handler) getDeviceLog`, `(h *Handler) getDeviceLogZip`.

- [ ] **Step 1: Write the failing tests**

`internal/handlers/device_log_test.go`:

```go
package handlers

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"GoFacialEmulator/internal/devicelog"

	"github.com/gin-gonic/gin"
)

// routerDeLog registra as rotas como em produção, junto de uma irmã com
// parâmetro, para pegar conflito de rota no gin.
func routerDeLog(t *testing.T) (*gin.Engine, *devicelog.Registry) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	reg := devicelog.NewRegistry(devicelog.Options{Dir: t.TempDir(), MaxSize: 1 << 20, MaxFiles: 8}, nil)
	h := &Handler{tracer: tracerDeTeste(t), logs: reg}
	r := gin.New()
	g := r.Group("/api/devices")
	g.GET("/:id/settings", func(c *gin.Context) { c.Status(204) })
	g.GET("/:id/log", h.getDeviceLog)
	g.GET("/:id/log.zip", h.getDeviceLogZip)
	return r, reg
}

func get(r *gin.Engine, alvo string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, alvo, nil))
	return rec
}

func TestGetDeviceLogServeArquivoAtivoInline(t *testing.T) {
	r, reg := routerDeLog(t)
	l := reg.For(devicelog.Info{ID: 3, Name: "x", Type: "Dahua", Port: 4003})
	_ = l.SetEnabled(true)
	l.Printf("IN", "linha de teste")

	rec := get(r, "/api/devices/3/log")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "inline") {
		t.Errorf("Content-Disposition %q", cd)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("sem no-store")
	}
	if !strings.Contains(rec.Body.String(), "IN linha de teste") {
		t.Errorf("corpo: %s", rec.Body.String())
	}
	if get(r, "/api/devices/3/settings").Code != 204 {
		t.Error("rota irmã quebrou")
	}
}

func TestGetDeviceLogZip(t *testing.T) {
	r, reg := routerDeLog(t)
	_ = reg.SetEnabled(devicelog.Info{ID: 3}, true)

	rec := get(r, "/api/devices/3/log.zip")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, `filename="device-3-logs-`) {
		t.Errorf("Content-Disposition %q", cd)
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != 1 || zr.File[0].Name != "device-3.log" {
		t.Errorf("zip: %+v", zr.File)
	}
}

func TestGetDeviceLogSemArquivo404(t *testing.T) {
	r, _ := routerDeLog(t)
	for _, alvo := range []string{"/api/devices/9/log", "/api/devices/9/log.zip"} {
		if rec := get(r, alvo); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, quero 404", alvo, rec.Code)
		}
	}
}

func TestGetDeviceLogIDInvalido400(t *testing.T) {
	r, _ := routerDeLog(t)
	for _, alvo := range []string{"/api/devices/abc/log", "/api/devices/0/log.zip", "/api/devices/-1/log"} {
		if rec := get(r, alvo); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, quero 400", alvo, rec.Code)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/handlers/ -run DeviceLog -v`
Expected: FAIL — `unknown field logs`, `h.getDeviceLog undefined`.

- [ ] **Step 3: Implement**

`internal/handlers/device_log.go`:

```go
package handlers

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"GoFacialEmulator/internal/devicelog"

	"github.com/gin-gonic/gin"
)

// idDoLog lê o :id. O caminho do arquivo sai só deste número, nunca de texto
// vindo da requisição.
func idDoLog(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid device ID"})
		return 0, false
	}
	return id, true
}

// getDeviceLog serve o arquivo de log mais recente do dispositivo para abrir
// no navegador ("Ver log" na gaveta).
func (h *Handler) getDeviceLog(c *gin.Context) {
	id, ok := idDoLog(c)
	if !ok {
		return
	}
	f, err := os.Open(h.logs.ActivePath(id))
	if errors.Is(err, os.ErrNotExist) {
		c.String(http.StatusNotFound, "Log desligado ou ainda vazio para o dispositivo %d.", id)
		return
	}
	if err != nil {
		h.tracer.Error("Abrindo log do dispositivo %d: %v", id, err)
		c.String(http.StatusInternalServerError, "Não foi possível ler o log.")
		return
	}
	defer f.Close()

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="device-%d.log"`, id))
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, f); err != nil {
		h.tracer.Warning("Enviando log do dispositivo %d: %v", id, err)
	}
}

// getDeviceLogZip devolve um zip com todos os arquivos de log do dispositivo.
func (h *Handler) getDeviceLogZip(c *gin.Context) {
	id, ok := idDoLog(c)
	if !ok {
		return
	}
	var buf bytes.Buffer
	err := h.logs.WriteZip(id, &buf)
	if errors.Is(err, devicelog.ErrNoLogs) {
		c.String(http.StatusNotFound, "Nenhum log para o dispositivo %d.", id)
		return
	}
	if err != nil {
		h.tracer.Error("Montando zip de logs do dispositivo %d: %v", id, err)
		c.String(http.StatusInternalServerError, "Não foi possível montar o zip.")
		return
	}

	nome := fmt.Sprintf("device-%d-logs-%s.zip", id, time.Now().Format("20060102-1504"))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, nome))
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}
```

Em `internal/handlers/handlers.go`:

1. Import `"GoFacialEmulator/internal/devicelog"`.
2. No struct `Handler`, depois de `env reachability.Environment`: `logs *devicelog.Registry`.
3. No literal `return &Handler{...}` de `NewHandler`, depois de `manager: manager,`: `logs: manager.Logs,`.
4. No grupo `devices`, depois de `devices.GET("/:id/settings", h.getDeviceSettings)`:

```go
			devices.GET("/:id/log", h.getDeviceLog)
			devices.GET("/:id/log.zip", h.getDeviceLogZip)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race ./internal/handlers/ -v 2>&1 | tail -30 && go build ./...`
Expected: PASS em todos; build OK (nenhum pânico de conflito de rota ao montar o router).

- [ ] **Step 5: Commit**

```bash
git add internal/handlers/device_log.go internal/handlers/device_log_test.go internal/handlers/handlers.go
git commit -m "feat(api): ver log do dispositivo e baixar zip com todos os arquivos"
```

---

### Task 8: UI — checkbox liberado, confirmação e links

**Files:**
- Modify: `assets/web/templates/devices.html` (painel `panel-settings`, ~linhas 196-200)
- Modify: `assets/web/static/js/device-drawer.js` (`pintarLog` ~linha 278, `salvarLog` ~linha 289)
- Modify: `assets/web/static/js/devices.js` (coluna Log ~linha 234, `trocarLog` ~linha 811)
- Modify: `assets/web/static/css/components.css` (depois de `.field__help`, ~linha 230)

**Interfaces:**
- Consumes: `GET /api/devices/:id/log`, `GET /api/devices/:id/log.zip` (Task 7); `device.log_enabled`, `device.id` do `FleetStream`.

- [ ] **Step 1: Template**

Em `assets/web/templates/devices.html`, substitua:

```html
          <p class="field__help" id="drawer-log-help">Grava na hora. Só pode ser trocado com o emulador parado.</p>
```

por:

```html
          <p class="field__help" id="drawer-log-help">Últimos 8 arquivos de 3MB. Desligar apaga os arquivos.</p>
          <p class="log-links" id="drawer-log-links" hidden>
            <a class="btn btn--sm" id="drawer-log-view" target="_blank" rel="noopener">Ver log</a>
            <a class="btn btn--sm" id="drawer-log-zip" download>Baixar logs (.zip)</a>
          </p>
```

- [ ] **Step 2: CSS**

Em `assets/web/static/css/components.css`, logo depois do bloco `.field__help { ... }`:

```css
.log-links {
    display: flex;
    gap: var(--gap-2);
    margin-top: var(--gap-2);
}

.log-links[hidden] {
    display: none;
}
```

- [ ] **Step 3: Gaveta**

Em `assets/web/static/js/device-drawer.js`, substitua o comentário e a função `pintarLog` inteiros:

```js
    // O emulador precisa estar parado para trocar a flag de log — mesma
    // regra da coluna Log da tabela.
    function pintarLog(device) {
        ...
    }
```

por:

```js
    var AVISO_DESLIGAR_LOG = 'Desligar apaga os logs deste emulador. Continuar?';

    // A flag troca com o emulador em qualquer estado. Os links só aparecem
    // com o log ligado, porque desligar apaga os arquivos.
    function pintarLog(device) {
        var log = el('drawer-log');
        var ligado = !!device && device.log_enabled === 1;
        log.checked = ligado;
        log.disabled = !device;
        el('drawer-log-links').hidden = !ligado;
        if (device) {
            el('drawer-log-view').href = '/api/devices/' + device.id + '/log';
            el('drawer-log-zip').href = '/api/devices/' + device.id + '/log.zip';
        }
    }
```

Em `salvarLog`, logo depois de `var ligado = log.checked;`:

```js
        if (!ligado && !window.confirm(AVISO_DESLIGAR_LOG)) {
            log.checked = true;
            return;
        }
```

- [ ] **Step 4: Tabela**

Em `assets/web/static/js/devices.js`, na coluna do checkbox de log, troque:

```js
                c.disabled = d.status !== 'stopped';
                c.setAttribute('aria-label', 'Gravar log de ' + d.name);
                c.title = c.disabled ? 'Pare o emulador para trocar' : 'Gravar log de eventos';
```

por:

```js
                c.setAttribute('aria-label', 'Gravar log de ' + d.name);
                c.title = 'Gravar log de eventos';
```

e a assinatura da coluna `sig: function (d) { return d.log_enabled + '|' + d.status; }` por `sig: function (d) { return String(d.log_enabled); }`.

Em `trocarLog`, logo depois de `var ligado = alvo.checked;`:

```js
        if (!ligado && !window.confirm('Desligar apaga os logs deste emulador. Continuar?')) {
            alvo.checked = true;
            return;
        }
```

- [ ] **Step 5: Verificar nos testes de render e no navegador**

Run: `go test ./internal/handlers/ -run Render -v 2>&1 | tail -10`
Expected: PASS (os templates continuam renderizando).

Depois, suba o app (skill `run` ou `./run-local.sh`), abra a tela de dispositivos e confira:
- checkbox de log habilitado na tabela e na gaveta com emulador rodando;
- ligar → aparecem "Ver log" e "Baixar logs (.zip)"; "Ver log" abre o texto em outra aba;
- desmarcar → aparece a confirmação; cancelar mantém marcado; confirmar some com os links.

- [ ] **Step 6: Commit**

```bash
git add assets/web/templates/devices.html assets/web/static/js/device-drawer.js assets/web/static/js/devices.js assets/web/static/css/components.css
git commit -m "feat(ui): log liga com emulador rodando, links Ver log e Baixar zip"
```

---

### Task 9: Manual e verificação final

**Files:**
- Modify: `docs/manual/conteudo/comum/logs.md` (seção "O log de um dispositivo específico")

- [ ] **Step 1: Atualizar o manual**

Substitua a seção `## O log de um dispositivo específico` inteira (até antes de `## O log de instalação`) por:

```markdown
## O log de um dispositivo específico

Cada emulador pode gravar o próprio log, separado do log geral. Ele
registra quem conectou (endereço e porta de origem — útil para descobrir
dois gerenciadores falando com o mesmo emulador), cada requisição recebida
com os dados principais do objeto (usuário, cartão, evento) e cada evento
que o emulador enviou ao gerenciador.

Para ligar, abra a gaveta de detalhes do dispositivo, aba Configurações, e
marque **Gravar log de eventos**. Vale na hora, com o emulador rodando ou
parado. Com o log ligado aparecem dois links:

- **Ver log** — abre o arquivo mais recente no navegador.
- **Baixar logs (.zip)** — baixa todos os arquivos daquele emulador.

Os arquivos ficam em `sistema/logs/devices/` (`device-<id>.log` é o atual;
`device-<id>.1.log` a `.7.log` são os anteriores). São guardados no máximo
8 arquivos de 3MB por emulador; o mais antigo é apagado ao passar disso.

**Desligar o log apaga os arquivos daquele emulador** — a tela pede
confirmação. Baixe o zip antes se quiser guardar. Excluir o emulador também
apaga os arquivos.
```

- [ ] **Step 2: Suite completa**

Run: `go vet ./... && go test -race ./... 2>&1 | tail -20`
Expected: vet sem avisos; todos os pacotes `ok`.

- [ ] **Step 3: Verificação manual (dois gerenciadores)**

Com o serviço rodando e um emulador Hikvision na porta P com log ligado:

```bash
curl -s -o /dev/null "http://127.0.0.1:P/ISAPI/System/deviceInfo"
curl -s -o /dev/null --interface <outro IP local, se houver> "http://<IP do host>:P/ISAPI/System/deviceInfo"
curl -s "http://127.0.0.1:7070/api/devices/<id>/log" | tail -20
```

Expected: duas origens diferentes em linhas `CONN ... aberta` e `IN ...`. No deploy Docker, repetir de outra máquina e confirmar que aparece o IP real dela (não o gateway do Docker) — premissa do spec.

Baixar `http://127.0.0.1:7070/api/devices/<id>/log.zip`, abrir e conferir `device-<id>.log`. Desligar o checkbox e conferir que `logs/devices/device-<id>*.log` sumiu.

- [ ] **Step 4: Commit**

```bash
git add docs/manual/conteudo/comum/logs.md
git commit -m "docs(manual): log por dispositivo — onde fica, links e retenção"
```
