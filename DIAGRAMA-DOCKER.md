# Diagrama Visual - Docker GoFacialEmulator

Documentação visual para entender a arquitetura Docker do projeto.

---

## Arquitetura do Sistema

```
┌─────────────────────────────────────────────────────────────────┐
│                         SEU COMPUTADOR                          │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐ │
│  │                    DOCKER ENGINE                          │ │
│  │                                                           │ │
│  │  ┌─────────────────────────────────────────────────────┐ │ │
│  │  │         REDE: facial-network                        │ │ │
│  │  │                                                     │ │ │
│  │  │  ┌──────────────────────┐  ┌───────────────────┐  │ │ │
│  │  │  │  Container: APP      │  │ Container: DB     │  │ │ │
│  │  │  │                      │  │                   │  │ │ │
│  │  │  │  GoFacialEmulator    │──│  PostgreSQL 15    │  │ │ │
│  │  │  │  (Alpine Linux)      │  │  (Alpine Linux)   │  │ │ │
│  │  │  │                      │  │                   │  │ │ │
│  │  │  │  Porta: 8080         │  │  Porta: 5432      │  │ │ │
│  │  │  └──────────────────────┘  └───────────────────┘  │ │ │
│  │  │           │                          │             │ │ │
│  │  └───────────┼──────────────────────────┼─────────────┘ │ │
│  │              │                          │                 │ │
│  └──────────────┼──────────────────────────┼─────────────────┘ │
│                 │                          │                   │
│                 ▼                          ▼                   │
│         ┌──────────────┐          ┌──────────────┐            │
│         │  ./logs/     │          │ Volume:      │            │
│         │  ./traces/   │          │ postgres_data│            │
│         └──────────────┘          └──────────────┘            │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
                         │
                         │ Porta 8080
                         ▼
                  ┌──────────────┐
                  │  NAVEGADOR   │
                  │ localhost:   │
                  │    8080      │
                  └──────────────┘
```

---

## Fluxo de Instalação

```
    INÍCIO
      │
      ▼
┌──────────────┐
│ Instalar     │
│ Docker       │
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ Obter        │
│ Arquivos do  │
│ Projeto      │
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ Executar:    │
│ docker       │
│ compose      │
│ up -d        │
└──────┬───────┘
       │
       ├─────────────────────────┐
       │                         │
       ▼                         ▼
┌──────────────┐        ┌──────────────┐
│ Baixar       │        │ Baixar       │
│ Imagem       │        │ Imagem       │
│ PostgreSQL   │        │ Go/Alpine    │
└──────┬───────┘        └──────┬───────┘
       │                       │
       │                       ▼
       │              ┌──────────────┐
       │              │ Compilar     │
       │              │ Aplicação Go │
       │              └──────┬───────┘
       │                     │
       ├─────────────────────┘
       │
       ▼
┌──────────────┐
│ Criar        │
│ Containers   │
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ Iniciar      │
│ PostgreSQL   │
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ Aguardar     │
│ DB Healthy   │
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ Iniciar      │
│ Aplicação    │
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ Validar DB   │
│ Criar        │
│ Tabelas      │
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ Servidor HTTP│
│ Pronto!      │
│ Porta 8080   │
└──────┬───────┘
       │
       ▼
    PRONTO
 Acesse no
 Navegador
```

---

## O Que Acontece ao Executar "docker compose up -d"

