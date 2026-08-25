# Guia de Integração: Sistema de Pool Dinâmico

Este guia explica como integrar o novo sistema de pools dinâmicos com separação de reads/writes.

## 📊 Arquitetura

```
┌─────────────────────────────────────────────────────────────────┐
│                       300-1000 Emuladores                       │
└────────┬────────────────────────────────────────┬───────────────┘
         │                                        │
         │ READS (rápido, direto)                 │ WRITES (ordenado, fila)
         ↓                                        ↓
┌──────────────────────┐             ┌──────────────────────────────┐
│   Read Pool          │             │   OrderedWriteQueue          │
│   Adaptive           │             │   - N workers (5-50)         │
│   Min: 5-50          │             │   - Queue: 1K-50K ops        │
│   Max: 20-600+       │             │   - Batching automático      │
│   Auto-scaling       │             │   - Retry com backoff        │
└──────────────────────┘             │   - Ordem garantida/emulator │
         │                           └──────────────────────────────┘
         │                                        │
         │                                        ↓
         │                           ┌──────────────────────────┐
         │                           │    Write Pool            │
         │                           │    Min: 5-10             │
         │                           │    Max: 6-60             │
         └───────────────┬───────────┴───────────┘
                         ↓
                 ┌──────────────────┐
                 │   PostgreSQL     │
                 │   max_conns:     │
                 │   100-800+       │
                 └──────────────────┘
```

## 🚀 Configuração Dinâmica

O sistema calcula automaticamente os tamanhos baseado no número de emuladores:

### Para 300 emuladores:
```
READ POOL:     Min: 75,  Initial: 375,  Max: 750
WRITE POOL:    Min: 17,  Max: 21
WRITE WORKERS: 17
QUEUE SIZE:    1500 ops
BATCH SIZE:    24 ops/batch
```

### Para 1000 emuladores:
```
READ POOL:     Min: 250, Initial: 1250, Max: 2500
WRITE POOL:    Min: 31,  Max: 38
WRITE WORKERS: 31
QUEUE SIZE:    5000 ops
BATCH SIZE:    30 ops/batch
```

## 📝 Mudanças no Código

### 1. Modificar Manager para usar DualPoolManager

**Antes (manager.go):**
```go
type Manager struct {
    ServiceDB  *database.AdaptivePool    // Pool único
    EmulatorDB *database.AdaptivePool    // Pool único
    ...
}
```

**Depois (manager.go):**
```go
type Manager struct {
    ServiceDB  *database.DualPoolManager  // Pool duplo
    EmulatorDB *database.DualPoolManager  // Pool duplo
    ...
}
```

### 2. Atualizar inicialização no main.go

**Antes:**
```go
serviceDB, err := database.NewAdaptivePool(serviceDSN, 0)
emulatorDB, err := database.NewAdaptivePool(emulatorDSN, 300)
```

**Depois:**
```go
// Contar emuladores dinamicamente
devices, err := countDevicesFromWxs(wxsDB)
emulatorCount := len(devices)

// Criar pools dinâmicos
serviceDB, err := database.NewDualPoolManager(serviceDSN, emulatorCount, tracer)
emulatorDB, err := database.NewDualPoolManager(emulatorDSN, emulatorCount, tracer)

// Log da configuração
tracer.Info("Database pools initialized for %d emulators", emulatorCount)
tracer.Info("Recommended PostgreSQL max_connections: %d",
    serviceDB.config.GetRecommendedPostgresMaxConnections())
```

### 3. Usar contexto com EmulatorID

**Em todos os métodos do Repository (hikvision/repository.go, dahua/repository.go):**

```go
// Antes
func (r *Repository) AddUser(user *User) error {
    query := "INSERT INTO users ..."
    _, err := r.db.Exec(context.Background(), query, args...)
    return err
}

// Depois
func (r *Repository) AddUser(user *User) error {
    // Criar contexto com EmulatorID para garantir ordem de operações
    ctx := database.WithEmulatorID(context.Background(), r.deviceID)

    query := "INSERT INTO users ..."
    _, err := r.db.Exec(ctx, query, args...)
    return err
}
```

**Adicionar deviceID ao Repository:**
```go
type Repository struct {
    db       database.DBInterface
    deviceID int  // NOVO: ID do emulador
    tracer   *trace.Tracer
}

func NewRepository(db database.DBInterface, deviceID int, tracer *trace.Tracer) *Repository {
    return &Repository{
        db:       db,
        deviceID: deviceID,
        tracer:   tracer,
    }
}
```

