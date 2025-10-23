# Guia de Instalação - GoFacialEmulator com Docker

Este guia foi criado para pessoas que nunca ou pouco utilizaram Docker. Siga cada passo com atenção.

## O que é Docker?

Docker é uma ferramenta que permite executar aplicações em "containers" (ambientes isolados). Imagine como se fosse uma caixa que contém tudo que a aplicação precisa para funcionar, sem precisar instalar nada no seu computador.

### Vantagens de usar Docker:

- Não precisa instalar Go, PostgreSQL ou outras dependências
- Funciona igual em qualquer computador (Windows, Linux, Mac)
- Fácil de iniciar, parar e remover
- Não "suja" o seu sistema operacional

---

## Passo 1: Instalar o Docker

### Windows

1. Acesse: https://www.docker.com/products/docker-desktop
2. Clique em "Download for Windows"
3. Execute o instalador baixado
4. Siga o assistente de instalação (deixe as opções padrão)
5. Reinicie o computador quando solicitado
6. Após reiniciar, o Docker Desktop deve abrir automaticamente
7. Aceite os termos de uso

**Verificar instalação:**
- Abra o "Prompt de Comando" ou "PowerShell"
- Digite: `docker --version`
- Deve aparecer algo como: `Docker version 24.0.x`

### Linux (Ubuntu/Debian)

Abra o terminal e execute os comandos abaixo, um por vez:

```bash
# Atualizar lista de pacotes
sudo apt update

# Instalar dependências
sudo apt install -y apt-transport-https ca-certificates curl software-properties-common

# Adicionar chave GPG do Docker
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg

# Adicionar repositório do Docker
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

# Atualizar novamente
sudo apt update

# Instalar Docker
sudo apt install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin

# Adicionar seu usuário ao grupo docker (para não precisar usar sudo)
sudo usermod -aG docker $USER

# IMPORTANTE: Fazer logout e login novamente para aplicar as mudanças
```

**Após fazer logout e login, verifique:**
```bash
docker --version
docker compose version
```

### Linux (CentOS/RHEL/Fedora)

```bash
# Instalar Docker
sudo dnf install -y docker docker-compose-plugin

# Iniciar serviço Docker
sudo systemctl start docker
sudo systemctl enable docker

# Adicionar usuário ao grupo docker
sudo usermod -aG docker $USER

# Fazer logout e login novamente
```

### Mac

1. Acesse: https://www.docker.com/products/docker-desktop
2. Clique em "Download for Mac"
3. Escolha a versão correta:
   - **Intel chip**: Download for Mac (Intel)
   - **Apple Silicon (M1/M2)**: Download for Mac (Apple Silicon)
4. Abra o arquivo .dmg baixado
5. Arraste o Docker para a pasta Applications
6. Abra o Docker da pasta Applications
7. Aceite os termos

**Verificar instalação:**
- Abra o "Terminal"
- Digite: `docker --version`

---

## Passo 2: Obter os Arquivos do Projeto

### Opção A: Se você tem o projeto em um arquivo ZIP

1. Extraia o arquivo ZIP para uma pasta de sua escolha
   - Exemplo: `C:\Projetos\GoFacialEmulator` (Windows)
   - Exemplo: `/home/usuario/GoFacialEmulator` (Linux/Mac)

### Opção B: Se você tem acesso ao Git

**Windows:**
```cmd
cd C:\Projetos
git clone https://github.com/rphpires/GoFacialEmulator.git GoFacialEmulator
cd GoFacialEmulator
```

**Linux/Mac:**
```bash
cd ~
git clone https://github.com/rphpires/GoFacialEmulator.git GoFacialEmulator
cd GoFacialEmulator
```

### Verificar se está na pasta certa

Você deve estar na pasta que contém os arquivos:
- `docker-compose.yml`
- `Dockerfile`
- `configs/`
- `cmd/`
- etc.

**Verificar (Windows):**
```cmd
dir
```

**Verificar (Linux/Mac):**
```bash
ls -la
```

Você deve ver o arquivo `docker-compose.yml` listado.

---

## Passo 3: Iniciar a Aplicação

### Windows (Prompt de Comando ou PowerShell)

```cmd
# Navegar até a pasta do projeto (ajuste o caminho se necessário)
cd C:\Projetos\GoFacialEmulator

# Iniciar os containers
docker compose up -d
```

### Linux/Mac (Terminal)

```bash
# Navegar até a pasta do projeto (ajuste o caminho se necessário)
cd ~/GoFacialEmulator

# Iniciar os containers
docker compose up -d
```

### O que vai acontecer:

1. O Docker vai baixar as imagens necessárias (PostgreSQL e Alpine Linux)
   - Primeira vez: pode demorar 5-10 minutos dependendo da internet
   - Próximas vezes: será muito mais rápido

2. Vai compilar a aplicação Go

