# Alcançabilidade de Rede e Modo do Dispositivo — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fazer a aplicação avisar quando o ambiente não vai conseguir entregar as portas que o W-Access pediu, corrigir os padrões de rede dos pacotes, e expor o modo online/standalone na interface — deixando a tela de dispositivos estável para os manuais da fase seguinte.

**Architecture:** Um pacote novo `internal/reachability` concentra a decisão em uma função pura `Analyze(ports, env) Report`, alimentada por um `Detect()` que lê o ambiente uma vez na inicialização. O `Manager` passa a guardar o último erro de bind por dispositivo, que é o sinal de alcançabilidade em ambiente nativo. Um endpoint `GET /api/reachability` serve o relatório e a tela de dispositivos exibe um aviso quando há dispositivo inalcançável. Em paralelo, os composes e o `instalar.sh` deixam de produzir ambientes onde a falha é silenciosa.

**Tech Stack:** Go 1.21 (toolchain local 1.25.4), Gin, pgx/v5, `gopkg.in/yaml.v3`, testes com a biblioteca padrão (o repositório não usa testify), Docker Compose v2, batch (`cmd.exe`) e bash.

**Spec:** `docs/superpowers/specs/2026-08-14-manuais-ilustrados-e-rede-design.md`

**Escopo deste plano:** fases B e C da spec. A fase A já está no commit `ba85808`. A fase D (manuais) tem plano próprio, escrito depois que este aterrissar.

## Global Constraints

- Porta HTTP da aplicação: **7070** em todos os arquivos, sem exceção.
- Porta do PostgreSQL: **5432** no Docker e no Linux; **5433** no pacote Windows portátil.
- Faixa padrão de portas de emuladores publicada pelo pacote Docker no Windows: **4000-4499** (era 4000-4099).
- Nenhum arquivo versionado pode conter credencial real do W_Access. Strings proibidas: `db_W-X-S@Wellcare924_`, `172.16.17.67`, `172.20.112.1`.
- Diretório de logs da aplicação: `logs`.
- A aplicação **obedece** o `BaseCommPort` do W-Access e nunca o contradiz: não escolhe porta, não realoca, não recusa dispositivo. Só avisa.
- Toda mensagem que o usuário final lê é em português, sem jargão.
- Comentários e nomes de teste em português, seguindo `internal/monitoring/health_test.go` e `internal/handlers/health_http_test.go`.
- Testes usam apenas a biblioteca padrão. Não adicionar testify.
- Commits com título em inglês no formato Conventional Commits e corpo em português quando descreverem comportamento para o usuário final.

---

### Task 1: Decisão de alcançabilidade como função pura

O núcleo da fase B. Uma função sem I/O, sem relógio e sem banco, para que a matriz inteira de ambientes vire teste de tabela.

**Files:**
- Create: `internal/reachability/reachability.go`
- Test: `internal/reachability/reachability_test.go`

**Interfaces:**
- Consumes: nada.
- Produces:
  - `type Kind int` com `KindLinux`, `KindWSL`, `KindDocker`, `KindWindows`
  - `type PortRange struct { Start, End int }` e `func (r PortRange) Contains(port int) bool`
  - `type Environment struct { Kind Kind; PublishedRanges []PortRange; RangesKnown bool; HostNetwork bool; WSLMirrored bool; MaxOpenFiles uint64 }`
  - `type Status int` com `StatusOK`, `StatusUnreachable`, `StatusUnknown`
  - `type DevicePort struct { DeviceID, Port int; Started bool; BindError string }`
  - `type DeviceReachability struct { DeviceID, Port int; Status Status; Reason string }`
  - `type Report struct { Environment Environment; Devices []DeviceReachability; Unreachable, Unknown int }`
  - `func Analyze(ports []DevicePort, env Environment) Report`
  - `func ParseRanges(s string) ([]PortRange, error)`

- [ ] **Step 1: Escrever o teste que falha**

Criar `internal/reachability/reachability_test.go`:

```go
package reachability

import "testing"

// TestParseRanges cobre o formato que o compose grava em
// PUBLISHED_PORT_RANGE. Entrada malformada precisa virar erro, não faixa
// vazia silenciosa: sem isso a aplicação acusaria todo dispositivo como
// inalcançável por causa de um typo no compose.
func TestParseRanges(t *testing.T) {
	casos := []struct {
		nome     string
		entrada  string
		quero    []PortRange
		queroErr bool
	}{
		{"faixa simples", "4000-4499", []PortRange{{4000, 4499}}, false},
		{"porta avulsa", "7070", []PortRange{{7070, 7070}}, false},
		{"duas entradas", "7070,4000-4099", []PortRange{{7070, 7070}, {4000, 4099}}, false},
		{"espaços em volta", " 4000-4099 , 7070 ", []PortRange{{4000, 4099}, {7070, 7070}}, false},
		{"vazio", "", nil, true},
		{"invertida", "4499-4000", nil, true},
		{"não numérica", "abc", nil, true},
		{"fora do intervalo de porta", "0-70000", nil, true},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			tenho, err := ParseRanges(c.entrada)
			if c.queroErr {
				if err == nil {
					t.Fatalf("ParseRanges(%q) = %v, quero erro", c.entrada, tenho)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRanges(%q): %v", c.entrada, err)
			}
			if len(tenho) != len(c.quero) {
				t.Fatalf("ParseRanges(%q) = %v, quero %v", c.entrada, tenho, c.quero)
			}
			for i := range tenho {
				if tenho[i] != c.quero[i] {
					t.Errorf("faixa %d = %v, quero %v", i, tenho[i], c.quero[i])
				}
			}
		})
	}
}

// TestAnalyze cobre a matriz ambiente × porta. O caso que motivou o
// trabalho inteiro é "docker com faixa publicada menor que a pedida pelo
// W-Access": hoje o emulador sobe, escuta dentro do container e é
// invisível de fora, com a tela verde e o W-Access offline.
func TestAnalyze(t *testing.T) {
	dockerPublicado := Environment{
		Kind:            KindDocker,
		PublishedRanges: []PortRange{{4000, 4099}},
		RangesKnown:     true,
	}

	casos := []struct {
		nome        string
		portas      []DevicePort
		env         Environment
		queroStatus []Status
		queroReason []string
	}{
		{
			nome:        "docker: porta dentro da faixa publicada",
			portas:      []DevicePort{{DeviceID: 1, Port: 4001, Started: true}},
			env:         dockerPublicado,
			queroStatus: []Status{StatusOK},
			queroReason: []string{""},
		},
		{
			nome:        "docker: porta fora da faixa publicada",
			portas:      []DevicePort{{DeviceID: 2, Port: 4200, Started: true}},
			env:         dockerPublicado,
			queroStatus: []Status{StatusUnreachable},
			queroReason: []string{"a porta 4200 não está publicada neste ambiente Docker"},
		},
		{
			nome:   "docker sem faixa conhecida: não inventa aviso",
			portas: []DevicePort{{DeviceID: 3, Port: 4200, Started: true}},
			env: Environment{
				Kind:        KindDocker,
				RangesKnown: false,
			},
			queroStatus: []Status{StatusUnknown},
			queroReason: []string{"não foi possível verificar quais portas este ambiente Docker publica"},
		},
		{
			nome:   "docker com rede de host: tratado como nativo",
			portas: []DevicePort{{DeviceID: 4, Port: 4200, Started: true}},
			env: Environment{
				Kind:        KindDocker,
				HostNetwork: true,
			},
			queroStatus: []Status{StatusOK},
			queroReason: []string{""},
		},
		{
			nome:        "linux nativo: bind funcionou",
			portas:      []DevicePort{{DeviceID: 5, Port: 4200, Started: true}},
			env:         Environment{Kind: KindLinux},
			queroStatus: []Status{StatusOK},
			queroReason: []string{""},
		},
		{
			nome:        "linux nativo: bind falhou",
			portas:      []DevicePort{{DeviceID: 6, Port: 4200, Started: true, BindError: "address already in use"}},
			env:         Environment{Kind: KindLinux},
			queroStatus: []Status{StatusUnreachable},
			queroReason: []string{"a porta 4200 não pôde ser aberta: address already in use"},
		},
		{
			nome:        "linux nativo: emulador nem foi iniciado",
			portas:      []DevicePort{{DeviceID: 7, Port: 4200, Started: false}},
			env:         Environment{Kind: KindLinux},
			queroStatus: []Status{StatusUnknown},
			queroReason: []string{"o emulador ainda não foi iniciado"},
		},
		{
			nome:        "wsl sem modo espelhado: só a máquina local alcança",
			portas:      []DevicePort{{DeviceID: 8, Port: 4001, Started: true}},
			env:         Environment{Kind: KindWSL, WSLMirrored: false},
			queroStatus: []Status{StatusUnreachable},
			queroReason: []string{"nesta WSL sem modo espelhado, só esta máquina alcança a porta 4001"},
		},
		{
			nome:        "wsl com modo espelhado: tratado como nativo",
			portas:      []DevicePort{{DeviceID: 9, Port: 4001, Started: true}},
			env:         Environment{Kind: KindWSL, WSLMirrored: true},
			queroStatus: []Status{StatusOK},
			queroReason: []string{""},
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			r := Analyze(c.portas, c.env)
			if len(r.Devices) != len(c.queroStatus) {
				t.Fatalf("len(Devices) = %d, quero %d", len(r.Devices), len(c.queroStatus))
			}
			for i, d := range r.Devices {
				if d.Status != c.queroStatus[i] {
					t.Errorf("dispositivo %d: status = %v, quero %v", d.DeviceID, d.Status, c.queroStatus[i])
				}
				if d.Reason != c.queroReason[i] {
					t.Errorf("dispositivo %d: motivo = %q, quero %q", d.DeviceID, d.Reason, c.queroReason[i])
				}
			}
		})
	}
}

// TestAnalyzeContadores garante que os totais do topo do relatório batem
// com a lista — é o número que aparece no aviso da tela.
func TestAnalyzeContadores(t *testing.T) {
	env := Environment{
		Kind:            KindDocker,
		PublishedRanges: []PortRange{{4000, 4099}},
		RangesKnown:     true,
	}
	portas := []DevicePort{
		{DeviceID: 1, Port: 4001, Started: true},
		{DeviceID: 2, Port: 4200, Started: true},
		{DeviceID: 3, Port: 4300, Started: true},
	}

	r := Analyze(portas, env)
	if r.Unreachable != 2 {
		t.Errorf("Unreachable = %d, quero 2", r.Unreachable)
	}
	if r.Unknown != 0 {
		t.Errorf("Unknown = %d, quero 0", r.Unknown)
	}
}
```

