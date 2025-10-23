# Checklist de Instalação Rápida

Use este guia como checklist visual. Marque cada item conforme for completando.

---

## ETAPA 1: Instalar Docker

### Windows
- [ ] Baixar Docker Desktop de: https://www.docker.com/products/docker-desktop
- [ ] Executar o instalador
- [ ] Reiniciar o computador
- [ ] Abrir Prompt de Comando
- [ ] Testar: `docker --version`
- [ ] Deve mostrar a versão do Docker

### Linux
- [ ] Abrir terminal
- [ ] Executar comando de instalação (ver GUIA-INSTALACAO-DOCKER.md)
- [ ] Adicionar usuário ao grupo docker: `sudo usermod -aG docker $USER`
- [ ] Fazer logout e login
- [ ] Testar: `docker --version`
- [ ] Testar: `docker compose version`

### Mac
- [ ] Baixar Docker Desktop de: https://www.docker.com/products/docker-desktop
- [ ] Escolher versão correta (Intel ou Apple Silicon)
- [ ] Instalar e abrir o Docker
- [ ] Abrir Terminal
- [ ] Testar: `docker --version`

---

## ETAPA 2: Preparar o Projeto

- [ ] Extrair arquivos do ZIP ou clonar repositório Git
- [ ] Abrir terminal/prompt de comando
- [ ] Navegar até a pasta do projeto
  - Windows: `cd C:\Projetos\GoFacialEmulator`
  - Linux/Mac: `cd ~/GoFacialEmulator`
- [ ] Verificar se está na pasta certa: `dir` (Windows) ou `ls` (Linux/Mac)
- [ ] Confirmar que vê o arquivo `docker-compose.yml`

---

## ETAPA 3: Iniciar a Aplicação

- [ ] Na pasta do projeto, executar: `docker compose up -d`
- [ ] Aguardar o download das imagens (primeira vez pode demorar)
- [ ] Aguardar a compilação da aplicação
- [ ] Ver mensagem: "Container facial-emulator-app Started"
- [ ] Ver mensagem: "Container facial-emulator-db Started"

---

## ETAPA 4: Verificar Funcionamento

- [ ] Executar: `docker compose ps`
- [ ] Ver status "Up" nos dois containers
- [ ] Ver "(healthy)" no container do banco
- [ ] Executar: `docker compose logs app --tail=20`
- [ ] Ver mensagem: "Starting HTTP server on 0.0.0.0:8080"
- [ ] Abrir navegador
- [ ] Acessar: http://localhost:8080
- [ ] Ver a interface do GoFacialEmulator

---

## ETAPA 5: Configurar (Opcional)

Se precisar conectar ao WXS:

- [ ] Acessar: http://localhost:8080/settings
- [ ] Preencher dados do servidor WXS
  - [ ] Host/IP
  - [ ] Porta (geralmente 1433)
  - [ ] Nome do banco
  - [ ] Usuário
  - [ ] Senha
- [ ] Clicar em "Testar Conexão"
- [ ] Verificar mensagem de sucesso
- [ ] Clicar em "Salvar"

---

## ETAPA 6: Usar a Aplicação

- [ ] Voltar para: http://localhost:8080
- [ ] Clicar em "Refresh DB"
- [ ] Aguardar carregamento dos dispositivos
- [ ] Ver lista de dispositivos
- [ ] Clicar em "Start All" ou "Start" em um dispositivo
- [ ] Ver status mudar para "Running"

---

## Comandos para Copiar e Colar

### Verificar status
```bash
docker compose ps
```

### Ver logs
```bash
docker compose logs app --tail=50
```

### Acompanhar logs em tempo real
```bash
docker compose logs -f app
```

### Parar
```bash
docker compose stop
```

### Iniciar novamente
```bash
docker compose start
```

### Reiniciar
```bash
docker compose restart
```

### Parar e remover (mantém dados)
```bash
docker compose down
```

### Reconstruir após mudanças
```bash
docker compose down
docker compose up -d --build
```

---

## Problemas? Use este fluxograma:

```
NÃO FUNCIONA?
    |
    ├─> Container não inicia?
    |       └─> Ver logs: docker compose logs app --tail=100
    |
    ├─> Porta em uso?
    |       └─> Mudar porta no docker-compose.yml (8080 -> 8081)
    |
    ├─> Página não carrega?
    |       ├─> Aguardar 60 segundos
    |       └─> Verificar status: docker compose ps
    |
    └─> Outro erro?
            └─> Recriar tudo: docker compose down -v && docker compose up -d
```

---

## Checklist de Sucesso Final

Você instalou com sucesso quando:

- [ ] `docker compose ps` mostra Status = "Up"
- [ ] http://localhost:8080 carrega a interface
- [ ] Logs não mostram erros: `docker compose logs app --tail=20`
- [ ] Health check funciona: http://localhost:8080/health

---

## Precisa Desinstalar Tudo?

Execute em ordem:

```bash
# 1. Parar containers
docker compose down -v

# 2. Remover imagens
docker rmi gofacialemulator-app postgres:15-alpine

# 3. Limpar sistema (opcional)
docker system prune -a
```

---

## Comandos de Emergência

Se algo der muito errado:

```bash
# Parar tudo
docker compose down -v

# Limpar completamente o Docker (cuidado!)
docker system prune -a --volumes

# Começar do zero
docker compose up -d --build
```

---

## Atalhos Úteis

Crie aliases para facilitar (opcional):

### Windows (PowerShell)
Adicione ao perfil do PowerShell:
```powershell
function dc-up { docker compose up -d }
function dc-down { docker compose down }
function dc-logs { docker compose logs -f app }
function dc-restart { docker compose restart }
```

### Linux/Mac (Bash/Zsh)
Adicione ao ~/.bashrc ou ~/.zshrc:
```bash
alias dc-up='docker compose up -d'
alias dc-down='docker compose down'
alias dc-logs='docker compose logs -f app'
alias dc-restart='docker compose restart'
```

---

## Conclusão

✅ Se você marcou todos os itens das Etapas 1-4, está funcionando!

✅ Guarde este checklist para referência rápida

✅ Consulte GUIA-INSTALACAO-DOCKER.md para detalhes completos

✅ Em caso de problemas, veja a seção "Problemas Comuns" no guia completo
