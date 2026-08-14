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
		{
			nome:        "docker: bind falhou dentro do container, porta publicada",
			portas:      []DevicePort{{DeviceID: 10, Port: 4001, Started: true, BindError: "address already in use"}},
			env:         dockerPublicado,
			queroStatus: []Status{StatusUnreachable},
			queroReason: []string{"a porta 4001 não pôde ser aberta: address already in use"},
		},
		{
			nome:        "wsl sem espelho: bind falhou tem precedência sobre o aviso de NAT",
			portas:      []DevicePort{{DeviceID: 11, Port: 4001, Started: true, BindError: "permission denied"}},
			env:         Environment{Kind: KindWSL, WSLMirrored: false},
			queroStatus: []Status{StatusUnreachable},
			queroReason: []string{"a porta 4001 não pôde ser aberta: permission denied"},
		},
		{
			nome:        "docker: emulador parado com porta publicada continua ok",
			portas:      []DevicePort{{DeviceID: 12, Port: 4001, Started: false}},
			env:         dockerPublicado,
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