```
PASSO 1: Leitura
┌─────────────────────────────────┐
│ Docker lê docker-compose.yml    │
│ Entende que precisa de:         │
│ - 1 banco PostgreSQL             │
│ - 1 aplicação Go                 │
│ - 1 rede para comunicação        │
│ - 1 volume para dados            │
└─────────────────────────────────┘
            ↓
PASSO 2: Download
┌─────────────────────────────────┐
│ Se for primeira vez:             │
│ - Baixa postgres:15-alpine       │
│ - Baixa golang:1.21-alpine       │
│ - Baixa alpine:latest            │
│                                  │
│ Primeira vez: 5-10 min           │
│ Próximas vezes: 0 min            │
└─────────────────────────────────┘
            ↓
PASSO 3: Build
┌─────────────────────────────────┐
│ Compila a aplicação Go:          │
│ 1. Copia arquivos                │
│ 2. Baixa dependências            │
│ 3. Compila o código              │
│                                  │
│ Tempo: ~1-2 min                  │
└─────────────────────────────────┘
            ↓
PASSO 4: Criação
┌─────────────────────────────────┐
│ Cria estrutura:                  │
│ - Rede: facial-network           │
│ - Volume: postgres_data          │
│ - Container: postgres            │
│ - Container: app                 │
└─────────────────────────────────┘
            ↓
PASSO 5: Inicialização
┌─────────────────────────────────┐
│ 1. Inicia PostgreSQL             │
│ 2. Executa docker-init.sql       │
│ 3. Aguarda DB ficar "healthy"    │
│ 4. Inicia aplicação Go           │
│ 5. Aplicação valida/cria tabelas │
│ 6. Inicia servidor HTTP          │
└─────────────────────────────────┘
            ↓
         PRONTO!
```

---

## Ciclo de Vida dos Containers

```
┌─────────────────────────────────────────────────────────────┐
│                    ESTADO DOS CONTAINERS                    │
└─────────────────────────────────────────────────────────────┘

                    [NÃO EXISTE]
                         │
                         │ docker compose up -d
                         ▼
                    [CREATING]
                         │
                         ▼
                     [RUNNING] ◄─────┐
                         │           │
        ┌────────────────┼───────────┤
        │                │           │
        │ docker         │ docker    │ docker
        │ compose        │ compose   │ compose
        │ stop           │ restart   │ start
        │                │           │
        ▼                ▼           │
    [STOPPED]        [RESTARTING]────┘
        │
        │ docker compose down
        ▼
    [REMOVED]
```

---

## Estrutura de Arquivos

```
GoFacialEmulator/
│
├── 📁 Arquivos Docker (PRINCIPAIS)
│   ├── 📄 docker-compose.yml         ← Orquestração (quais serviços criar)
│   ├── 📄 Dockerfile                 ← Como construir a aplicação
│   ├── 📄 .dockerignore              ← O que não copiar no build
│   └── 📄 docker-init.sql            ← Script de inicialização do DB
│
├── 📁 Documentação Docker
│   ├── 📘 GUIA-INSTALACAO-DOCKER.md  ← Guia completo passo-a-passo
│   ├── 📘 CHECKLIST-INSTALACAO.md    ← Checklist rápido
│   ├── 📘 QUICKSTART-DOCKER.md       ← Início super rápido
│   ├── 📘 DOCKER.md                  ← Documentação técnica detalhada
│   └── 📘 DIAGRAMA-DOCKER.md         ← Este arquivo (visual)
│
├── 📁 Código Fonte
│   ├── cmd/                          ← Código principal
│   ├── internal/                     ← Código interno
│   ├── configs/                      ← Configurações
│   └── web/                          ← Interface web
│
└── 📁 Dados Gerados (criados automaticamente)
    ├── logs/                         ← Logs da aplicação
    └── traces/                       ← Arquivos de trace
```

---

## Comunicação Entre Containers

```
┌─────────────────────────────────────────────────────────────┐
│                    COMO SE COMUNICAM                        │
└─────────────────────────────────────────────────────────────┘

Container APP (facial-emulator-app)
    │
    │ 1. Aplicação precisa do banco de dados
    │
    ▼
┌──────────────────────────────────────┐
│ Usa hostname: "postgres"             │ ← Nome do serviço no
│ Porta: 5432                          │   docker-compose.yml
└──────────────────────────────────────┘
    │
    │ 2. DNS interno do Docker resolve
    │    "postgres" para IP do container
    │
    ▼
Container DB (facial-emulator-db)
    │
    │ 3. PostgreSQL responde na porta 5432
    │
    ▼
Dados trafegam pela rede "facial-network"


VOCÊ (Navegador)
    │
    │ Acessa: localhost:8080
    │
    ▼
Docker mapeia: porta 8080 do HOST → porta 8080 do CONTAINER
    │
    ▼
Container APP responde
```

---

## Persistência de Dados