- [ ] **Step 2: Rodar o teste e confirmar que falha**

Run: `go test ./internal/reachability/ -v`
Expected: FAIL na compilação — o pacote `internal/reachability` não existe.

- [ ] **Step 3: Escrever a implementação mínima**

Criar `internal/reachability/reachability.go`:

```go
// Package reachability decide se as portas que o W-Access pediu conseguem
// ser alcançadas no ambiente onde a aplicação está rodando.
//
// A aplicação obedece o BaseCommPort de cada controlador e nunca o
// contradiz: este pacote não escolhe porta nem recusa dispositivo, apenas
// aponta quais não vão funcionar e por quê.
package reachability

import (
	"fmt"
	"strconv"
	"strings"
)

// Kind identifica o tipo de ambiente onde a aplicação está rodando.
type Kind int

const (
	KindLinux Kind = iota
	KindWSL
	KindDocker
	KindWindows
)

func (k Kind) String() string {
	switch k {
	case KindLinux:
		return "linux"
	case KindWSL:
		return "wsl"
	case KindDocker:
		return "docker"
	case KindWindows:
		return "windows"
	default:
		return "desconhecido"
	}
}

// PortRange é uma faixa fechada de portas.
type PortRange struct {
	Start int
	End   int
}

func (r PortRange) Contains(port int) bool {
	return port >= r.Start && port <= r.End
}

func (r PortRange) String() string {
	if r.Start == r.End {
		return strconv.Itoa(r.Start)
	}
	return fmt.Sprintf("%d-%d", r.Start, r.End)
}

// Environment é a fotografia do ambiente, tirada uma vez na inicialização.
type Environment struct {
	Kind            Kind        `json:"kind"`
	PublishedRanges []PortRange `json:"published_ranges"`
	// RangesKnown falso significa que não foi possível descobrir o que o
	// host publica. Nesse caso nenhum aviso é inventado.
	RangesKnown  bool   `json:"ranges_known"`
	HostNetwork  bool   `json:"host_network"`
	WSLMirrored  bool   `json:"wsl_mirrored"`
	MaxOpenFiles uint64 `json:"max_open_files"`
}

// Status é o veredito para um dispositivo.
type Status int

const (
	StatusOK Status = iota
	StatusUnreachable
	StatusUnknown
)

func (s Status) String() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusUnreachable:
		return "inalcancavel"
	default:
		return "desconhecido"
	}
}

func (s Status) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(s.String())), nil
}

// DevicePort é a entrada de Analyze: o que o W-Access pediu para um
// dispositivo e o que aconteceu quando tentamos abrir aquela porta.
type DevicePort struct {
	DeviceID int
	Port     int
	// Started indica que o emulador chegou a ser iniciado. Sem isso não há
	// informação de bind, e o veredito honesto é "desconhecido".
	Started bool
	// BindError é o erro do sistema operacional na abertura da porta, vazio
	// quando o bind funcionou.
	BindError string
}

// DeviceReachability é o veredito por dispositivo.
type DeviceReachability struct {
	DeviceID int    `json:"device_id"`
	Port     int    `json:"port"`
	Status   Status `json:"status"`
	Reason   string `json:"reason,omitempty"`
}

// Report é o que a interface consome.
type Report struct {
	Environment Environment          `json:"environment"`
	Devices     []DeviceReachability `json:"devices"`
	Unreachable int                  `json:"unreachable"`
	Unknown     int                  `json:"unknown"`
}

// ParseRanges lê o formato gravado em PUBLISHED_PORT_RANGE pelo compose:
// entradas separadas por vírgula, cada uma "porta" ou "inicio-fim".
func ParseRanges(s string) ([]PortRange, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("faixa de portas vazia")
	}

	var faixas []PortRange
	for _, parte := range strings.Split(s, ",") {
		parte = strings.TrimSpace(parte)
		if parte == "" {
			return nil, fmt.Errorf("entrada vazia em %q", s)
		}

		inicio, fim, temHifen := strings.Cut(parte, "-")
		if !temHifen {
			fim = inicio
		}

		p1, err := porta(inicio)
		if err != nil {
			return nil, fmt.Errorf("em %q: %w", parte, err)
		}
		p2, err := porta(fim)
		if err != nil {
			return nil, fmt.Errorf("em %q: %w", parte, err)
		}
		if p1 > p2 {
			return nil, fmt.Errorf("em %q: início maior que fim", parte)
		}
		faixas = append(faixas, PortRange{Start: p1, End: p2})
	}
	return faixas, nil
}

func porta(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("%q não é um número de porta", s)
	}
	if n < 1 || n > 65535 {
		return 0, fmt.Errorf("porta %d fora do intervalo 1-65535", n)
	}
	return n, nil
}

// Analyze é pura: mesmo par de argumentos, mesmo relatório. Sem I/O, sem
// relógio, sem banco.
func Analyze(ports []DevicePort, env Environment) Report {
	r := Report{Environment: env}

	for _, p := range ports {
		d := DeviceReachability{DeviceID: p.DeviceID, Port: p.Port}

		switch {
		case env.Kind == KindDocker && !env.HostNetwork:
			d.Status, d.Reason = vereditoDocker(p, env)
		case env.Kind == KindWSL && !env.WSLMirrored:
			d.Status = StatusUnreachable
			d.Reason = fmt.Sprintf("nesta WSL sem modo espelhado, só esta máquina alcança a porta %d", p.Port)
		default:
			d.Status, d.Reason = vereditoNativo(p)
		}

		switch d.Status {
		case StatusUnreachable:
			r.Unreachable++
		case StatusUnknown:
			r.Unknown++
		}
		r.Devices = append(r.Devices, d)
	}

	return r
}

func vereditoDocker(p DevicePort, env Environment) (Status, string) {
	if !env.RangesKnown {
		return StatusUnknown, "não foi possível verificar quais portas este ambiente Docker publica"
	}
	for _, faixa := range env.PublishedRanges {
		if faixa.Contains(p.Port) {
			return StatusOK, ""
		}
	}
	return StatusUnreachable, fmt.Sprintf("a porta %d não está publicada neste ambiente Docker", p.Port)
}

func vereditoNativo(p DevicePort) (Status, string) {
	if p.BindError != "" {
		return StatusUnreachable, fmt.Sprintf("a porta %d não pôde ser aberta: %s", p.Port, p.BindError)
	}
	if !p.Started {
		return StatusUnknown, "o emulador ainda não foi iniciado"
	}
	return StatusOK, ""
}
```

- [ ] **Step 4: Rodar o teste e confirmar que passa**

Run: `go test ./internal/reachability/ -v`
Expected: PASS em todos os subtestes.

- [ ] **Step 5: Commit**

```bash
git add internal/reachability/
git commit -m "$(cat <<'EOF'
feat(reachability): decide port reachability as a pure function

A aplicação obedece o BaseCommPort do W-Access e nunca o contradiz, mas
até agora nada verificava se o ambiente conseguiria entregar aquela porta.
No pacote Docker padrão, um emulador com porta fora da faixa publicada
sobe, escuta dentro do container e fica invisível de fora — tela toda
verde, W-Access todo offline, nenhum log.

Analyze é pura sobre (portas, ambiente), então a matriz inteira de casos
é teste de tabela. Sem faixa conhecida o veredito é "desconhecido": não
inventa aviso a partir de suposição.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Detecção do ambiente

`Detect()` lê o mundo uma vez. Para ser testável, a leitura fica atrás de uma interface pequena.

**Files:**
- Create: `internal/reachability/detect.go`
- Test: `internal/reachability/detect_test.go`

**Interfaces:**
- Consumes: os tipos da Task 1.
- Produces:
  - `func Detect() Environment` — usado por `main`/`NewHandler`
  - `type leitor interface { existe(path string) bool; ler(path string) ([]byte, error) }` (não exportado)
  - `func detect(fs leitor, getenv func(string) string, goos string, maxOpenFiles uint64) Environment` (não exportado, é o que os testes exercitam)

- [ ] **Step 1: Escrever o teste que falha**

Criar `internal/reachability/detect_test.go`:

```go
package reachability

