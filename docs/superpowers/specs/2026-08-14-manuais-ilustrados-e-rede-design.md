# Manuais ilustrados e alcançabilidade de rede — Design

**Data:** 2026-08-14
**Substitui:** Task 8 de `docs/superpowers/plans/2026-08-13-instalacao-simplificada.md`
**Depende de:** Tasks 1 a 5 do mesmo plano (já commitadas)

## 1. Problema

O plano de instalação simplificada entregou três pacotes auto-contidos, mas parou antes da documentação. Hoje o cliente recebe um ZIP com um `LEIA-ME.txt` de vinte linhas que termina apontando para um `MANUAL.md` que nunca foi escrito. Os itens 4 e 5 do `new_tasks.md` — manual detalhado com caminhos de instalação, procedimento de teste e localização dos logs — continuam abertos.

O usuário-alvo é técnico de campo, não desenvolvedor. Texto corrido não basta: as etapas de instalar o Docker Desktop e habilitar o WSL2 são feitas em interfaces gráficas de terceiros, onde "clique no botão certo" só se ensina mostrando.

Ao levantar o estado atual, apareceu um segundo problema, mais grave que o primeiro:

**A aplicação obedece o `BaseCommPort` de cada controlador do W-Access, mas nenhum dos três pacotes garante que essa porta seja alcançável.** No ambiente de referência há 301 controladores marcados `emulator%` ocupando as portas 4001-4301, enquanto o pacote Docker publica apenas 4000-4099. Os 201 emuladores excedentes sobem, escutam dentro do container e são invisíveis do lado de fora. A interface do emulador mostra todos verdes; o W-Access mostra todos offline; nenhum log registra o problema.

Documentar isso não basta — o manual passaria a ensinar a contornar um defeito. As duas frentes andam juntas nesta spec.

## 2. Escopo

**Dentro:**

- Commit do pacote Linux (Task 6, pronto e *staged*)
- Detecção de alcançabilidade e aviso na interface
- Revisão do padrão de portas do pacote Docker
- Firewall, `ulimit` e detecção de WSL no `instalar.sh`
- Toggle online/standalone na interface (Task 7 do plano anterior, absorvida aqui)
- Três manuais em PDF, um por pacote, com imagens
- Pipeline de geração: montagem, captura de telas, PDF
- Integração no `build-pacotes.bat`

**Fora:**

- Compatibilidade com o Gerenciador GO (item 3 do `new_tasks.md`) — mantida fora conforme decisão registrada em `2026-08-13-instalacao-simplificada-design.md`, linha 63
- Rotação da senha do W_Access exposta no histórico do git — ação de infraestrutura, não de código
- Tradução dos manuais para outro idioma

## 3. Decisões

| Decisão | Escolha | Razão |
|---|---|---|
| Formato de entrega | PDF dentro de cada ZIP | Técnico de campo trabalha offline e imprime |
| Fatiamento | Um PDF por pacote, capítulos comuns em fonte única | Quem pegou o pacote Windows nunca lê sobre Docker; a trilha errada é onde o leigo erra |
| Telas não capturáveis | Placeholder no PDF + checklist gerado | Manual já é utilizável hoje e a dívida fica visível |
| Capítulo de testes | Roteiro de validação guiado, com resultado esperado por etapa | É o que responde "funcionou?" no campo |
| Toolchain do PDF | Markdown → HTML → Playwright `page.pdf()` | Chrome já instalado; mesmo motor dos screenshots; uma dependência |
| Ordem de trabalho | Rede e UI antes dos manuais | Cada screenshot refeito é trabalho perdido |

## 4. Ordem de execução

| Fase | Conteúdo | Pré-requisito da seguinte porque |
|---|---|---|
| A | Commit da Task 6 | Trabalho pronto e parado; zero risco |
| B | Alcançabilidade de rede | Define o comportamento que o manual descreve |
| C | Toggle online/standalone | Última mudança na tela que vira screenshot |
| D | Manuais | Só depois que a interface parar de mudar |

---

## 5. Fase A — pacote Linux

`packaging/linux/` está completo e *staged*, com o alvo `build_linux` já presente em `packaging/build-pacotes.bat:115`. Falta o commit.

Nenhuma mudança de código. A fase existe para não deixar trabalho concluído fora do histórico enquanto as fases seguintes mexem nos mesmos arquivos.

