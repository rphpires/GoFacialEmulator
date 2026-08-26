# Docker - GoFacialEmulator

Guia para executar o GoFacialEmulator usando Docker com PostgreSQL integrado.

## Pré-requisitos

- Docker Engine 20.10 ou superior
- Docker Compose 2.0 ou superior

## Estrutura

O projeto inclui:

- **Dockerfile**: Build multi-stage da aplicação Go
- **docker-compose.yml**: Orquestração de serviços (aplicação + PostgreSQL)
- **docker-init.sql**: Script de inicialização do banco de dados
- **.dockerignore**: Otimização do build

## Iniciar o Sistema

### Primeira execução

```bash
docker-compose up -d
```

Este comando irá:
1. Baixar a imagem do PostgreSQL 15
2. Compilar a aplicação Go
3. Criar os bancos de dados (service_db e emulator_db)
4. Iniciar os serviços

### Acompanhar os logs

```bash
# Logs de todos os serviços
docker-compose logs -f

# Apenas logs da aplicação
docker-compose logs -f app

# Apenas logs do banco de dados
docker-compose logs -f postgres
```

## Acessar a Aplicação

- **Interface Web**: http://localhost:7070
- **API REST**: http://localhost:7070/api
- **Página de Comparação**: http://localhost:7070/comparison

## Gerenciar o Sistema

### Parar os serviços

```bash
docker-compose stop
```

### Reiniciar os serviços

```bash
docker-compose restart
```

### Parar e remover containers

```bash
docker-compose down
```

### Remover tudo incluindo volumes (dados do banco)

```bash
docker-compose down -v
```

## Atualizar a Aplicação

Após fazer alterações no código:

```bash
docker-compose up -d --build
```

## Configurações

### Variáveis de Ambiente

As configurações podem ser alteradas no [docker-compose.yml](docker-compose.yml) na seção `environment` do serviço `app`:

```yaml
environment:
  # Servidor HTTP
  SERVER_HOST: "0.0.0.0"
  SERVER_PORT: "7070"

  # PostgreSQL
  PG_HOST: postgres
  PG_PORT: "5432"
  PG_USER: emulator
  PG_PASSWORD: emulator123
  PG_DATABASE: service_db
```

### Portas Expostas

- **7070**: Interface web e API
- **4000-4999**: Range de portas para emuladores de dispositivos (1000 emuladores possíveis)
- **5432**: PostgreSQL (opcional, para acesso externo)

## Volumes Persistentes

### Volume do PostgreSQL

Os dados do banco são armazenados em um volume Docker nomeado `postgres_data`:

```bash
# Ver detalhes do volume
docker volume inspect gofacialemulator_postgres_data

# Backup do volume
docker run --rm -v gofacialemulator_postgres_data:/data -v $(pwd):/backup alpine tar czf /backup/postgres-backup.tar.gz -C /data .

# Restaurar backup
docker run --rm -v gofacialemulator_postgres_data:/data -v $(pwd):/backup alpine tar xzf /backup/postgres-backup.tar.gz -C /data
```

### Volumes da Aplicação

Logs e traces são mapeados para o host:

- `./logs`: Logs da aplicação
- `./traces`: Arquivos de trace

## Conexão Direta ao Banco de Dados

### Dentro do container

```bash
docker exec -it facial-emulator-db psql -U emulator -d service_db
```

### Do host (se porta 5432 estiver exposta)

```bash
psql -h localhost -U emulator -d service_db
```

Senha padrão: `emulator123`

## Troubleshooting

### Verificar status dos containers

```bash
docker-compose ps
```

### Verificar logs de erro

```bash
docker-compose logs --tail=100 app
```

### Recriar containers do zero

```bash
docker-compose down -v
docker-compose up -d --build
```

### Verificar conectividade do banco

```bash
docker exec facial-emulator-db pg_isready -U emulator
```

### Limpar recursos Docker não utilizados

```bash
docker system prune -a
```

## Estrutura de Rede

Os serviços comunicam através de uma rede bridge chamada `facial-network`:

- **postgres**: Hostname do banco de dados
- **app**: Hostname da aplicação

## Segurança

### Alterar senhas padrão

Edite o [docker-compose.yml](docker-compose.yml):

```yaml
services:
  postgres:
    environment:
      POSTGRES_PASSWORD: sua_senha_forte

  app:
    environment:
      PG_PASSWORD: sua_senha_forte
```

### Remover acesso externo ao PostgreSQL

Comente a seção `ports` do serviço postgres no [docker-compose.yml](docker-compose.yml):

```yaml
services:
  postgres:
    # ports:
    #   - "5432:5432"
```

## Deploy em Produção

Para ambiente de produção, considere:

1. Usar secrets do Docker Swarm ou variáveis de ambiente do orchestrator
2. Configurar health checks adequados
3. Configurar limites de recursos (CPU/memória)
4. Usar volumes externos para backup
5. Configurar SSL/TLS para conexões
6. Implementar monitoramento e alertas

Exemplo com resource limits:

```yaml
services:
  app:
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
        reservations:
          cpus: '1'
          memory: 1G
```

## Comandos Úteis

```bash
# Ver uso de recursos
docker stats

# Inspecionar container da aplicação
docker inspect facial-emulator-app

# Executar shell no container
docker exec -it facial-emulator-app sh

# Ver versão do PostgreSQL
docker exec facial-emulator-db psql -U emulator -c "SELECT version();"

# Listar bancos de dados
docker exec facial-emulator-db psql -U emulator -c "\l"
```