import (
	"fmt"
	"testing"
)

// leitorFalso é um sistema de arquivos de mentira: só os caminhos
// presentes no mapa existem.
type leitorFalso map[string]string

func (l leitorFalso) existe(path string) bool {
	_, ok := l[path]
	return ok
}

func (l leitorFalso) ler(path string) ([]byte, error) {
	v, ok := l[path]
	if !ok {
		return nil, fmt.Errorf("não existe: %s", path)
	}
	return []byte(v), nil
}

func semEnv(string) string { return "" }

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// TestDetect cobre como o ambiente é reconhecido. O caso mais delicado é
// o Docker: de dentro do container não há como enxergar o mapeamento de
// portas do host, então a única fonte é a variável que o compose grava.
func TestDetect(t *testing.T) {
	casos := []struct {
		nome            string
		fs              leitorFalso
		getenv          func(string) string
		goos            string
		queroKind       Kind
		queroRangesOK   bool
		queroFaixas     []PortRange
		queroHostNet    bool
		queroWSLEspelho bool
	}{
		{
			nome:      "windows nativo",
			fs:        leitorFalso{},
			getenv:    semEnv,
			goos:      "windows",
			queroKind: KindWindows,
		},
		{
			nome:      "linux nativo",
			fs:        leitorFalso{},
			getenv:    semEnv,
			goos:      "linux",
			queroKind: KindLinux,
		},
		{
			nome:      "wsl sem modo espelhado",
			fs:        leitorFalso{"/proc/version": "Linux version 5.15.0-microsoft-standard-WSL2"},
			getenv:    semEnv,
			goos:      "linux",
			queroKind: KindWSL,
		},
		{
			nome: "wsl com modo espelhado",
			fs: leitorFalso{
				"/proc/version":                   "Linux version 5.15.0-microsoft-standard-WSL2",
				"/sys/class/net/loopback0/address": "00:00:00:00:00:00",
			},
			getenv:          semEnv,
			goos:            "linux",
			queroKind:       KindWSL,
			queroWSLEspelho: true,
		},
		{
			nome:          "docker com faixa publicada",
			fs:            leitorFalso{"/.dockerenv": ""},
			getenv:        env(map[string]string{"PUBLISHED_PORT_RANGE": "4000-4499"}),
			goos:          "linux",
			queroKind:     KindDocker,
			queroRangesOK: true,
			queroFaixas:   []PortRange{{4000, 4499}},
		},
		{
			nome:          "docker sem a variável: faixa desconhecida",
			fs:            leitorFalso{"/.dockerenv": ""},
			getenv:        semEnv,
			goos:          "linux",
			queroKind:     KindDocker,
			queroRangesOK: false,
		},
		{
			nome:          "docker com variável malformada: faixa desconhecida",
			fs:            leitorFalso{"/.dockerenv": ""},
			getenv:        env(map[string]string{"PUBLISHED_PORT_RANGE": "4499-4000"}),
			goos:          "linux",
			queroKind:     KindDocker,
			queroRangesOK: false,
		},
		{
			nome:         "docker com rede de host",
			fs:           leitorFalso{"/.dockerenv": ""},
			getenv:       env(map[string]string{"HOST_NETWORK": "1"}),
			goos:         "linux",
			queroKind:    KindDocker,
			queroHostNet: true,
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			e := detect(c.fs, c.getenv, c.goos, 1024)

			if e.Kind != c.queroKind {
				t.Errorf("Kind = %v, quero %v", e.Kind, c.queroKind)
			}
			if e.RangesKnown != c.queroRangesOK {
				t.Errorf("RangesKnown = %v, quero %v", e.RangesKnown, c.queroRangesOK)
			}
			if e.HostNetwork != c.queroHostNet {
				t.Errorf("HostNetwork = %v, quero %v", e.HostNetwork, c.queroHostNet)
			}
			if e.WSLMirrored != c.queroWSLEspelho {
				t.Errorf("WSLMirrored = %v, quero %v", e.WSLMirrored, c.queroWSLEspelho)
			}
			if len(e.PublishedRanges) != len(c.queroFaixas) {
				t.Fatalf("PublishedRanges = %v, quero %v", e.PublishedRanges, c.queroFaixas)
			}
			for i := range e.PublishedRanges {
				if e.PublishedRanges[i] != c.queroFaixas[i] {
					t.Errorf("faixa %d = %v, quero %v", i, e.PublishedRanges[i], c.queroFaixas[i])
				}
			}
		})
	}
}
```

- [ ] **Step 2: Rodar o teste e confirmar que falha**

Run: `go test ./internal/reachability/ -run TestDetect -v`
Expected: FAIL na compilação — `detect` não está definida.

- [ ] **Step 3: Escrever a implementação**

Criar `internal/reachability/detect.go`:

```go
package reachability

import (
	"os"
	"runtime"
	"strings"
)

// leitor isola as leituras de sistema de arquivos para que detect possa
// ser testada sem depender do ambiente real da máquina de build.
type leitor interface {
	existe(path string) bool
	ler(path string) ([]byte, error)
}

type leitorReal struct{}