---

## 6. Fase B — alcançabilidade de rede

### 6.1 Princípio

A aplicação obedece o `BaseCommPort` do W-Access e nunca o contradiz. Ela não escolhe porta, não realoca, não recusa dispositivo. O que ela passa a fazer é **avisar quando o ambiente não vai conseguir entregar o que o banco pediu**.

Em `internal/emulator/hikvision/emulator.go:110` o bind é `0.0.0.0:<BaseCommPort>`. Esse comportamento não muda.

### 6.2 Detecção de ambiente

Novo pacote `internal/reachability`.

```go
type Kind int // KindLinux, KindWSL, KindDocker, KindWindows

type Environment struct {
    Kind            Kind
    PublishedRanges []PortRange // preenchido só em Docker
    RangesKnown     bool        // false = não foi possível verificar
    WSLMirrored     bool
    MaxOpenFiles    uint64      // RLIMIT_NOFILE; 0 em Windows
}

func Detect() Environment
```

Regras de detecção:

- **Docker** — existência de `/.dockerenv`
- **WSL** — `/proc/version` contém `microsoft` ou `wsl`, sem diferenciar maiúsculas
- **Windows** — `runtime.GOOS == "windows"`
- **Linux** — o restante

De dentro de um container não há como enxergar o mapeamento de portas do host. O compose do pacote passa a exportar a variável de ambiente `PUBLISHED_PORT_RANGE` com o mesmo valor que publica:

```yaml
environment:
  PUBLISHED_PORT_RANGE: "4000-4499"
ports:
  - "4000-4499:4000-4499"
```

Quando a variável estiver ausente ou malformada, `RangesKnown` é `false` e a interface diz que não foi possível verificar — nunca inventa um aviso a partir de suposição. O plano de implementação inclui um teste que compara a variável com a lista de `ports` do compose versionado, para que as duas não divirjam em silêncio.

**Detecção de modo espelhado do WSL2:** o candidato é a presença de uma interface de rede chamada `loopback0`, que o modo espelhado cria. Isso precisa ser **verificado empiricamente** antes de virar código — o plano de implementação traz um passo específico para confirmar numa WSL com e sem `networkingMode=mirrored`. Se não confirmar, o recuo é tratar `WSLMirrored` como desconhecido e exibir o aviso de WSL sempre, redigido de forma condicional.

### 6.3 Relatório de alcançabilidade

```go
type Status int // StatusOK, StatusUnreachable, StatusUnknown, StatusNotStarted

type DeviceReachability struct {
    DeviceID int
    Port     int
    Status   Status
    Reason   string // vazio quando StatusOK
}

type Report struct {
    Environment  Environment
    Devices      []DeviceReachability
    Unreachable  int
    Unknown      int
    NotStarted   int
}

func Analyze(ports []DevicePort, env Environment) Report
```

`Analyze` é função pura sobre os dois argumentos — sem I/O, sem relógio, sem banco. Toda a matriz de casos vira teste de tabela.

A evidência é lida na ordem em que é confiável. **Um bind que falhou vale em qualquer ambiente** e tem precedência sobre a regra de ambiente: sem isso, um dispositivo cujo bind falhou dentro do container, mas cuja porta cai na faixa publicada, seria reportado como alcançável — exatamente a falha que esta funcionalidade existe para pegar. Sem erro de bind, vale a regra do ambiente.

Regras por ambiente:

- **Docker com `RangesKnown`** — porta fora de todas as faixas publicadas é `StatusUnreachable`
- **Docker sem `RangesKnown`** — tudo `StatusUnknown`
- **Docker com rede de host** — tratado como nativo
- **Linux e Windows nativos** — o bind é direto; o teste real é o bind ter funcionado. Falha de bind vira `StatusUnreachable` com o erro do sistema operacional em `Reason`. Hoje esse erro morre no log
- **WSL não espelhado** — todos os dispositivos ficam `StatusUnreachable` com motivo próprio: alcançáveis apenas a partir desta máquina
- **WSL espelhado** — mesma regra do Linux nativo

Em Docker e WSL o veredito de ambiente **não depende de o emulador ter sido iniciado**: avisar que a porta não está publicada antes de o usuário iniciar é o momento mais útil do aviso. Em ambiente nativo a única evidência possível é a tentativa de bind, então um emulador não iniciado é `StatusNotStarted`.