3. Vai criar dois containers:
   - `facial-emulator-db`: Banco de dados PostgreSQL
   - `facial-emulator-app`: Aplicação GoFacialEmulator

4. Quando terminar, você verá mensagens como:
   ```
   Container facial-emulator-db  Started
   Container facial-emulator-app  Started
   ```

---

## Passo 4: Verificar se Está Funcionando

### Verificar status dos containers

**Comando:**
```bash
docker compose ps
```

**Resultado esperado:**
```
NAME                  STATUS
facial-emulator-app   Up X minutes
facial-emulator-db    Up X minutes (healthy)
```

Se aparecer "Up", significa que está funcionando!

### Verificar logs da aplicação

**Ver últimas 20 linhas:**
```bash
docker compose logs app --tail=20
```

**Acompanhar logs em tempo real (pressione Ctrl+C para sair):**
```bash
docker compose logs -f app
```

**Logs saudáveis devem mostrar:**
```
Starting Facial Emulator Service
Validating database structure...
Starting HTTP server on 0.0.0.0:8080
```

### Testar acesso web

1. Abra seu navegador
2. Acesse: http://localhost:8080
3. Você deve ver a interface do GoFacialEmulator

**Se não funcionar:**
- Aguarde 30 segundos e tente novamente
- Verifique os logs (comando acima)
- Veja a seção "Problemas Comuns" no final deste guia

---

## Passo 5: Usar a Aplicação

### Acessar a Interface Web

- **Dashboard**: http://localhost:8080
- **Comparação de Dados**: http://localhost:8080/comparison
- **Configurações**: http://localhost:8080/settings

### Configurar Conexão WXS (se aplicável)

1. Acesse http://localhost:8080/settings
2. Preencha os dados do banco WXS:
   - Host/IP do servidor WXS
   - Porta (geralmente 1433)
   - Nome do banco
   - Usuário e senha
3. Clique em "Testar Conexão"
4. Se funcionar, clique em "Salvar"

### Carregar Dispositivos