func (leitorReal) existe(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (leitorReal) ler(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// Detect fotografa o ambiente. Chamada uma vez na inicialização.
func Detect() Environment {
	return detect(leitorReal{}, os.Getenv, runtime.GOOS, limiteArquivosAbertos())
}

func detect(fs leitor, getenv func(string) string, goos string, maxOpenFiles uint64) Environment {
	e := Environment{MaxOpenFiles: maxOpenFiles}

	switch {
	case fs.existe("/.dockerenv"):
		e.Kind = KindDocker
		e.HostNetwork = getenv("HOST_NETWORK") == "1"
		if !e.HostNetwork {
			if faixas, err := ParseRanges(getenv("PUBLISHED_PORT_RANGE")); err == nil {
				e.PublishedRanges = faixas
				e.RangesKnown = true
			}
		}

	case ehWSL(fs):
		e.Kind = KindWSL
		e.WSLMirrored = wslEspelhado(fs)

	case goos == "windows":
		e.Kind = KindWindows

	default:
		e.Kind = KindLinux
	}

	return e
}

func ehWSL(fs leitor) bool {
	conteudo, err := fs.ler("/proc/version")
	if err != nil {
		return false
	}
	v := strings.ToLower(string(conteudo))
	return strings.Contains(v, "microsoft") || strings.Contains(v, "wsl")
}

// wslEspelhado usa a interface loopback0, criada pelo WSL2 quando
// networkingMode=mirrored está ligado. O passo de verificação empírica do
// plano confirma esse marcador antes de confiar nele.
func wslEspelhado(fs leitor) bool {
	return fs.existe("/sys/class/net/loopback0/address")
}
```

Criar também `internal/reachability/rlimit_unix.go`:

```go
//go:build !windows

package reachability

import "syscall"

func limiteArquivosAbertos() uint64 {
	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &lim); err != nil {
		return 0
	}
	return uint64(lim.Cur)
}
```

E `internal/reachability/rlimit_windows.go`:

```go
//go:build windows

package reachability

// O Windows não tem RLIMIT_NOFILE. Zero significa "não se aplica".
func limiteArquivosAbertos() uint64 { return 0 }
```

- [ ] **Step 4: Rodar o teste e confirmar que passa**

Run: `go test ./internal/reachability/ -v`
Expected: PASS.

- [ ] **Step 5: Verificar empiricamente o marcador de modo espelhado do WSL2**

Esta é a pendência 3 da spec. Numa WSL2 **sem** `networkingMode=mirrored`:

Run: `wsl -e ls /sys/class/net/`
Expected: lista **sem** `loopback0` (normalmente `eth0`, `lo`, `sit0`)

Depois acrescentar ao `%USERPROFILE%\.wslconfig` do Windows:

```ini
[wsl2]
networkingMode=mirrored
```

Run: `wsl --shutdown` e, depois de reabrir, `wsl -e ls /sys/class/net/`
Expected: lista **com** `loopback0`

Se o marcador não se confirmar, trocar `wslEspelhado` por `func wslEspelhado(leitor) bool { return false }` com um comentário explicando que o modo não é detectável, e ajustar o texto do aviso da Task 5 para a forma condicional: "se esta WSL não estiver em modo espelhado, só esta máquina alcança os emuladores". Registrar a decisão no commit.

- [ ] **Step 6: Rodar a suíte inteira**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/reachability/
git commit -m "$(cat <<'EOF'
feat(reachability): detect docker, WSL and native environments

De dentro de um container não há como enxergar o mapeamento de portas do
host, então a única fonte confiável é a variável PUBLISHED_PORT_RANGE que
o compose grava. Ausente ou malformada, RangesKnown fica falso e a
aplicação diz que não conseguiu verificar, em vez de acusar todo
dispositivo por causa de um typo no compose.

As leituras de sistema de arquivos ficam atrás de uma interface pequena
para que a matriz de ambientes seja testável numa máquina só.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Manager guarda o último erro de bind

Em ambiente nativo o sinal de alcançabilidade é o bind ter funcionado. O erro já existe em `manager.go:443`, mas morre ali: vira erro de retorno e some. A Task 4 precisa dele por dispositivo.

**Files:**
- Modify: `internal/emulator/manager.go:19-45` (campo novo no `Manager`)
- Modify: `internal/emulator/manager.go:433-449` (registro do erro)
- Test: `internal/emulator/manager_bind_test.go` (criar)

**Interfaces:**
- Consumes: nada das tarefas anteriores.
- Produces:
  - `func (m *Manager) LastStartError(deviceID int) string` — string vazia quando o último start funcionou ou nunca houve start
  - `func (m *Manager) recordStartResult(deviceID int, err error)` — não exportada, chamada internamente

- [ ] **Step 1: Escrever o teste que falha**

Criar `internal/emulator/manager_bind_test.go`:

```go
package emulator

import (
	"errors"
	"testing"
)

// TestLastStartError garante que o erro de abertura de porta sobrevive à
// chamada de start. Sem isso, "address already in use" morre no log e a
// tela de dispositivos não tem como avisar que aquele emulador não está
// alcançável.
func TestLastStartError(t *testing.T) {
	m := &Manager{}

	if got := m.LastStartError(42); got != "" {
		t.Errorf("antes de qualquer start: %q, quero vazio", got)
	}

	m.recordStartResult(42, errors.New("listen tcp 0.0.0.0:4200: address already in use"))
	if got := m.LastStartError(42); got == "" {
		t.Error("depois de um start com erro: vazio, quero a mensagem do erro")
	}

	m.recordStartResult(42, nil)
	if got := m.LastStartError(42); got != "" {
		t.Errorf("depois de um start bem-sucedido: %q, quero vazio", got)
	}
}

// TestLastStartErrorIsolaDispositivos: o erro de um dispositivo não pode
// contaminar o veredito de outro.
func TestLastStartErrorIsolaDispositivos(t *testing.T) {
	m := &Manager{}
	m.recordStartResult(1, errors.New("boom"))
	m.recordStartResult(2, nil)

	if m.LastStartError(1) == "" {
		t.Error("dispositivo 1: quero erro registrado")
	}
	if got := m.LastStartError(2); got != "" {
		t.Errorf("dispositivo 2: %q, quero vazio", got)
	}
}
```

- [ ] **Step 2: Rodar o teste e confirmar que falha**

Run: `go test ./internal/emulator/ -run TestLastStartError -v`
Expected: FAIL na compilação — `recordStartResult` e `LastStartError` não existem.

- [ ] **Step 3: Acrescentar o campo ao Manager**

Em `internal/emulator/manager.go`, dentro do `type Manager struct`, logo depois do bloco do watchdog:

```go
	// Último erro de abertura de porta por dispositivo. É o sinal de
	// alcançabilidade em ambiente nativo, onde o bind é direto.
	startErrors     map[int]string
	startErrorMutex sync.RWMutex
```

- [ ] **Step 4: Escrever os dois métodos**

Ainda em `internal/emulator/manager.go`, ao lado dos outros acessores:

```go
// recordStartResult guarda ou limpa o erro do último start do dispositivo.
// Um start bem-sucedido apaga o erro anterior: o veredito precisa refletir
// a última tentativa, não a pior.
func (m *Manager) recordStartResult(deviceID int, err error) {
	m.startErrorMutex.Lock()
	defer m.startErrorMutex.Unlock()

	if m.startErrors == nil {
		m.startErrors = make(map[int]string)
	}

	if err == nil {
		delete(m.startErrors, deviceID)
		return
	}
	m.startErrors[deviceID] = err.Error()
}

// LastStartError devolve o erro do último start do dispositivo, ou string
// vazia se o último start funcionou ou se nunca houve start.
func (m *Manager) LastStartError(deviceID int) string {
	m.startErrorMutex.RLock()
	defer m.startErrorMutex.RUnlock()
	return m.startErrors[deviceID]
}
```

- [ ] **Step 5: Chamar no ponto de start**

Em `internal/emulator/manager.go`, substituir o bloco das linhas 439-449 por:

```go
	// Aguardar inicialização com timeout de 10 segundos
	select {
	case err := <-startErrChan:
		m.recordStartResult(id, err)
		if err != nil {
			return fmt.Errorf("failed to start emulator: %w", err)
		}
	case <-time.After(10 * time.Second):
		// Tentar parar o emulador que pode estar travado
		_ = emulator.Stop()
		erroTimeout := fmt.Errorf("timeout starting emulator %d after 10 seconds", id)
		m.recordStartResult(id, erroTimeout)
		return erroTimeout
	}
```

- [ ] **Step 6: Rodar os testes e confirmar que passam**

Run: `go test ./internal/emulator/ -run TestLastStartError -v`
Expected: PASS

Run: `go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/emulator/manager.go internal/emulator/manager_bind_test.go
git commit -m "$(cat <<'EOF'
feat(emulator): keep the last bind error per device

Em ambiente nativo o bind é direto, então "a porta abriu" é o próprio
teste de alcançabilidade. O erro já existia no start, mas virava retorno e
sumia — "address already in use" morria no log, sem chegar na tela.

Start bem-sucedido apaga o erro anterior: o veredito reflete a última
tentativa, não a pior.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Endpoint `GET /api/reachability`

**Files:**
- Modify: `internal/handlers/handlers.go:24-33` (campo no `Handler`)
- Modify: `internal/handlers/handlers.go:35-40` (`NewHandler`)
- Modify: `internal/handlers/handlers.go:263-266` (registro da rota)
- Create: `internal/handlers/reachability.go`
- Test: `internal/handlers/reachability_http_test.go`

**Interfaces:**
- Consumes: `reachability.Detect`, `reachability.Analyze`, `reachability.Report` (Tasks 1 e 2); `manager.ListDevicesWithFilters` e `manager.LastStartError` (Task 3).
- Produces:
  - Campo `env reachability.Environment` no `Handler`
  - `func (h *Handler) getReachability(c *gin.Context)`
  - `GET /api/reachability` → JSON do `reachability.Report`, sempre HTTP 200 quando o banco responde

- [ ] **Step 1: Escrever o teste que falha**

Criar `internal/handlers/reachability_http_test.go`:

```go
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"GoFacialEmulator/internal/reachability"

	"github.com/gin-gonic/gin"
)

// TestGetReachability_ContratoHTTP fixa a forma do JSON que a tela de
// dispositivos consome. O aviso da interface lê "unreachable" e a lista
// "devices": mudar qualquer um dos dois quebra a tela em silêncio.
func TestGetReachability_ContratoHTTP(t *testing.T) {
	env := reachability.Environment{
		Kind:            reachability.KindDocker,
		PublishedRanges: []reachability.PortRange{{Start: 4000, End: 4099}},
		RangesKnown:     true,
	}
	portas := []reachability.DevicePort{
		{DeviceID: 1, Port: 4001, Started: true},
		{DeviceID: 2, Port: 4200, Started: true},
	}

	h := &Handler{env: env}
	r := gin.New()
	r.GET("/api/reachability", func(c *gin.Context) {
		c.JSON(http.StatusOK, reachability.Analyze(portas, h.env))
	})

	req := httptest.NewRequest(http.MethodGet, "/api/reachability", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200", w.Code)
	}

	var corpo struct {
		Unreachable int `json:"unreachable"`
		Unknown     int `json:"unknown"`
		Devices     []struct {
			DeviceID int    `json:"device_id"`
			Port     int    `json:"port"`
			Status   string `json:"status"`
			Reason   string `json:"reason"`
		} `json:"devices"`
		Environment struct {
			Kind int `json:"kind"`
		} `json:"environment"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("JSON inválido: %v — corpo: %s", err, w.Body.String())
	}

	if corpo.Unreachable != 1 {
		t.Errorf("unreachable = %d, quero 1", corpo.Unreachable)
	}
	if len(corpo.Devices) != 2 {
		t.Fatalf("len(devices) = %d, quero 2", len(corpo.Devices))
	}
	if corpo.Devices[0].Status != "ok" {
		t.Errorf("devices[0].status = %q, quero \"ok\"", corpo.Devices[0].Status)
	}
	if corpo.Devices[1].Status != "inalcancavel" {
		t.Errorf("devices[1].status = %q, quero \"inalcancavel\"", corpo.Devices[1].Status)
	}
	if corpo.Devices[1].Reason == "" {
		t.Error("devices[1].reason vazio, quero a explicação em português")
	}
}
```

- [ ] **Step 2: Rodar o teste e confirmar que falha**

Run: `go test ./internal/handlers/ -run TestGetReachability -v`
Expected: FAIL na compilação — o `Handler` não tem campo `env`.

- [ ] **Step 3: Acrescentar o campo e preencher em NewHandler**

Em `internal/handlers/handlers.go`, no `type Handler struct`, depois de `metrics`:

```go
	env           reachability.Environment
```

No `import`, acrescentar `"GoFacialEmulator/internal/reachability"`.

Em `NewHandler`, antes do `return`, e no literal de retorno acrescentar `env: reachability.Detect(),`.

- [ ] **Step 4: Escrever o handler**

Criar `internal/handlers/reachability.go`:

```go
package handlers

import (
	"net/http"

	"GoFacialEmulator/internal/reachability"

	"github.com/gin-gonic/gin"
)