### 4. Operações que NÃO precisam de ordem (podem ser diretas)

Para operações administrativas que não precisam de ordenação:

```go
// Usar ExecDirect para bypass da fila
if dpm, ok := r.db.(*database.DualPoolManager); ok {
    _, err = dpm.ExecDirect(context.Background(), query, args...)
} else {
    _, err = r.db.Exec(context.Background(), query, args...)
}
```

Exemplos:
- UPDATE devices SET status = 'running'
- DELETE FROM settings WHERE ...
- Operações de watchdog
- Health checks

## ⚙️ Configuração do PostgreSQL

Ajustar `postgresql.conf`:

```ini
# Para 300 emuladores (recomendado: 1000)
max_connections = 1000

# Para 1000 emuladores (recomendado: 3250)
max_connections = 3250

# Outras configurações importantes
shared_buffers = 4GB
effective_cache_size = 12GB
work_mem = 8MB
maintenance_work_mem = 512MB
```

## 📊 Monitoramento

### Verificar estatísticas dos pools:

```go
stats := emulatorDB.GetStats()
fmt.Printf("%+v\n", stats)

// Output:
// {
//   "read_pool": {
//     "max_conns": 750,
//     "acquired_conns": 342,
//     "idle_conns": 408,
//     "utilization": 45.6%
//   },
//   "write_pool": {
//     "max_conns": 21,
//     "acquired_conns": 17,
//     "idle_conns": 4
//   },
//   "write_queue": {
//     "total_operations": 152430,
//     "total_batches": 6350,
//     "total_errors": 23,
//     "total_retries": 89,
//     "queue_utilization": 23.4%,
//     "avg_latency_ms": 45
//   }
// }
```

### Endpoint de métricas:

```go
router.GET("/metrics/database", func(c *gin.Context) {
    c.JSON(200, gin.H{
        "service_db":  manager.ServiceDB.GetStats(),
        "emulator_db": manager.EmulatorDB.GetStats(),
    })
})
```

## 🔥 Troubleshooting

### Erro: "timeout enqueuing operation (queue full)"

**Causa:** Write queue está cheia (muitas operações pendentes)

**Solução:**
1. Aumentar WriteQueueSize em `dynamic_config.go` (multiplicador maior)
2. Verificar se há operações travadas (deadlocks)
3. Aumentar número de workers

### Erro: "failed after 3 attempts: too many clients already"

**Causa:** PostgreSQL atingiu max_connections

**Solução:**
1. Aumentar `max_connections` no PostgreSQL
2. Verificar se outras aplicações estão usando conexões
3. Reduzir ReadPoolMax se necessário

### Operações lentas

**Sintomas:** `avg_latency_ms` > 1000ms

**Causas possíveis:**
1. Batch size muito grande (operações aguardando flush)
2. Contenção no write pool
3. Queries lentas no PostgreSQL

**Soluções:**
1. Reduzir `WriteBatchSize`
2. Reduzir `WriteFlushMs` (flush mais frequente)
3. Adicionar índices no banco

## 🧪 Testes

### Teste de carga com 300 emuladores:

```bash
# 1. Ajustar postgresql.conf
max_connections = 1000

# 2. Reiniciar PostgreSQL
sudo systemctl restart postgresql

# 3. Executar aplicação
./emulator-service

# 4. Monitorar logs
tail -f logs/app.log | grep -E "OrderedWriteQueue|DualPoolManager"

# 5. Verificar métricas
curl http://localhost:8080/metrics/database
```

### Teste de carga com 1000 emuladores:

```bash
# Ajustar max_connections primeiro!
max_connections = 3500

# Executar e monitorar
./emulator-service --emulators=1000
```

## 📈 Benefícios Esperados

1. **Redução de erros 500:** De ~40% para <1%
2. **Latência de writes:** De 2-5s para 50-200ms
3. **Throughput:** 3-5x maior
4. **Escalabilidade:** Suporta 1000+ emuladores
5. **Uso de conexões:** Redução de 80% (300 emuls: de 600 conns → 120 conns)

## ⚠️ Avisos Importantes

1. **Transações:** Transações não usam a fila (controle direto necessário)
2. **Ordem:** Apenas operações do MESMO emulador têm ordem garantida
3. **Context deadline:** Operações na fila têm timeout de 30s
4. **Batch flush:** Operações podem ter latência de até `WriteFlushMs`
5. **Shutdown:** Sempre chamar `.Close()` para flush de operações pendentes
