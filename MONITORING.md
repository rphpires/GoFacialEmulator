# 📊 Sistema de Monitoramento e Métricas

Este documento descreve o sistema de análise de métricas e monitoramento implementado no GoFacialEmulator.

## 📁 Arquivos do Sistema

- `internal/monitoring/metrics.go` - Coleta e armazenamento de métricas
- `internal/monitoring/health.go` - Sistema de health checks
- `internal/monitoring/middleware.go` - Middlewares de coleta automática
- `internal/handlers/handlers.go` - Endpoints HTTP para acesso

## 🎯 Recursos Disponíveis

### 1. Sistema de Métricas

#### HTTP Requests
- Total de requisições processadas
- Contagem de erros (4xx e 5xx)
- Taxa de erros (%)
- Duração média das requisições
- Percentis de latência (P50, P95, P99)
- Requisições por segundo (RPS)
- Histórico das últimas 1000 requisições

#### Métricas por Endpoint
- Rastreamento individual de cada rota
- Contagem de requisições por endpoint
- Erros específicos por endpoint
- Duração média de cada endpoint
- Último erro registrado com timestamp

#### Emuladores
- Total de tentativas de início
- Total de falhas ao iniciar
- Total de tentativas de parada
- Total de falhas ao parar
- Taxa de sucesso de inicialização

#### Database
- Total de queries executadas
- Total de erros em queries
- Taxa de erros (%)
- Duração média das queries
- Percentis de latência (P50, P95, P99)

### 2. Sistema de Health Check

Verifica a saúde dos seguintes componentes:

- **service_db** - Banco de dados principal do serviço
- **emulator_db** - Banco de dados dos emuladores
- **wxs_db** - Banco de dados WXS (se configurado)
- **emulators** - Status dos emuladores ativos

Para cada componente, fornece:
- Status (healthy, degraded, unhealthy)
- Mensagem descritiva
- Tempo de resposta
- Metadados adicionais (uso de pool, conexões ativas, etc.)

### 3. Métricas de Sistema

- **Uptime** - Tempo desde o início do serviço
- **Goroutines** - Número de goroutines ativas
- **Memória**:
  - Memória alocada (MB)
  - Memória total do sistema (MB)
  - Heap em uso (MB)
  - Heap disponível (MB)
  - Número de garbage collections
  - Pausa média do GC (ms)

## 🌐 Endpoints de Acesso

### Health Checks

#### Health Check Completo
```bash
GET /monitoring/health
```

Retorna verificação detalhada de todos os componentes. Executa testes em tempo real.

**Resposta HTTP:**
- `200 OK` - Sistema saudável ou degradado
- `503 Service Unavailable` - Sistema não saudável

**Exemplo de resposta:**
```json
{
  "status": "healthy",
  "timestamp": "2024-12-04T15:30:00Z",
  "duration": 45,
  "components": {
    "service_db": {
      "name": "service_db",
      "status": "healthy",
      "message": "Optimal",
      "last_check": "2024-12-04T15:30:00Z",
      "duration_ms": 15,
      "metadata": {
        "active_connections": 5,
        "max_connections": 20
      }
    },
    "emulators": {
      "name": "emulators",
      "status": "healthy",
      "message": "15/20 emulators running"
    }
  },
  "metrics": {
    "total_checks": 150,
    "total_failures": 2,
    "unhealthy_count": 0,
    "goroutines": 25,
    "memory_alloc_mb": 45,
    "num_gc": 10
  }
}
```

#### Health Check Rápido
```bash
GET /monitoring/health/quick
```

Retorna resultado em cache (atualizado a cada 5 segundos). Mais rápido que o health check completo.

### Métricas

#### Snapshot Completo
```bash
GET /monitoring/metrics
```

Retorna todas as métricas coletadas desde o início do serviço.

**Exemplo de resposta:**
```json
{
  "uptime_seconds": 3600,
  "http": {
    "total_requests": 1500,
    "total_errors": 15,
    "total_4xx": 10,
    "total_5xx": 5,
    "error_rate": 1.0,
    "avg_duration_ms": 25.5,
    "p50_duration_ms": 20.0,
    "p95_duration_ms": 50.0,
    "p99_duration_ms": 100.0,
    "requests_per_second": 0.42
  },
  "emulators": {
    "total_start_attempts": 50,
    "total_start_failures": 2,
    "total_stop_attempts": 30,
    "total_stop_failures": 0,
    "start_success_rate": 96.0
  },
  "database": {
    "total_queries": 5000,
    "total_errors": 5,
    "error_rate": 0.1,
    "avg_duration_ms": 10.5,
    "p50_duration_ms": 8.0,
    "p95_duration_ms": 25.0,
    "p99_duration_ms": 50.0
  },
  "endpoints": {
    "/api/devices": {
      "total_requests": 250,
      "total_errors": 2,
      "avg_duration_ms": 15.5
    }
  }
}
```

#### Métricas por Endpoint
```bash
GET /monitoring/metrics/endpoints
```

Retorna métricas detalhadas de cada endpoint individualmente.

#### Top Endpoints com Erros
```bash
GET /monitoring/metrics/errors?limit=10
```