// getReachability responde quais dispositivos não vão ser alcançados pelo
// Site Controller neste ambiente. É a informação que falta hoje quando a
// tela mostra tudo verde e o W-Access mostra tudo offline.
func (h *Handler) getReachability(c *gin.Context) {
	dispositivos, err := h.manager.ListDevicesWithFilters(map[string]string{})
	if err != nil {
		h.tracer.Error("Failed to list devices for reachability: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Não foi possível verificar as portas dos dispositivos",
		})
		return
	}

	portas := make([]reachability.DevicePort, 0, len(dispositivos))
	for _, d := range dispositivos {
		portas = append(portas, reachability.DevicePort{
			DeviceID:  d.ID,
			Port:      d.Port,
			Started:   d.Status == "running",
			BindError: h.manager.LastStartError(d.ID),
		})
	}

	c.JSON(http.StatusOK, reachability.Analyze(portas, h.env))
}
```

- [ ] **Step 5: Registrar a rota**

Em `internal/handlers/handlers.go`, no bloco `setupAPIRoutes`, junto das rotas soltas de `api`:

```go
		api.GET("/reachability", h.getReachability)
```

- [ ] **Step 6: Rodar os testes e confirmar que passam**

Run: `go test ./internal/handlers/ -v`
Expected: PASS

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/handlers/reachability.go internal/handlers/reachability_http_test.go internal/handlers/handlers.go
git commit -m "$(cat <<'EOF'
feat(api): serve a port reachability report

GET /api/reachability cruza a porta que o W-Access pediu para cada
dispositivo com o que o ambiente consegue entregar. O ambiente é
fotografado uma vez na inicialização; o relatório é recalculado a cada
chamada, porque a lista de dispositivos muda.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Aviso na tela de dispositivos

**Files:**
- Modify: `assets/web/templates/devices.html:16-17` (bloco novo entre o cabeçalho e os filtros)
- Modify: `assets/web/static/js/main.js` (busca e renderização)
- Modify: `assets/web/static/css/` (estilo do aviso — arquivo existente do projeto)

**Interfaces:**
- Consumes: `GET /api/reachability` da Task 4.
- Produces: elemento `#reachability-alert` na página de dispositivos, escondido quando não há o que avisar. A Task 8 da fase D fotografa esse elemento.

- [ ] **Step 1: Acrescentar o bloco ao template**

Em `assets/web/templates/devices.html`, entre o `</div>` da linha 16 e o `<div class="row mt-2">` da linha 18:

```html
  <div class="row" id="reachability-alert-row" style="display: none;">
    <div class="col-12">
      <div class="alert alert-warning d-flex align-items-start" id="reachability-alert" role="alert">
        <i class="bi bi-exclamation-triangle-fill me-2 mt-1"></i>
        <div class="flex-grow-1">
          <div id="reachability-headline" class="fw-semibold"></div>
          <div id="reachability-reason" class="small"></div>
          <div id="reachability-help" class="small"></div>
          <button type="button" class="btn btn-link btn-sm p-0 mt-1" id="reachability-toggle"
                  onclick="toggle_reachability_list()">ver a lista</button>
          <div id="reachability-list" class="small mt-2" style="display: none;"></div>
        </div>
      </div>
    </div>
  </div>
```

- [ ] **Step 2: Escrever o JavaScript**

Em `assets/web/static/js/main.js`, acrescentar:

```javascript
// Aviso de alcançabilidade: mostra quais dispositivos não vão ser
// alcançados pelo Site Controller neste ambiente. Sem ele, a tela fica
// toda verde enquanto o W-Access mostra tudo offline.
async function carregar_alcancabilidade() {
  const linha = document.getElementById("reachability-alert-row");
  if (!linha) return;

  let relatorio;
  try {
    const resposta = await fetch("/api/reachability");
    if (!resposta.ok) return;
    relatorio = await resposta.json();
  } catch (e) {
    return;
  }

  const problemas = (relatorio.devices || []).filter((d) => d.status !== "ok");
  if (problemas.length === 0) {
    linha.style.display = "none";
    return;
  }

  const inalcancaveis = relatorio.unreachable || 0;
  const desconhecidos = relatorio.unknown || 0;

  const titulo =
    inalcancaveis > 0
      ? `${inalcancaveis} dispositivo(s) não vão ser alcançados pelo Site Controller.`
      : `Não foi possível verificar ${desconhecidos} dispositivo(s).`;

  document.getElementById("reachability-headline").textContent = titulo;
  document.getElementById("reachability-reason").textContent = problemas[0].reason || "";
  document.getElementById("reachability-help").textContent =
    "O que fazer: veja o capítulo Portas e rede do manual.";

  document.getElementById("reachability-list").innerHTML = problemas
    .map((d) => `Dispositivo ${d.device_id} — porta ${d.port}: ${d.reason}`)
    .join("<br>");

  linha.style.display = "";
}

function toggle_reachability_list() {
  const lista = document.getElementById("reachability-list");
  const botao = document.getElementById("reachability-toggle");
  const escondida = lista.style.display === "none";
  lista.style.display = escondida ? "" : "none";
  botao.textContent = escondida ? "esconder a lista" : "ver a lista";
}

document.addEventListener("DOMContentLoaded", carregar_alcancabilidade);
```

- [ ] **Step 3: Verificar na aplicação rodando**

Run: `go run cmd/emulator-service/main.go -config configs/config.yaml`

Abrir http://localhost:7070 e confirmar:
- Sem dispositivo problemático, o bloco não aparece.
- Forçando o cenário — subir com `PUBLISHED_PORT_RANGE=4000-4099` no ambiente e dispositivos acima de 4099 no W-Access — o aviso aparece com a contagem certa, e `ver a lista` expande e recolhe.

- [ ] **Step 4: Commit**

