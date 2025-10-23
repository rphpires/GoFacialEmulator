# Início Rápido com Docker

Guia simplificado para executar o GoFacialEmulator usando Docker.

## Pré-requisitos

```bash
docker --version  # Versão 20.10+
docker-compose --version  # Versão 2.0+
```

## Iniciar em 3 passos

### 1. Clone o repositório (se ainda não o fez)

```bash
git clone <repository-url>
cd GoFacialEmulator
```

### 2. Inicie os containers

```bash
docker-compose up -d
```

### 3. Acesse a aplicação

Abra o navegador em: http://localhost:8080

## Verificar Status

```bash
# Ver status dos containers
docker-compose ps

# Ver logs em tempo real
docker-compose logs -f app

# Ver logs do banco de dados
docker-compose logs -f postgres
```

## Parar a Aplicação

```bash
docker-compose stop
```

## Reiniciar a Aplicação

```bash
docker-compose restart
```

## Remover Tudo

```bash
# Parar e remover containers (mantém dados)
docker-compose down

# Remover containers E dados do banco
docker-compose down -v
```

## Atualizar após Mudanças no Código

```bash
docker-compose up -d --build
```

## Estrutura Criada

Após iniciar, você terá:

- **facial-emulator-app**: Container da aplicação Go
- **facial-emulator-db**: Container PostgreSQL
- **gofacialemulator_postgres_data**: Volume com dados do banco
- **gofacialemulator_facial-network**: Rede interna Docker

## Portas Utilizadas

- **8080**: Interface web e API REST
- **5432**: PostgreSQL (opcional, apenas para acesso externo)

## Dados Persistentes

Os seguintes diretórios são criados/mapeados:

- `./logs`: Logs da aplicação
- `./traces`: Arquivos de trace

## Solução de Problemas

### Container não inicia

```bash
docker-compose logs app
```

### Resetar banco de dados

```bash
docker-compose down -v
docker-compose up -d
```

### Verificar conectividade do banco

```bash
docker exec facial-emulator-db pg_isready -U emulator
```

### Acessar banco de dados diretamente

```bash
docker exec -it facial-emulator-db psql -U emulator -d service_db
```

## Próximos Passos

1. Acesse http://localhost:8080
2. Configure a conexão WXS nas configurações
3. Clique em "Refresh DB" para carregar dispositivos
4. Inicie os emuladores

Para mais detalhes, consulte [DOCKER.md](DOCKER.md)
