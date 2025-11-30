# Configuração de Portas dos Emuladores

## 📋 Visão Geral

Este documento explica como as portas dos emuladores são configuradas e como garantir que funcionem corretamente com Docker.

## 🔌 Range de Portas Configurado

O `docker-compose.yml` está configurado para expor as portas:

```yaml
ports:
  - "8080:8080"              # API principal
  - "4000-4999:4000-4999"    # Emuladores (1000 possíveis)
```

**Total de emuladores suportados:** **1000** (do 4000 ao 4999)

## 🎯 Como Funciona

### **1. Origem das Portas**

As portas dos emuladores **NÃO são definidas pela aplicação Go**. Elas vêm do **sistema WXS (W-Access)**.

```
WXS Database → LocalControllers.Port → GoFacialEmulator → Emulador na porta X
```

### **2. Fluxo de Configuração**

```
┌─────────────────────────────────────────────────────────┐
│ 1. WXS Database (SQL Server)                            │
│    - Tabela: LocalControllers                           │
│    - Campo: Port                                         │
└────────────────┬────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────┐
│ 2. GoFacialEmulator lê as configurações                 │
│    - RefreshDevices() busca dados do WXS                │
│    - Cria emulador usando a porta especificada          │
└────────────────┬────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────┐
│ 3. Emulador inicia na porta especificada                │
│    - Hikvision ou Dahua                                  │
│    - Porta definida pelo WXS                             │
└─────────────────────────────────────────────────────────┘
```

## ⚙️ Configurar Portas no WXS

Para garantir que os emuladores funcionem com o Docker, você precisa configurar as portas **4000-4999** no sistema WXS.

### **Opção 1: Interface do W-Access**

1. Acesse o W-Access
2. Vá em **Configurações** → **Controladores Locais**
3. Para cada controlador, defina a porta no range **4000-4999**

### **Opção 2: Diretamente no SQL Server**

```sql
-- Verificar portas atuais
SELECT LocalControllerID, Name, IPAddress, Port
FROM LocalControllers
ORDER BY Port;

-- Atualizar portas para o range 4000-4999
-- Exemplo: Mudar controladores para começar em 4000
UPDATE LocalControllers
SET Port = 4000 + (ROW_NUMBER() OVER (ORDER BY LocalControllerID) - 1)
WHERE Port < 4000 OR Port > 4999;
```

### **Opção 3: Script de Migração**

```sql
-- Criar tabela temporária com novo mapeamento
DECLARE @NewPort INT = 4000;

UPDATE LocalControllers
SET Port = @NewPort + LocalControllerID
WHERE Port NOT BETWEEN 4000 AND 4999;
```

## 🚀 Após Configurar as Portas

1. **Reinicie o WXS** (se necessário)

2. **Atualize os dispositivos no GoFacialEmulator:**
   - Via interface web: Acesse `/devices` e clique em "Atualizar Dispositivos"
   - Via API: `POST http://localhost:8080/api/devices/refresh`

3. **Verifique os emuladores:**
   ```bash
   # Listar dispositivos
   curl http://localhost:8080/api/devices

   # Testar emulador específico
   curl http://localhost:4001/ISAPI/System/deviceInfo
   ```

## 🔍 Verificar Configuração

### **1. Verificar portas no Docker:**

```bash
docker ps
```

Você deve ver:
```
PORTS
0.0.0.0:4000-4999->4000-4999/tcp
0.0.0.0:8080->8080/tcp
```

### **2. Verificar emuladores ativos:**

```bash
# Via API
curl http://localhost:8080/api/devices

# Resposta exemplo:
# {
#   "devices": [
#     {"id": 1, "port": 4001, "status": "running"},
#     {"id": 2, "port": 4002, "status": "running"}
#   ]
# }
```

### **3. Testar endpoint de um emulador:**

```bash
# Substituir 4001 pela porta do seu emulador
curl http://localhost:4001/ISAPI/System/deviceInfo

# Ou testar o endpoint de eventos
curl -N http://localhost:4001/ISAPI/Event/notification/alertStream
```

## ⚠️ Problemas Comuns

### **Erro: "Connection refused" ao acessar emulador**

**Causa:** Porta não está exposta no Docker ou emulador não está rodando.

**Solução:**
1. Verificar se a porta está no range 4000-4999
2. Verificar se o emulador está rodando: `curl http://localhost:8080/api/devices`
3. Reiniciar o Docker: `docker-compose restart`

### **Erro: "Address already in use"**

**Causa:** Dois emuladores tentando usar a mesma porta.

**Solução:**
1. Verificar portas duplicadas no WXS:
   ```sql
   SELECT Port, COUNT(*) as Count
   FROM LocalControllers
   GROUP BY Port
   HAVING COUNT(*) > 1;
   ```
2. Corrigir portas duplicadas

### **Emulador não aparece após refresh**

**Causa:** Porta fora do range 4000-4999.

**Solução:**
1. Verificar porta no WXS
2. Ajustar para range 4000-4999
3. Fazer refresh novamente

## 📊 Monitoramento

### **Logs dos emuladores:**

```bash
# Logs em tempo real
docker-compose logs -f app

# Filtrar por porta específica
docker-compose logs app | grep "4001"
```

### **Verificar todas as portas em uso:**

```bash
# Linux/Mac
netstat -tuln | grep -E "4[0-9]{3}"

# Windows (PowerShell)
netstat -an | Select-String "4[0-9]{3}"

# Ou via Docker
docker exec facial-emulator-app netstat -tuln | grep LISTEN
```

## 📝 Resumo

- ✅ Range configurado: **4000-4999** (1000 emuladores)
- ✅ Portas definidas pelo **WXS**, não pela aplicação
- ✅ Configurar portas no WXS **ANTES** de criar emuladores
- ✅ Acessar via: `http://localhost:<porta>`
- ✅ API principal: `http://localhost:8080`

## 🔗 Links Úteis

- [DOCKER.md](DOCKER.md) - Guia completo do Docker
- [README.md](README.md) - Documentação geral do projeto