```bash
git add assets/web/templates/devices.html assets/web/static/js/main.js
git commit -m "$(cat <<'EOF'
feat(web): warn about devices the Site Controller cannot reach

A falha mais cara do emulador hoje não emite sinal nenhum: os emuladores
sobem, a tela fica toda verde e o W-Access mostra tudo offline. O aviso
aparece acima da lista, diz quantos são e por quê, e aponta o capítulo do
manual.

Sem dispositivo problemático o bloco não aparece — o aviso só existe
quando há o que avisar.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Padrões de rede dos pacotes

**Files:**
- Modify: `packaging/docker/docker-compose.yml`
- Create: `packaging/docker/docker-compose.linux.yml`
- Modify: `packaging/docker/LEIA-ME.txt`
- Modify: `packaging/docker/instalar.sh`, `packaging/docker/iniciar.sh`, `packaging/docker/parar.sh` (escolha do compose no Linux)
- Test: `internal/reachability/compose_test.go` (criar)

**Interfaces:**
- Consumes: `reachability.ParseRanges` da Task 1.
- Produces: `packaging/docker/docker-compose.yml` com `PUBLISHED_PORT_RANGE: "4000-4499"` coerente com `ports`; `docker-compose.linux.yml` com `network_mode: host` e `HOST_NETWORK: "1"`.

- [ ] **Step 1: Escrever o teste que falha**

Criar `internal/reachability/compose_test.go`:

```go
package reachability

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestComposeCoerenteComVariavel: o aviso da tela depende de
// PUBLISHED_PORT_RANGE descrever exatamente o que o compose publica. Se os
// dois divergirem, a aplicação acusa dispositivo saudável ou deixa passar
// dispositivo inalcançável — os dois piores desfechos possíveis para essa
// funcionalidade.
func TestComposeCoerenteComVariavel(t *testing.T) {
	conteudo, err := os.ReadFile("../../packaging/docker/docker-compose.yml")
	if err != nil {
		t.Fatalf("lendo o compose: %v", err)
	}

	var compose struct {
		Services struct {
			App struct {
				Environment map[string]string `yaml:"environment"`
				Ports       []string          `yaml:"ports"`
			} `yaml:"app"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(conteudo, &compose); err != nil {
		t.Fatalf("parseando o compose: %v", err)
	}

	declarado := compose.Services.App.Environment["PUBLISHED_PORT_RANGE"]
	if declarado == "" {
		t.Fatal("PUBLISHED_PORT_RANGE ausente no compose do pacote Docker")
	}

	faixas, err := ParseRanges(declarado)
	if err != nil {
		t.Fatalf("PUBLISHED_PORT_RANGE = %q é inválida: %v", declarado, err)
	}

	// Toda porta publicada que não seja a da interface web precisa estar
	// coberta pela variável.
	for _, p := range compose.Services.App.Ports {
		host, _, _ := cortarMapeamento(p)
		if host == "7070" {
			continue
		}
		publicadas, err := ParseRanges(host)
		if err != nil {
			t.Fatalf("mapeamento %q não pôde ser lido: %v", p, err)
		}
		for _, pub := range publicadas {
			if !cobre(faixas, pub) {
				t.Errorf("o compose publica %s mas PUBLISHED_PORT_RANGE=%q não cobre", pub, declarado)
			}
		}
	}
}

func cortarMapeamento(s string) (host, container string, ok bool) {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			return s[:i], s[i+1:], true
		}
	}
	return s, s, false
}

func cobre(faixas []PortRange, alvo PortRange) bool {
	for _, f := range faixas {
		if f.Contains(alvo.Start) && f.Contains(alvo.End) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Rodar o teste e confirmar que falha**

Run: `go test ./internal/reachability/ -run TestComposeCoerente -v`
Expected: FAIL — `PUBLISHED_PORT_RANGE ausente no compose do pacote Docker`.

- [ ] **Step 3: Medir o custo de publicar 500 portas**

Pendência 2 da spec. Com o Docker Desktop aberto, numa pasta temporária, criar um compose mínimo com `image: postgres:15-alpine`, `command: sleep 600` e `ports: - "4000-4499:4000-4499"`, e medir:

Run: `powershell -NoProfile -Command "Measure-Command { docker compose up -d } | Select-Object -Expand TotalSeconds"`

Anotar o número. Repetir com `4000-4099` para comparar. Se 500 portas passarem de 60 segundos, reduzir o padrão para `4000-4299` e registrar a medição no corpo do commit. O restante do passo assume `4000-4499`; ajustar os três lugares se a medição mandar outra coisa.

Derrubar o teste com `docker compose down` ao final.

- [ ] **Step 4: Atualizar o compose do pacote Docker**

Em `packaging/docker/docker-compose.yml`, no serviço `app`, o bloco `environment` passa a incluir a variável, e `ports` sobe para a faixa medida:

```yaml
    environment:
      SERVER_HOST: "0.0.0.0"
      SERVER_PORT: "7070"
      DB_HOST: postgres
      DB_PORT: "5432"
      DB_USER: emulator
      DB_PASSWORD: emulator123
      DB_DATABASE: emulator_db
      # Precisa descrever exatamente o que a lista ports abaixo publica.
      # A aplicação usa esta variável para avisar quais dispositivos não
      # vão ser alcançados. Um teste de contrato guarda essa coerência.
      PUBLISHED_PORT_RANGE: "4000-4499"
    ports:
      - "7070:7070"
      # 500 emuladores. Para mais, troque as DUAS linhas abaixo e a
      # variável PUBLISHED_PORT_RANGE acima pela mesma faixa.
      # - "4000-4999:4000-4999"
      - "4000-4499:4000-4499"
```

- [ ] **Step 5: Rodar o teste e confirmar que passa**

Run: `go test ./internal/reachability/ -run TestComposeCoerente -v`
Expected: PASS

- [ ] **Step 6: Criar o compose de Linux com rede de host**

Criar `packaging/docker/docker-compose.linux.yml`. O container usa a pilha de rede do host, então não há publicação de portas e o problema desaparece: qualquer porta que o W-Access pedir funciona.

```yaml
# Compose para Docker em Linux. Usa a rede do host, então qualquer porta
# que o W-Access pedir funciona, sem faixa publicada e sem limite.
# No Docker Desktop (Windows/Mac) a rede de host é experimental — lá vale
# o docker-compose.yml, que publica por faixa.
services:
  postgres:
    image: postgres:15-alpine
    container_name: facial-emulator-db
    environment:
      POSTGRES_USER: emulator
      POSTGRES_PASSWORD: emulator123
      POSTGRES_DB: emulator_db
    expose:
      - "5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U emulator -d emulator_db"]
      interval: 5s
      timeout: 3s
      retries: 20
    restart: unless-stopped

  app:
    image: gofacialemulator:1.0
    container_name: facial-emulator-app
    network_mode: host
    environment:
      SERVER_HOST: "0.0.0.0"
      SERVER_PORT: "7070"
      DB_HOST: 127.0.0.1
      DB_PORT: "5432"
      DB_USER: emulator
      DB_PASSWORD: emulator123
      DB_DATABASE: emulator_db
      HOST_NETWORK: "1"
    depends_on:
      postgres:
        condition: service_healthy
    restart: unless-stopped
    volumes:
      - ./logs:/app/logs

volumes:
  postgres_data:
    driver: local
```

Com `network_mode: host` o serviço `app` sai da rede do compose e perde o alias `postgres`, por isso `DB_HOST` vira `127.0.0.1`. Para que isso funcione, o serviço `postgres` precisa publicar a porta no host — acrescentar a ele:

```yaml
    ports:
      - "127.0.0.1:5432:5432"
```

O bind em `127.0.0.1` mantém o banco fora da rede, atendendo à mesma preocupação que motivou o `expose` no compose original.

- [ ] **Step 7: Fazer os scripts bash escolherem o compose**

Nos três scripts `packaging/docker/instalar.sh`, `iniciar.sh` e `parar.sh`, logo depois do `cd "$(dirname "$0")"`:

```bash
# Em Linux usamos a rede do host, que não tem limite de portas. No Docker
# Desktop (Windows/Mac) a rede de host é experimental, então lá vale o
# compose que publica por faixa.
COMPOSE=sistema/docker-compose.yml
if [ "$(uname -s)" = "Linux" ] && ! grep -qiE "microsoft|wsl" /proc/version 2>/dev/null; then
    COMPOSE=sistema/docker-compose.linux.yml
fi
```

E trocar todas as ocorrências de `-f sistema/docker-compose.yml` por `-f "$COMPOSE"` nos três arquivos.

A WSL fica de fora da rede de host de propósito: lá o problema não é publicação de porta, e sim o NAT da própria WSL, que a Task 7 trata.

- [ ] **Step 8: Copiar o compose novo no build**

Em `packaging/build-pacotes.bat`, no bloco `:build_docker`, ao lado da linha que copia o compose:

```bat
copy /Y packaging\docker\docker-compose.linux.yml "%STAGE%\sistema\" >nul
```

- [ ] **Step 9: Atualizar o LEIA-ME do pacote**

Em `packaging/docker/LEIA-ME.txt`, acrescentar antes da seção LOGS:

```
PORTAS
  As portas dos emuladores vem do W-Access, nao do emulador.
  No Windows este pacote publica a faixa 4000-4499. Controlador com
  porta fora dessa faixa sobe mas NAO e alcancado pelo Site Controller.
  Para alargar, edite sistema/docker-compose.yml: a linha ports e a
  variavel PUBLISHED_PORT_RANGE precisam ter a MESMA faixa.
  No Linux nao ha limite: o pacote usa a rede do host.
```

- [ ] **Step 10: Gerar e conferir o pacote**

Run: `packaging\build-pacotes.bat docker`
Expected: `[docker] OK: packaging\.out\GoFacialEmulator-docker.zip`

Run: `powershell -NoProfile -Command "Add-Type -A System.IO.Compression.FileSystem; [IO.Compression.ZipFile]::OpenRead((Resolve-Path packaging\.out\GoFacialEmulator-docker.zip)).Entries | Select-Object -Expand FullName"`
Expected: `sistema/docker-compose.yml` e `sistema/docker-compose.linux.yml` presentes.

- [ ] **Step 11: Rodar a suíte inteira**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 12: Commit**

```bash
git add packaging/ internal/reachability/compose_test.go
git commit -m "$(cat <<'EOF'
fix(packaging): raise the published port range and add host networking on Linux

O ambiente de referência tem 301 controladores nas portas 4001-4301, contra
os 4000-4099 que o pacote publicava: 201 emuladores subiam e ficavam
invisíveis. No Windows a faixa padrão sobe para 4000-4499.

No Linux o compose passa a usar a rede do host, onde o problema deixa de
existir — qualquer porta que o W-Access pedir funciona. O Postgres passa a
publicar em 127.0.0.1 porque o app sai da rede do compose e perde o alias.

Um teste de contrato garante que PUBLISHED_PORT_RANGE e a lista ports
descrevam a mesma faixa: divergir entre as duas faria a aplicação acusar
dispositivo saudável ou deixar passar dispositivo inalcançável.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: `instalar.sh` — firewall, `ulimit` e WSL

**Files:**
- Modify: `packaging/linux/instalar.sh`
- Modify: `packaging/linux/LEIA-ME.txt`

**Interfaces:**
- Consumes: nada do código Go.
- Produces: `instalar.sh` pergunta a faixa de portas (padrão `4000-4499`), libera o firewall, ajusta `ulimit -n` e avisa sobre o modo espelhado da WSL.

- [ ] **Step 1: Perguntar a faixa de portas**

Em `packaging/linux/instalar.sh`, depois do bloco de detecção de WSL e antes de `[1/3]`:

```bash
echo
echo "Qual faixa de portas os emuladores vao usar?"
echo "As portas vem do W-Access (campo BaseCommPort de cada controlador)."
printf "Faixa [4000-4499]: "
read -r FAIXA
FAIXA="${FAIXA:-4000-4499}"

PORTA_INICIO="${FAIXA%%-*}"
PORTA_FIM="${FAIXA##*-}"

if ! echo "$PORTA_INICIO$PORTA_FIM" | grep -qE '^[0-9]+$' || [ "$PORTA_INICIO" -gt "$PORTA_FIM" ]; then
    echo
    echo "❌ Faixa invalida: $FAIXA. Use o formato 4000-4499."
    exit 1
fi

QTD_PORTAS=$((PORTA_FIM - PORTA_INICIO + 1))
```

- [ ] **Step 2: Liberar o firewall**

Logo depois, ainda antes de `[1/3]`, renumerando os passos existentes para `[1/5]` a `[3/5]`:

```bash
echo "[4/5] Liberando as portas no firewall ..."
if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi "^Status: active"; then
    if ufw allow "${PORTA_INICIO}:${PORTA_FIM}/tcp" >>"$LOG" 2>&1; then
        echo "      ufw: liberado ${PORTA_INICIO}-${PORTA_FIM}/tcp."
    else
        echo "      ⚠  Nao foi possivel liberar no ufw — veja sistema/logs/instalacao.log"
    fi
elif command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1; then
    if firewall-cmd --permanent --add-port="${PORTA_INICIO}-${PORTA_FIM}/tcp" >>"$LOG" 2>&1 \
       && firewall-cmd --reload >>"$LOG" 2>&1; then
        echo "      firewalld: liberado ${PORTA_INICIO}-${PORTA_FIM}/tcp."
    else
        echo "      ⚠  Nao foi possivel liberar no firewalld — veja sistema/logs/instalacao.log"
    fi
else
    echo "      Nenhum firewall ativo. Nada a fazer."
fi
```

Rodar duas vezes não é problema: `ufw allow` e `firewall-cmd --permanent --add-port` são idempotentes.

- [ ] **Step 3: Ajustar o limite de arquivos abertos**

```bash
echo "[5/5] Conferindo o limite de arquivos abertos ..."
# Cada emulador abre um socket de escuta e mantém conexões. A folga de
# 2 vezes o número de portas mais 1024 cobre as conexões simultâneas.
NECESSARIO=$((QTD_PORTAS * 2 + 1024))
ATUAL="$(ulimit -n)"

if [ "$ATUAL" -lt "$NECESSARIO" ]; then
    cat > /etc/security/limits.d/gofacialemulator.conf <<EOF
# Gerado pelo instalar.sh do GoFacialEmulator para $QTD_PORTAS emuladores.
* soft nofile $NECESSARIO
* hard nofile $NECESSARIO
EOF
    echo "      Limite atual ($ATUAL) e menor que o necessario ($NECESSARIO)."
    echo "      ⚠  Ajustado em /etc/security/limits.d/gofacialemulator.conf."
    echo "         Saia e entre de novo na sessao antes de rodar ./iniciar.sh."
else
    echo "      Limite atual ($ATUAL) e suficiente."
fi
```

- [ ] **Step 4: Avisar sobre o modo espelhado da WSL**

Antes da linha final de sucesso:

```bash
if [ "$NO_WSL" -eq 1 ] && [ ! -e /sys/class/net/loopback0 ]; then
    echo
    echo "⚠  ATENCAO — esta WSL nao esta em modo espelhado."
    echo "   Sem isso, so ESTA maquina alcanca os emuladores: o Site"
    echo "   Controller em outro computador nao vai conseguir conectar."
    echo
    echo "   Para corrigir, crie ou edite o arquivo .wslconfig na pasta"
    echo "   do seu usuario no Windows (C:\\Users\\SEU_USUARIO\\.wslconfig)"
    echo "   com o conteudo:"
    echo
    echo "     [wsl2]"
    echo "     networkingMode=mirrored"
    echo
    echo "   Depois, no PowerShell do Windows: wsl --shutdown"
    echo "   e abra a WSL de novo."
    echo
fi
```

Se o Step 5 da Task 2 tiver desmentido o marcador `loopback0`, trocar a condição por `[ "$NO_WSL" -eq 1 ]` e reescrever a primeira linha na forma condicional: `⚠  ATENCAO — se esta WSL nao estiver em modo espelhado ...`.

- [ ] **Step 5: Atualizar o LEIA-ME do pacote**

Em `packaging/linux/LEIA-ME.txt`, acrescentar antes da seção LOGS:

```
PORTAS
  As portas dos emuladores vem do W-Access, nao do emulador.
  O instalar.sh pergunta a faixa e libera no firewall.
  Em WSL2, o Site Controller de outra maquina so alcanca os emuladores
  com o modo espelhado ligado. O instalar.sh avisa e mostra como fazer.
```

- [ ] **Step 6: Testar o instalador**

Numa WSL2 ou VM Linux descartável, com o ZIP extraído:

Run: `sudo bash instalar.sh` aceitando o padrão da faixa
Expected: termina em `✅ Instalado. Rode ./iniciar.sh`, tendo impresso as cinco etapas

Run: `sudo bash instalar.sh` de novo
Expected: mesma saída, sem erro — o firewall e o `limits.d` são idempotentes

Run: `cat /etc/security/limits.d/gofacialemulator.conf`
Expected: o arquivo existe com o valor calculado, ou o script informou que o limite atual já bastava

Em WSL sem modo espelhado, conferir que o bloco de aviso apareceu.

- [ ] **Step 7: Commit**

```bash
git add packaging/linux/
git commit -m "$(cat <<'EOF'
feat(packaging): open the firewall, raise ulimit and warn about WSL NAT

Três armadilhas do pacote Linux que só apareciam depois, como "o Site
Controller não conecta": firewall fechado na faixa dos emuladores, ulimit
de 1024 contra centenas de sockets de escuta, e o NAT da WSL2.

A WSL é o caso mais traiçoeiro: localhost funciona, a tela abre, tudo
parece certo — e nenhuma outra máquina alcança os emuladores. O instalador
detecta e mostra o bloco de .wslconfig para colar.

A faixa é perguntada na instalação, com 4000-4499 ao dar Enter.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: Modo online/standalone — backend

Fase C da spec. Os dois modos já existem e são controlados por `LocalAuthentication` em `emulator.device_settings` (`0` = online, o Site Controller valida; `1` = standalone, o dispositivo valida e gera o evento). Hoje só o Site Controller troca esse valor.

**Files:**
- Create: `internal/handlers/device_mode.go`
- Create: `internal/handlers/device_mode_test.go`
- Modify: `internal/handlers/handlers.go:256-261` (grupo de rotas `devices`)

**Interfaces:**
- Consumes: `h.serviceDB database.DBInterface`.
- Produces:
  - `func (h *Handler) getDeviceMode(ctx context.Context, deviceID int) (string, error)` — `"online"` ou `"standalone"`
  - `func (h *Handler) setDeviceMode(ctx context.Context, deviceID int, mode string) error`
  - `GET /api/devices/:id/mode` → `{"mode":"online"}`
  - `POST /api/devices/:id/mode` com corpo `{"mode":"standalone"}` → `{"mode":"standalone"}`

- [ ] **Step 1: Escrever os testes que falham**

Criar `internal/handlers/device_mode_test.go`:

```go
package handlers

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// linhaFalsa devolve um valor fixo no Scan, para exercitar
// getDeviceMode sem banco.
type linhaFalsa struct {
	valor string
	err   error
}

func (l linhaFalsa) Scan(dest ...any) error {
	if l.err != nil {
		return l.err
	}
	p, ok := dest[0].(*string)
	if !ok {
		return errors.New("destino não é *string")
	}
	*p = l.valor
	return nil
}

// TestGetDeviceMode traduz o LocalAuthentication do banco para o
// vocabulário da interface. A tradução é o ponto: "0" e "1" não dizem nada
// para quem opera.
func TestGetDeviceMode(t *testing.T) {
	casos := []struct {
		nome  string
		valor string
		quero string
	}{
		{"zero é online", "0", "online"},
		{"um é standalone", "1", "standalone"},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h := &Handler{serviceDB: &dbFalso{linha: linhaFalsa{valor: c.valor}}}
			tenho, err := h.getDeviceMode(context.Background(), 1)
			if err != nil {
				t.Fatalf("getDeviceMode: %v", err)
			}
			if tenho != c.quero {
				t.Errorf("modo = %q, quero %q", tenho, c.quero)
			}
		})
	}
}