```
┌─────────────────────────────────────────────────────────────┐
│                 ONDE OS DADOS FICAM                         │
└─────────────────────────────────────────────────────────────┘

╔═══════════════════════════════════════════════════════════╗
║  DADOS PERSISTENTES (sobrevivem a "docker compose down")  ║
╚═══════════════════════════════════════════════════════════╝

1. Volume: postgres_data
   ├─ Tipo: Volume Docker
   ├─ Conteúdo: Todos os dados do PostgreSQL
   ├─ Local no host: Gerenciado pelo Docker
   └─ Removido apenas com: docker compose down -v

2. Bind Mount: ./logs
   ├─ Tipo: Pasta mapeada
   ├─ Conteúdo: Logs da aplicação
   └─ Local no host: Pasta do projeto

3. Bind Mount: ./traces
   ├─ Tipo: Pasta mapeada
   ├─ Conteúdo: Arquivos de trace
   └─ Local no host: Pasta do projeto

╔═══════════════════════════════════════════════════════════╗
║  DADOS NÃO PERSISTENTES (perdidos no "docker compose down")║
╚═══════════════════════════════════════════════════════════╝

1. Memória dos containers
   └─ Variáveis, cache, processos em execução

2. Sistemas de arquivos dos containers
   └─ Tudo que não está em volume ou bind mount
```

---

## Portas e Acesso

```
┌─────────────────────────────────────────────────────────────┐
│                    MAPA DE PORTAS                           │
└─────────────────────────────────────────────────────────────┘

SEU COMPUTADOR              CONTAINER
┌──────────────┐            ┌──────────────┐
│ localhost:   │            │ Container:   │
│ 8080         │────────────│ 8080         │ GoFacialEmulator
└──────────────┘            └──────────────┘
                                   │
                                   │ Comunicação interna
                                   │ (não exposta)
                                   ▼
┌──────────────┐            ┌──────────────┐
│ localhost:   │            │ Container:   │
│ 5432         │────────────│ 5432         │ PostgreSQL
└──────────────┘            └──────────────┘
 (opcional)                  (hostname: postgres)


URLs de Acesso:
├─ http://localhost:8080              → Dashboard principal
├─ http://localhost:8080/comparison   → Página de comparação
├─ http://localhost:8080/settings     → Configurações
├─ http://localhost:8080/health       → Health check
└─ http://localhost:8080/api/*        → API REST

Conexão Banco (se porta 5432 exposta):
└─ postgresql://emulator:emulator123@localhost:5432/service_db
```

---

## Comandos e Seus Efeitos Visuais

```
┌─────────────────────────────────────────────────────────────┐
│             docker compose up -d                            │
└─────────────────────────────────────────────────────────────┘

ANTES:                          DEPOIS:
[ ] Container APP               [✓] Container APP (Running)
[ ] Container DB                [✓] Container DB (Running)
[ ] Rede facial-network         [✓] Rede facial-network
[ ] Volume postgres_data        [✓] Volume postgres_data


┌─────────────────────────────────────────────────────────────┐
│             docker compose stop                             │
└─────────────────────────────────────────────────────────────┘

ANTES:                          DEPOIS:
[✓] Container APP (Running)     [✓] Container APP (Stopped)
[✓] Container DB (Running)      [✓] Container DB (Stopped)
[✓] Rede facial-network         [✓] Rede facial-network
[✓] Volume postgres_data        [✓] Volume postgres_data
                                    (DADOS MANTIDOS)


┌─────────────────────────────────────────────────────────────┐
│             docker compose down                             │
└─────────────────────────────────────────────────────────────┘

ANTES:                          DEPOIS:
[✓] Container APP               [ ] Container APP (Removed)
[✓] Container DB                [ ] Container DB (Removed)
[✓] Rede facial-network         [ ] Rede facial-network
[✓] Volume postgres_data        [✓] Volume postgres_data
                                    (DADOS MANTIDOS)


┌─────────────────────────────────────────────────────────────┐
│             docker compose down -v                          │
└─────────────────────────────────────────────────────────────┘

ANTES:                          DEPOIS:
[✓] Container APP               [ ] Container APP (Removed)
[✓] Container DB                [ ] Container DB (Removed)
[✓] Rede facial-network         [ ] Rede facial-network
[✓] Volume postgres_data        [ ] Volume postgres_data
                                    (DADOS PERDIDOS!)
```