`StatusNotStarted` existe separado de `StatusUnknown` por uma razão de produto: um dispositivo apenas parado é o estado normal de uma instalação recém-feita, e contá-lo como problema faria o aviso da seção 6.4 disparar com centenas de linhas numa máquina onde nada está errado. Aviso que dispara no estado normal treina o usuário a ignorá-lo. `NotStarted` é contado à parte e não levanta o aviso.

### 6.4 Exposição

- `GET /api/reachability` devolve o `Report` em JSON
- `assets/web/templates/devices.html` ganha um bloco acima da tabela `#device-table`, exibido apenas quando `Unreachable > 0` ou `Unknown > 0`:

```
⚠  201 dispositivos não vão ser alcançados pelo Site Controller.
   As portas 4100-4301 não estão publicadas neste ambiente Docker.
   O que fazer: capítulo 6 do manual.                  [ver a lista]
```

O texto do motivo e a referência ao capítulo variam por ambiente. `[ver a lista]` expande as linhas afetadas dentro da própria página, sem navegação nova.

### 6.5 Padrão de portas do pacote Docker

Duas configurações distintas, porque a resposta certa depende do sistema operacional do host:

- **Docker em Linux** (`dev-linux`, o alvo real de deploy) — `network_mode: host`. O container usa a pilha de rede do host e o problema de publicação deixa de existir: qualquer porta que o W-Access pedir funciona. Passa a ser o padrão do compose Linux. Com `network_mode: host`, `PublishedRanges` é irrelevante e o ambiente é tratado como Linux nativo.
- **Docker Desktop em Windows** — rede de host é experimental e não é distribuível com confiança. Continua publicando por faixa, com o padrão subindo de `4000-4099` para `4000-4499`, cobrindo o ambiente de referência com folga. A linha `4000-4999` permanece comentada logo acima, e o manual ensina a estreitar ou alargar.

O plano inclui **medir** o tempo de `docker compose up` com 500 portas publicadas no Docker Desktop antes de fixar o número. Se o custo for alto, o padrão cai e o manual carrega o aviso correspondente. A medição é um passo do plano, não uma suposição desta spec.

### 6.6 `instalar.sh`

Três acréscimos, todos idempotentes e todos com mensagem em português dizendo o que foi feito:

1. **Firewall** — com `ufw` ativo, liberar a faixa de portas dos emuladores; o mesmo para `firewalld`. Sem firewall ativo, não fazer nada e não avisar.
2. **`ulimit -n`** — comparar o limite atual com o necessário para o número de dispositivos previsto. Abaixo do necessário, escrever `/etc/security/limits.d/gofacialemulator.conf` e avisar que a sessão precisa ser reaberta.
3. **WSL** — detectado o WSL sem modo espelhado, imprimir o bloco exato para colar no `.wslconfig` do Windows e explicar em uma frase que, sem isso, apenas a máquina local alcança os emuladores.

A faixa usada pelos itens 1 e 2 vem de uma pergunta feita ao usuário durante a instalação, com padrão `4000-4499` ao dar Enter.

---

## 7. Fase C — modo online/standalone

Absorve a Task 7 de `2026-08-13-instalacao-simplificada.md` sem alteração de conteúdo: coluna **Modo** na tabela de dispositivos, `GET`/`POST /api/devices/:id/mode`, leitura e escrita de `LocalAuthentication` em `emulator.device_settings`.

Está aqui por uma razão de sequência: é a última mudança visível na tela que será fotografada. O plano de implementação reaproveita os passos já escritos naquele documento.

---

## 8. Fase D — manuais

### 8.1 Estrutura de fontes

```
docs/manual/
  manifesto.yaml            # ordem dos capítulos por alvo
  conteudo/
    comum/                  # entra nos três PDFs
      configurar-wxs.md
      validacao.md
      logs.md
      problemas.md
    docker/
      antes-de-comecar.md
      instalar-docker-desktop.md
      instalar-emulador.md
      portas-e-rede.md
    windows/
      antes-de-comecar.md
      instalar-emulador.md
      portas-e-rede.md
    linux/
      antes-de-comecar.md
      instalar-wsl.md
      instalar-emulador.md
      portas-e-rede.md
  ativos/
    css/manual.css
    img/gerado/             # screenshots automáticos, versionados
    img/manual/             # capturas humanas, versionadas
    img/svg/                # diagramas desenhados
  ferramentas/
    montar.mjs
    capturar.mjs
    pdf.mjs
  CAPTURAS-PENDENTES.md     # gerado; nunca editado à mão
```

