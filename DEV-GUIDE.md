# Guia de Desenvolvimento

Este guia mostra como rodar a aplicação localmente para desenvolvimento rápido.

## 🚀 Desenvolvimento Local (RECOMENDADO)

### Windows

```bash
# 1. Iniciar apenas o banco de dados
docker-compose -f docker-compose.db-only.yml up -d

# 2. Rodar a aplicação localmente
run-local.bat

# OU manualmente:
go run cmd/emulator-service/main.go -config=configs/config.local.yaml
```

### Linux/Mac

```bash
# 1. Dar permissão de execução
chmod +x run-local.sh

# 2. Executar
./run-local.sh

# OU manualmente:
docker-compose -f docker-compose.db-only.yml up -d
go run cmd/emulator-service/main.go -config=configs/config.local.yaml
```

### Vantagens do Desenvolvimento Local

✅ **Recompilação instantânea** (~2 segundos vs ~2 minutos de rebuild Docker)
✅ **Debugging direto** com breakpoints
✅ **Hot reload** natural do Go
✅ **Logs imediatos** no terminal
✅ **Sem cache de Docker** atrapalhando

## 🐳 Rodar Tudo no Docker (Produção)

```bash
# Iniciar tudo (app + banco)
docker-compose up -d

# Rebuild apenas do app
docker-compose up -d --build app

# Rebuild completo
docker-compose down
docker-compose up -d --build
```

## 🔄 Hot Reload com Air (Desenvolvimento no Docker)

```bash
# Usa Air para hot reload automático dentro do container
docker-compose -f docker-compose.dev.yml up
```

## 🛠️ Comandos Úteis

```bash
# Ver logs do banco
docker-compose -f docker-compose.db-only.yml logs -f postgres

# Parar banco
docker-compose -f docker-compose.db-only.yml down

# Parar banco E apagar dados
docker-compose -f docker-compose.db-only.yml down -v

# Build local para teste
go build -o facial_emulator.exe cmd/emulator-service/main.go

# Rodar com config customizada
go run cmd/emulator-service/main.go -config=configs/config.custom.yaml
```

## 📝 Arquivos de Configuração

- `configs/config.yaml` - Configuração para Docker (usa `host: postgres`)
- `configs/config.local.yaml` - Configuração para desenvolvimento local (usa `host: localhost`)

## 🔍 Troubleshooting

**Erro: "connection refused" ao conectar no banco**
```bash
# Verificar se o banco está rodando
docker ps | grep facial-emulator-db

# Se não estiver, iniciar
docker-compose -f docker-compose.db-only.yml up -d
```

**Porta 5432 já em uso**
```bash
# Parar PostgreSQL local se estiver rodando
# Windows: services.msc -> PostgreSQL
# Linux: sudo systemctl stop postgresql
```

**Erro de migração do banco**
```bash
# Recriar banco do zero
docker-compose -f docker-compose.db-only.yml down -v
docker-compose -f docker-compose.db-only.yml up -d
```