// TestSetDeviceModeRecusaValorInvalido: um modo desconhecido não pode
// chegar ao banco, senão grava lixo em LocalAuthentication.
func TestSetDeviceModeRecusaValorInvalido(t *testing.T) {
	db := &dbFalso{}
	h := &Handler{serviceDB: db}

	err := h.setDeviceMode(context.Background(), 1, "turbo")
	if err == nil {
		t.Fatal("setDeviceMode(\"turbo\") = nil, quero erro")
	}
	if db.execChamado {
		t.Error("o banco foi chamado com um modo inválido")
	}
}

// TestSetDeviceModeGravaOValorCerto confere a tradução na direção oposta.
func TestSetDeviceModeGravaOValorCerto(t *testing.T) {
	casos := []struct {
		modo  string
		quero string
	}{
		{"online", "0"},
		{"standalone", "1"},
	}

	for _, c := range casos {
		t.Run(c.modo, func(t *testing.T) {
			db := &dbFalso{}
			h := &Handler{serviceDB: db}

			if err := h.setDeviceMode(context.Background(), 7, c.modo); err != nil {
				t.Fatalf("setDeviceMode: %v", err)
			}
			if !db.execChamado {
				t.Fatal("o banco não foi chamado")
			}
			if !strings.Contains(db.execArgsTexto(), c.quero) {
				t.Errorf("argumentos = %s, quero conter %q", db.execArgsTexto(), c.quero)
			}
		})
	}
}
```

Criar também, no mesmo arquivo, o duplo de banco. `database.DBInterface` (definida em `internal/database/connection.go:11`) tem cinco métodos; só `QueryRow` e `Exec` são exercitados, os outros três existem para satisfazer a interface:

```go
// dbFalso implementa database.DBInterface para os testes deste arquivo.
// Só QueryRow e Exec importam; Query, Begin e Ping existem para satisfazer
// a interface e devolvem zero.
type dbFalso struct {
	linha       linhaFalsa
	execChamado bool
	execArgs    []interface{}
}

func (d *dbFalso) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	return d.linha
}

func (d *dbFalso) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	d.execChamado = true
	d.execArgs = args
	return pgconn.CommandTag{}, nil
}

func (d *dbFalso) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	return nil, nil
}
func (d *dbFalso) Begin(ctx context.Context) (pgx.Tx, error) { return nil, nil }
func (d *dbFalso) Ping(ctx context.Context) error            { return nil }