---

## Fluxo de Troubleshooting

```
                    PROBLEMA?
                        │
                        ▼
            ┌───────────────────────┐
            │ docker compose ps     │
            └───────────┬───────────┘
                        │
        ┌───────────────┴───────────────┐
        │                               │
        ▼                               ▼
   Status: Up?                     Status: Exit?
        │                               │
       SIM                             NÃO
        │                               │
        ▼                               ▼
┌──────────────┐              ┌──────────────┐
│ Ver logs:    │              │ Ver logs:    │
│ docker       │              │ docker       │
│ compose logs │              │ compose logs │
│ app          │              │ app --tail   │
│              │              │ =100         │
└──────┬───────┘              └──────┬───────┘
       │                             │
       ▼                             ▼
   Funciona?                   Erro visível?
       │                             │
      SIM                           SIM
       │                             │
       ▼                             ▼
┌──────────────┐              ┌──────────────┐
│ Problema     │              │ Resolver     │
│ resolvido!   │              │ erro         │
└──────────────┘              │ específico   │
                              └──────┬───────┘
                                     │
                                     ▼
                              ┌──────────────┐
                              │ Recriar:     │
                              │ docker       │
                              │ compose down │
                              │ docker       │
                              │ compose up -d│
                              └──────────────┘
```

---

## Resumo Visual de Comandos

```
╔════════════════════════════════════════════════════════════╗
║              COMANDOS MAIS USADOS                          ║
╚════════════════════════════════════════════════════════════╝

┌────────────────────────────────────────────────────────────┐
│ docker compose up -d                                       │
│ ↳ Cria e inicia tudo                                       │
│ ↳ Modo detached (roda em background)                       │
└────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────┐
│ docker compose ps                                          │
│ ↳ Mostra status dos containers                             │
│ ↳ Use sempre para verificar                                │
└────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────┐
│ docker compose logs app --tail=50                          │
│ ↳ Mostra últimas 50 linhas de log                          │
│ ↳ Use para diagnosticar problemas                          │
└────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────┐
│ docker compose logs -f app                                 │
│ ↳ Acompanha logs em tempo real                             │
│ ↳ Pressione Ctrl+C para sair                               │
└────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────┐
│ docker compose restart                                     │
│ ↳ Reinicia os containers                                   │
│ ↳ Mantém configurações                                     │
└────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────┐
│ docker compose down                                        │
│ ↳ Para e remove containers                                 │
│ ↳ MANTÉM dados do banco                                    │
└────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────┐
│ docker compose down -v                                     │
│ ↳ Para, remove containers E volumes                        │
│ ↳ APAGA dados do banco (cuidado!)                          │
└────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────┐
│ docker compose up -d --build                               │
│ ↳ Reconstrói e reinicia                                    │
│ ↳ Use após mudanças no código                              │
└────────────────────────────────────────────────────────────┘
```

---

## Quando Usar Cada Comando

```
SITUAÇÃO                           COMANDO
─────────────────────────────────────────────────────────
Primeira vez                   →   docker compose up -d
Verificar se está rodando      →   docker compose ps
Ver o que está acontecendo     →   docker compose logs -f app
Algo não funciona              →   docker compose logs app --tail=100
Parar temporariamente          →   docker compose stop
Continuar após parar           →   docker compose start
Reiniciar a aplicação          →   docker compose restart
Mudou o código                 →   docker compose up -d --build
Acabou, pode desligar          →   docker compose down
Recomeçar do zero              →   docker compose down -v
                                   docker compose up -d
```

---

## Conclusão

Este diagrama ajuda a visualizar:

✅ Como os containers se comunicam
✅ Onde os dados ficam armazenados
✅ O que cada comando faz
✅ Como resolver problemas

Para instruções detalhadas, consulte:
- **GUIA-INSTALACAO-DOCKER.md** - Guia passo-a-passo completo
- **CHECKLIST-INSTALACAO.md** - Checklist rápido
- **DOCKER.md** - Documentação técnica avançada