1. No dashboard principal (http://localhost:8080)
2. Clique no botão "Refresh DB"
3. Aguarde o carregamento
4. Os dispositivos aparecerão na lista

### Iniciar Emuladores

1. Na lista de dispositivos
2. Clique em "Start All" para iniciar todos
3. Ou clique em "Start" em um dispositivo específico

---

## Comandos Úteis do Dia a Dia

### Ver status
```bash
docker compose ps
```

### Ver logs
```bash
# Últimas 50 linhas
docker compose logs app --tail=50

# Acompanhar em tempo real
docker compose logs -f app

# Ver logs do banco de dados
docker compose logs postgres
```

### Parar a aplicação
```bash
docker compose stop
```
*Os dados são mantidos. Você pode iniciar novamente depois.*

### Iniciar novamente
```bash
docker compose start
```

### Reiniciar a aplicação
```bash
docker compose restart
```

### Parar e remover containers (mantém dados do banco)
```bash
docker compose down
```
*Para iniciar novamente, use: `docker compose up -d`*

### Parar e remover TUDO (incluindo dados do banco)
```bash
docker compose down -v
```
*CUIDADO: Isso apaga todos os dados! Use apenas se quiser começar do zero.*

### Atualizar após mudanças no código
```bash
docker compose down
docker compose up -d --build
```

### Ver quanto de recursos está usando
```bash
docker stats
```
*Pressione Ctrl+C para sair*

---

## Acessar o Banco de Dados Diretamente

Se você precisar executar comandos SQL diretamente:

```bash
docker exec -it facial-emulator-db psql -U emulator -d service_db
```

**Comandos úteis no psql:**
```sql
-- Listar tabelas
\dt

-- Ver estrutura de uma tabela
\d nome_da_tabela

-- Executar consulta
SELECT * FROM service.devices;

-- Sair
\q
```

---

## Problemas Comuns e Soluções

### Problema 1: "docker: command not found"

**Causa:** Docker não está instalado ou não está no PATH

**Solução:**
1. Verifique se o Docker foi instalado corretamente
2. No Windows: Reinicie o Prompt de Comando
3. No Linux: Faça logout e login novamente após instalação

### Problema 2: "permission denied"

**Causa:** No Linux, seu usuário não está no grupo docker

**Solução:**
```bash
sudo usermod -aG docker $USER
# Fazer logout e login novamente
```

**Alternativa temporária:**
```bash
sudo docker compose up -d
```

### Problema 3: "port is already allocated"

**Causa:** Porta 8080 ou 5432 já está em uso

**Solução 1 - Descobrir o que está usando a porta:**

Windows:
```cmd
netstat -ano | findstr :8080
```

Linux/Mac:
```bash
sudo lsof -i :8080
```

**Solução 2 - Mudar a porta no docker-compose.yml:**

Abra o arquivo `docker-compose.yml` e na seção `app`, mude:
```yaml
ports:
  - "8080:8080"
```
Para:
```yaml
ports:
  - "8081:8080"  # Usar porta 8081 em vez de 8080
```

Depois acesse: http://localhost:8081

### Problema 4: Container fica reiniciando

**Verificar o problema:**
```bash
docker compose logs app --tail=100
```

**Causas comuns:**
- Erro de conexão com banco de dados
- Erro de configuração
- Falta de recursos (memória/CPU)

**Solução:**
1. Verifique os logs (comando acima)
2. Se for problema de banco, aguarde o banco ficar "healthy":
```bash
docker compose ps
```
3. Se persistir, recrie tudo:
```bash
docker compose down -v
docker compose up -d
```

### Problema 5: "Cannot connect to Docker daemon"

**Causa:** Docker Desktop não está rodando (Windows/Mac)

**Solução:**
1. Abra o Docker Desktop
2. Aguarde até o ícone ficar verde/estável
3. Tente o comando novamente

**No Linux:**
```bash
sudo systemctl start docker
```

### Problema 6: Página não carrega (erro de conexão)

**Soluções:**
1. Aguarde 30-60 segundos após iniciar
2. Verifique se os containers estão rodando:
```bash
docker compose ps
```
3. Verifique os logs:
```bash
docker compose logs app --tail=50
```
4. Tente acessar: http://127.0.0.1:8080

### Problema 7: Download muito lento

**Causa:** Conexão lenta ou problema no Docker Hub

**Solução:**
- Tenha paciência na primeira execução
- Use uma conexão de internet mais rápida
- Configure um mirror do Docker Hub (avançado)

### Problema 8: Espaço em disco insuficiente

**Ver uso do Docker:**
```bash
docker system df
```

**Limpar recursos não utilizados:**
```bash
docker system prune -a
```
*CUIDADO: Remove imagens, containers e redes não utilizados*

---

## Entendendo o que Foi Instalado

Após executar `docker compose up -d`, o Docker criou:

### Containers (como máquinas virtuais leves)
- **facial-emulator-app**: Roda a aplicação Go
- **facial-emulator-db**: Roda o PostgreSQL

### Volumes (armazenamento persistente)
- **gofacialemulator_postgres_data**: Guarda os dados do banco
- **./logs**: Logs da aplicação (na pasta do projeto)
- **./traces**: Arquivos de trace (na pasta do projeto)

### Rede
- **gofacialemulator_facial-network**: Rede interna para comunicação entre containers

### Ver tudo criado:
```bash
# Containers
docker compose ps

# Volumes
docker volume ls | grep gofacialemulator

# Redes
docker network ls | grep gofacialemulator

# Imagens
docker images | grep gofacialemulator
```

---

## Removendo Completamente

Se você quiser remover tudo relacionado ao projeto:

```bash
# 1. Parar e remover containers
docker compose down -v

# 2. Remover imagens
docker rmi gofacialemulator-app postgres:15-alpine

# 3. Limpar sistema (opcional)
docker system prune -a
```

---

## Backup dos Dados

### Fazer backup do banco de dados

```bash
# Criar pasta para backups
mkdir backups

# Fazer backup
docker exec facial-emulator-db pg_dump -U emulator service_db > backups/service_db_backup.sql
docker exec facial-emulator-db pg_dump -U emulator emulator_db > backups/emulator_db_backup.sql
```

### Restaurar backup

```bash
# Restaurar service_db
docker exec -i facial-emulator-db psql -U emulator -d service_db < backups/service_db_backup.sql

# Restaurar emulator_db
docker exec -i facial-emulator-db psql -U emulator -d emulator_db < backups/emulator_db_backup.sql
```

---

## Dicas Finais

1. **Sempre execute os comandos dentro da pasta do projeto** (onde está o arquivo docker-compose.yml)

2. **Use `docker compose logs` frequentemente** para entender o que está acontecendo

3. **Não delete o volume do PostgreSQL** a menos que queira perder os dados:
   ```bash
   docker volume rm gofacialemulator_postgres_data  # Não faça isso!
   ```

4. **Para atualizações do código**, sempre use:
   ```bash
   docker compose up -d --build
   ```

5. **Documente suas configurações customizadas** (portas, senhas, etc.)

---

## Precisa de Ajuda?

Se algo não funcionar:

1. Verifique os logs: `docker compose logs app --tail=100`
2. Verifique o status: `docker compose ps`
3. Tente recriar: `docker compose down && docker compose up -d`
4. Consulte a seção "Problemas Comuns" acima

---

## Próximos Passos

Agora que a aplicação está rodando:

1. Configure a conexão WXS em: http://localhost:8080/settings
2. Clique em "Refresh DB" para carregar dispositivos
3. Inicie os emuladores
4. Monitore a página de comparação: http://localhost:8080/comparison

Pronto! Você está usando Docker com sucesso.