Capítulo comum é escrito uma vez e referenciado três vezes; não existe cópia. O nome `portas-e-rede.md` repetido nas três pastas de alvo é deliberado: o conteúdo é genuinamente diferente por pacote, conforme a seção 6.

### 8.2 Toolchain

Um `package.json` novo na raiz de `docs/manual/`, com duas dependências:

- **Playwright**, configurado com `channel: 'chrome'` — usa o Chrome já instalado, sem baixar os browsers próprios. Serve tanto para capturar telas quanto para gerar o PDF via `page.pdf()`, que fala o mesmo CDP do `--print-to-pdf` mas espera o carregamento e permite rodar JS antes de imprimir.
- **paged.js**, JS puro executado dentro da página antes da impressão. Sem ela não há numeração de página, cabeçalho corrido nem sumário com número de página, e manual impresso sem número de página não serve no campo.

Node 24 e npm 11 já estão na máquina de build.

### 8.3 `montar.mjs`

Lê o `manifesto.yaml`, resolve os capítulos do alvo, converte Markdown para HTML e emite um documento único com capa, sumário e CSS de impressão.

Responsabilidades adicionais:

- Numerar capítulos, seções e figuras automaticamente. Nenhum número é escrito à mão no Markdown.
- Resolver referências cruzadas por âncora.
- **Placeholder de imagem:** referência a arquivo inexistente vira caixa tracejada com o texto alternativo da figura, e a linha é registrada em `CAPTURAS-PENDENTES.md`. O build **não** falha — se falhasse, o primeiro PDF nunca seria gerado. A pendência aparece em três lugares: no PDF, no checklist e na saída do build (`[manual] N imagens pendentes`).
- Capítulo declarado no manifesto e ausente do disco **falha** o build. Ausência de capítulo é erro; ausência de figura é dívida conhecida.

### 8.4 `capturar.mjs`

Duas suítes independentes:

- **Emulador** — `http://localhost:7070`. Telas: lista de dispositivos (`#device-table`), filtros (`#filter-form`), configurações (`#wxsSettingsForm`), comparação, métricas, e o aviso de alcançabilidade da seção 6.4.
- **W-Access** — `https://localhost/W-Access` com `ignoreHTTPSErrors`, certificado self-signed. Telas do cadastro de controlador: onde vai a descrição `emulator_NN`, o endereço e o `BaseCommPort`.

Parâmetros fixos: viewport 1440×900, `deviceScaleFactor: 2`. Abaixo disso a figura sai borrada no papel.

**Credenciais fora do repositório.** O login do W-Access vem de `docs/manual/.captura.env`, incluído no `.gitignore`, com `.captura.env.exemplo` versionado ao lado. Sem o arquivo, a suíte do W-Access é pulada com aviso e a do emulador roda normalmente. Três commits recentes removeram credenciais de arquivos versionados; nada aqui as reintroduz.

**Callouts.** Antes do disparo, o script injeta o círculo vermelho numerado ancorado a um seletor e esmaece o restante da tela. **Seletor que não casa lança erro em vez de produzir a figura.** Uma seta apontando para o lugar errado é pior que figura nenhuma, e o erro precisa aparecer no build e não na ligação do cliente.

**Mascaramento.** A tela `/settings` exibe host, banco e usuário do W-Access. É capturada com valores de exemplo preenchidos por script (`servidor-wxs`, `W_Access`, `usuario`) e o campo de senha mascarado pelo Playwright. Nenhuma figura carrega dado da máquina de quem gerou.

**Saída versionada.** Os PNGs vão para o git. Custa poucos megabytes e compra que o `build-pacotes.bat` gere os PDFs em qualquer estação, sem W-Access, sem SQL Server e sem emulador de pé. Um build que exigisse o ambiente completo vivo quebraria na primeira release feita de outra máquina.

### 8.5 Conteúdo dos três PDFs

Capítulos comuns aos três, na mesma ordem final:

| Capítulo | Conteúdo |
|---|---|
| Configurar o W-Access | Cadastro do controlador, descrição `emulator_NN`, endereço, `BaseCommPort`; tela `/settings` do emulador; teste de conexão |
| Roteiro de validação | Passo a passo com resultado esperado por etapa: subir um emulador, vê-lo online no W-Access, simular uma leitura, conferir o evento chegando, conferir a linha no log. Cada etapa que falhar aponta para a seção correspondente de problemas |
| Onde estão os logs | Caminho por pacote, o que cada arquivo contém, como ler o `trace.html` |
| Problemas comuns | Tabela sintoma → causa → o que fazer, alimentada pelos modos de falha conhecidos |

Capítulos por alvo:

- **Docker** — antes de começar; instalar o Docker Desktop (download, wizard, primeira execução); instalar o emulador; portas e rede (publicação por faixa, `network_mode: host` no Linux, como alargar)
- **Windows** — antes de começar; instalar o emulador; portas e rede (firewall do Windows, porta em uso)
- **Linux/WSL** — antes de começar; instalar o WSL2 (`wsl --install`, reinício, distribuição); instalar o emulador; portas e rede (firewall, `ulimit`, modo espelhado do WSL2 com o bloco de `.wslconfig`)

O roteiro de validação é executável de verdade no ambiente de referência — W-Access local com 301 controladores `emulator%` — e cada resultado esperado é capturado, não descrito de memória.

### 8.6 Integração no build

`packaging/build-pacotes.bat` ganha o alvo `manuais`, executável isoladamente:

```
build-pacotes.bat manuais    # só regera os três PDFs
build-pacotes.bat docker     # gera o pacote, com o PDF já existente
build-pacotes.bat todos      # manuais primeiro, depois os três pacotes
```

Cada alvo de pacote copia o PDF correspondente para a raiz do seu ZIP, ao lado do `LEIA-ME.txt`. O alvo `manuais` não recaptura telas: captura é um passo manual e deliberado (`npm run capturar`), porque exige ambiente vivo. O build monta e imprime a partir do que está versionado.

Os três `LEIA-ME.txt` passam a apontar para o PDF ao lado, encerrando a referência quebrada ao `MANUAL.md` inexistente.

### 8.7 Testes

| Alvo | Verificação |
|---|---|
| `reachability.Analyze` | Teste de tabela cobrindo a matriz ambiente × porta, incluindo `RangesKnown: false` |
| `reachability.Detect` | Testes com sistema de arquivos falso para `/.dockerenv` e `/proc/version` |
| Compose × variável | O valor de `PUBLISHED_PORT_RANGE` bate com as faixas em `ports` do compose versionado |
| `GET /api/reachability` | Contrato HTTP: forma do JSON e código de status |
| `montar.mjs` | Manifesto resolve; capítulo ausente falha; figura ausente vira placeholder e entra no checklist |
| Integridade de links | Toda âncora interna resolve; toda figura referenciada existe ou é placeholder registrado |
| PDF | Os três arquivos existem, têm no mínimo 10 páginas e contêm os títulos de capítulo declarados no manifesto |
| `capturar.mjs` | O próprio erro em seletor que não casa é a verificação |

---

## 9. Pendências

1. **Capturas humanas** — cerca de seis a dez telas de aplicativo nativo que nenhum script alcança: wizard do instalador do Docker Desktop, Docker Desktop aberto, diálogo de reinício do WSL. `CAPTURAS-PENDENTES.md` traz a lista numerada com o que abrir, o que destacar, nome de arquivo esperado e resolução. Enquanto não chegarem, o PDF mostra a caixa tracejada e o texto do passo.
2. **Medição do `up` com 500 portas** no Docker Desktop, antes de fixar o padrão da seção 6.5.
3. **Confirmação empírica** do marcador de modo espelhado do WSL2 descrito na seção 6.2.

## 10. Riscos

| Risco | Tratamento |
|---|---|
| Screenshot envelhece quando a interface muda | Captura é script versionado, não trabalho manual: regerar é um comando. Callout com seletor quebrado falha alto |
| `network_mode: host` muda o comportamento do compose Linux | Fica restrito ao compose Linux; o pacote Windows continua publicando por faixa. Coberto pelo roteiro de validação |
| PNGs incham o repositório | Poucos megabytes, contra um build que exigiria ambiente completo vivo. Aceito conscientemente |
| Manual descreve comportamento que muda depois | Origem da ordem A→B→C→D: os manuais são escritos por último, sobre interface estável |