// execArgsTexto junta os argumentos do Exec em uma string, para que o
// teste possa afirmar sobre o valor gravado sem depender da posição.
func (d *dbFalso) execArgsTexto() string {
	partes := make([]string, 0, len(d.execArgs))
	for _, a := range d.execArgs {
		partes = append(partes, fmt.Sprint(a))
	}
	return strings.Join(partes, " ")
}
```

O bloco de imports do arquivo de teste fica:

```go
import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)
```

`linhaFalsa` satisfaz `pgx.Row` porque essa interface tem um método só, `Scan(dest ...any) error`.

- [ ] **Step 2: Rodar os testes e confirmar que falham**

Run: `go test ./internal/handlers/ -run TestDeviceMode -v`
Expected: FAIL na compilação — `getDeviceMode` e `setDeviceMode` não existem.

- [ ] **Step 3: Escrever a implementação**

Criar `internal/handlers/device_mode.go`:

```go
package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Os dois modos de operação de um dispositivo, gravados em
// emulator.device_settings.LocalAuthentication:
//
//	online     (0) — o Site Controller valida o acesso e responde
//	standalone (1) — o dispositivo valida sozinho e gera o evento
const (
	modoOnline     = "online"
	modoStandalone = "standalone"
)

func modoParaBanco(modo string) (string, error) {
	switch modo {
	case modoOnline:
		return "0", nil
	case modoStandalone:
		return "1", nil
	default:
		return "", fmt.Errorf("modo inválido: %q", modo)
	}
}

func bancoParaModo(valor string) string {
	if valor == "1" {
		return modoStandalone
	}
	return modoOnline
}

func (h *Handler) getDeviceMode(ctx context.Context, deviceID int) (string, error) {
	var valor string
	err := h.serviceDB.QueryRow(ctx,
		`SELECT COALESCE("LocalAuthentication", '0')
		   FROM emulator.device_settings
		  WHERE local_controller_id = $1`, deviceID).Scan(&valor)
	if err != nil {
		return "", err
	}
	return bancoParaModo(valor), nil
}

func (h *Handler) setDeviceMode(ctx context.Context, deviceID int, modo string) error {
	valor, err := modoParaBanco(modo)
	if err != nil {
		return err
	}

	_, err = h.serviceDB.Exec(ctx,
		`UPDATE emulator.device_settings
		    SET "LocalAuthentication" = $1
		  WHERE local_controller_id = $2`, valor, deviceID)
	return err
}

// apiGetDeviceMode responde o modo atual do dispositivo.
func (h *Handler) apiGetDeviceMode(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id inválido"})
		return
	}

	modo, err := h.getDeviceMode(c.Request.Context(), id)
	if err != nil {
		h.tracer.Error("Failed to read device mode for %d: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Não foi possível ler o modo do dispositivo"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"mode": modo})
}

// apiSetDeviceMode troca o modo do dispositivo.
func (h *Handler) apiSetDeviceMode(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id inválido"})
		return
	}

	var corpo struct {
		Mode string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&corpo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "corpo inválido"})
		return
	}

	if err := h.setDeviceMode(c.Request.Context(), id, corpo.Mode); err != nil {
		h.tracer.Error("Failed to set device mode for %d: %v", id, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"mode": corpo.Mode})
}
```

Confirmar as assinaturas de `QueryRow` e `Exec` em `internal/database/` e ajustar as chamadas acima se divergirem — o `dbFalso` do teste precisa casar com a mesma interface.

- [ ] **Step 4: Registrar as rotas**

Em `internal/handlers/handlers.go`, no grupo `devices`:

```go
			devices.GET("/:id/mode", h.apiGetDeviceMode)
			devices.POST("/:id/mode", h.apiSetDeviceMode)
```

- [ ] **Step 5: Rodar os testes e confirmar que passam**

Run: `go test ./internal/handlers/ -v`
Expected: PASS

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/handlers/device_mode.go internal/handlers/device_mode_test.go internal/handlers/handlers.go
git commit -m "$(cat <<'EOF'
feat(api): expose the online/standalone device mode

Os dois modos já existiam em LocalAuthentication, mas só o Site Controller
conseguia trocar o valor — não dava para testar o comportamento standalone
sem ter um SC de pé.

A tradução acontece na fronteira: o banco continua com 0 e 1, a API fala
"online" e "standalone". Modo desconhecido é recusado antes de chegar ao
banco.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: Coluna Modo na tela de dispositivos

Última mudança visível antes dos manuais.

**Files:**
- Modify: `internal/handlers/handlers.go:952-970` (mapa de dispositivo em `getCurrentDevicesWithFilters`)
- Modify: `assets/web/templates/devices.html:87-101` (cabeçalho da tabela) e o corpo da tabela
- Modify: `assets/web/static/js/main.js`

**Interfaces:**
- Consumes: `getDeviceMode` da Task 8.
- Produces: chave `local_auth` (`"online"` ou `"standalone"`) no mapa de cada dispositivo; coluna **Modo** com um `select` por linha.

- [ ] **Step 1: Acrescentar a chave ao mapa de dispositivo**

Em `internal/handlers/handlers.go`, dentro do laço de `getCurrentDevicesWithFilters`, antes do `append`:

```go
		// Modo de operação. Falha de leitura não derruba a listagem: a
		// coluna mostra o padrão e o erro fica no log.
		modo := modoOnline
		if m, err := h.getDeviceMode(context.Background(), device.ID); err == nil {
			modo = m
		} else {
			h.tracer.Error("Failed to read mode for device %d: %v", device.ID, err)
		}
```

E no literal do mapa, junto das outras chaves:

```go
			"local_auth":  modo,
```

- [ ] **Step 2: Acrescentar a coluna ao cabeçalho**

Em `assets/web/templates/devices.html`, no `<thead>` da tabela `#device-table`, entre `Port` e `Log`:

```html
        <th scope="col" class="text-center">Modo</th>
```

- [ ] **Step 3: Acrescentar a célula ao corpo da tabela**

Na linha de cada dispositivo, na mesma posição:

```html
        <td class="text-center">
          <select class="form-select form-select-sm device-mode"
                  data-device-id="{{ .lc_id }}"
                  onchange="trocar_modo(this)">
            <option value="online" {{ if eq .local_auth "online" }}selected{{ end }}>Online</option>
            <option value="standalone" {{ if eq .local_auth "standalone" }}selected{{ end }}>Standalone</option>
          </select>
        </td>
```

- [ ] **Step 4: Escrever o JavaScript**

Em `assets/web/static/js/main.js`:

```javascript
// Troca o modo de operação do dispositivo. Online: o Site Controller
// valida o acesso. Standalone: o dispositivo valida sozinho e gera o
// evento.
async function trocar_modo(select) {
  const id = select.dataset.deviceId;
  const modo = select.value;
  const anterior = modo === "online" ? "standalone" : "online";

  select.disabled = true;
  try {
    const resposta = await fetch(`/api/devices/${id}/mode`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ mode: modo }),
    });
    if (!resposta.ok) {
      select.value = anterior;
      alert("Não foi possível trocar o modo do dispositivo " + id + ".");
    }
  } catch (e) {
    select.value = anterior;
    alert("Não foi possível trocar o modo do dispositivo " + id + ".");
  } finally {
    select.disabled = false;
  }
}
```

- [ ] **Step 5: Verificar na aplicação rodando**

Run: `go run cmd/emulator-service/main.go -config configs/config.yaml`

Abrir http://localhost:7070 e confirmar:
- A coluna **Modo** aparece com o valor atual de cada dispositivo.
- Trocar o valor persiste: recarregar a página mantém a escolha.
- `curl -s http://localhost:7070/api/devices/109/mode` devolve o mesmo valor mostrado na tela.

- [ ] **Step 6: Rodar a suíte inteira**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/handlers/handlers.go assets/web/templates/devices.html assets/web/static/js/main.js
git commit -m "$(cat <<'EOF'
feat(web): add the online/standalone mode column

Fecha o item 2 do new_tasks.md: dá para testar o comportamento standalone
sem um Site Controller de pé, trocando o modo direto na tela.

Falha de leitura do modo não derruba a listagem — a coluna mostra o padrão
e o erro vai para o log. Uma tela de dispositivos em branco seria pior que
uma coluna imprecisa.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

## Cobertura da spec

| Seção da spec | Tarefa |
|---|---|
| 6.1 Princípio (obedece o BaseCommPort) | Constraint global; Tasks 1 e 4 |
| 6.2 Detecção de ambiente | Task 2, com a verificação empírica da WSL no Step 5 |
| 6.3 Relatório de alcançabilidade | Task 1 (`Analyze`), Task 3 (erro de bind) |
| 6.4 Exposição (endpoint e aviso) | Tasks 4 e 5 |
| 6.5 Padrão de portas do Docker | Task 6, com a medição no Step 3 |
| 6.6 `instalar.sh` | Task 7 |
| 7 Modo online/standalone | Tasks 8 e 9 |
| 8 Manuais (fase D) | Plano próprio, depois deste |

Pendências da spec resolvidas aqui: a 2 (medição do `up` com 500 portas) no Step 3 da Task 6, e a 3 (marcador de modo espelhado da WSL2) no Step 5 da Task 2. A pendência 1 (capturas humanas) pertence à fase D.