Retorna os endpoints com mais erros 5xx.

**Parâmetros:**
- `limit` (opcional) - Número de endpoints a retornar (padrão: 10)

**Exemplo de resposta:**
```json
{
  "top_errors": [
    {
      "path": "/api/devices/123/start",
      "total_5xx": 5,
      "total_requests": 100,
      "last_error": "Failed to start device",
      "last_error_time": "2024-12-04T15:30:00Z"
    }
  ],
  "limit": 10,
  "timestamp": "2024-12-04T15:30:00Z"
}
```

### Debug e Diagnóstico

#### Contagem de Goroutines
```bash
GET /monitoring/debug/goroutines
```

Retorna o número atual de goroutines em execução.

**Exemplo de resposta:**
```json
{
  "goroutines": 25,
  "timestamp": "2024-12-04T15:30:00Z"
}
```

#### Estatísticas de Memória
```bash
GET /monitoring/debug/memory
```

Retorna estatísticas detalhadas de uso de memória.

**Exemplo de resposta:**
```json
{
  "alloc_mb": 45,
  "total_alloc_mb": 150,
  "sys_mb": 70,
  "num_gc": 10,
  "gc_pause_ms": 0.5,
  "heap_alloc_mb": 45,
  "heap_sys_mb": 60,
  "heap_idle_mb": 15,
  "heap_inuse_mb": 45,
  "timestamp": "2024-12-04T15:30:00Z"
}
```

## 🔧 Exemplos de Uso

### cURL

```bash
# Ver todas as métricas
curl http://localhost:8080/monitoring/metrics

# Ver health check completo
curl http://localhost:8080/monitoring/health

# Ver health check rápido
curl http://localhost:8080/monitoring/health/quick

# Ver top 5 endpoints com mais erros
curl http://localhost:8080/monitoring/metrics/errors?limit=5

# Ver estatísticas de memória
curl http://localhost:8080/monitoring/debug/memory

# Ver contagem de goroutines
curl http://localhost:8080/monitoring/debug/goroutines

# Ver métricas por endpoint
curl http://localhost:8080/monitoring/metrics/endpoints
```

### PowerShell

```powershell
# Ver todas as métricas
Invoke-RestMethod -Uri "http://localhost:8080/monitoring/metrics" -Method Get

# Ver health check
Invoke-RestMethod -Uri "http://localhost:8080/monitoring/health" -Method Get

# Ver uso de memória
Invoke-RestMethod -Uri "http://localhost:8080/monitoring/debug/memory" -Method Get
```

### Navegador

Acesse diretamente no navegador:
- http://localhost:8080/monitoring/metrics
- http://localhost:8080/monitoring/health
- http://localhost:8080/monitoring/debug/memory

## 📈 Interpretação dos Dados

### Status de Health

- **healthy** ✅ - Componente funcionando normalmente
- **degraded** ⚠️ - Componente funcionando com problemas menores (ex: alta latência, uso elevado de recursos)
- **unhealthy** ❌ - Componente com falha crítica

### Percentis de Latência

- **P50 (mediana)** - 50% das requisições são mais rápidas que este valor
- **P95** - 95% das requisições são mais rápidas que este valor
- **P99** - 99% das requisições são mais rápidas que este valor

### Taxa de Erros

- **< 1%** - Excelente
- **1-5%** - Aceitável (investigar)
- **> 5%** - Crítico (requer ação imediata)

### Uso de Conexões

- **< 50%** - Saudável
- **50-90%** - Monitorar
- **> 90%** - Degradado (considerar aumentar o pool)

## 🔄 Coleta Automática

As métricas são coletadas automaticamente através de middlewares:

- **MetricsMiddleware** - Coleta métricas de todas as requisições HTTP
- **RecoveryMiddleware** - Captura e registra panics
- **RequestLoggerMiddleware** - Log detalhado (quando ativado)

Não é necessário instrumentar manualmente o código. As métricas são coletadas automaticamente para:
- Todas as rotas HTTP
- Queries de banco de dados (quando usando o pool adaptativo)
- Operações de emuladores (start/stop)

## 🎨 Integração com Ferramentas

### Grafana / Prometheus

Para integrar com Prometheus, você pode criar um endpoint de exportação ou usar um exporter customizado que consulte `/monitoring/metrics`.

### Dashboards Personalizados

Os dados JSON podem ser facilmente consumidos por:
- Ferramentas de BI
- Dashboards customizados em React/Vue/Angular
- Scripts de monitoramento
- Alertas automatizados

## 🚨 Alertas Sugeridos

Configure alertas para:

1. **Taxa de erro > 5%**
2. **P99 de latência > 1000ms**
3. **Uso de conexões DB > 90%**
4. **Health status = unhealthy**
5. **Taxa de falha de start de emuladores > 10%**
6. **Memória > 80% do limite**
7. **Número de goroutines crescendo constantemente**

## 📝 Notas

- O sistema mantém um histórico das últimas 1000 requisições para cálculo de percentis
- O cache de health check é atualizado a cada 5 segundos
- Métricas são thread-safe usando atomic operations
- Não há impacto significativo de performance na coleta de métricas
