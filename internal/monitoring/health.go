package monitoring

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// HealthStatus representa o status de saúde de um componente
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// ComponentHealth representa a saúde de um componente individual
type ComponentHealth struct {
	Name       string                 `json:"name"`
	Status     HealthStatus           `json:"status"`
	Message    string                 `json:"message,omitempty"`
	LastCheck  time.Time              `json:"last_check"`
	Duration   time.Duration          `json:"duration_ms"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	ErrorCount int64                  `json:"error_count"`
}

// HealthChecker interface para componentes verificáveis
type HealthChecker interface {
	Name() string
	Check(ctx context.Context) ComponentHealth
}

// HealthMonitor gerencia verificações de saúde de todos os componentes
type HealthMonitor struct {
	checkers []HealthChecker
	mu       sync.RWMutex

	// Métricas globais
	totalChecks   atomic.Int64
	totalFailures atomic.Int64
	lastCheckTime atomic.Value // time.Time

	// Cache de resultados
	cachedResults     map[string]ComponentHealth
	cacheExpiry       time.Time
	cacheDuration     time.Duration
	cacheResultsMutex sync.RWMutex
}

// NewHealthMonitor cria um novo monitor de saúde
func NewHealthMonitor() *HealthMonitor {
	hm := &HealthMonitor{
		checkers:      make([]HealthChecker, 0),
		cachedResults: make(map[string]ComponentHealth),
		cacheDuration: 5 * time.Second, // Cache por 5 segundos
	}
	hm.lastCheckTime.Store(time.Time{})
	return hm
}

// RegisterChecker adiciona um checker ao monitor
func (hm *HealthMonitor) RegisterChecker(checker HealthChecker) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.checkers = append(hm.checkers, checker)
}

// CheckAll verifica a saúde de todos os componentes
func (hm *HealthMonitor) CheckAll(ctx context.Context) map[string]interface{} {
	// Verificar cache primeiro
	if hm.isCacheValid() {
		return hm.getCachedResponse()
	}

	start := time.Now()
	hm.totalChecks.Add(1)

	// Executar checks em paralelo
	resultsChan := make(chan ComponentHealth, len(hm.checkers))
	var wg sync.WaitGroup

	hm.mu.RLock()
	checkers := make([]HealthChecker, len(hm.checkers))
	copy(checkers, hm.checkers)
	hm.mu.RUnlock()

	for _, checker := range checkers {
		wg.Add(1)
		go func(c HealthChecker) {
			defer wg.Done()

			// Timeout individual por check
			checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()

			result := c.Check(checkCtx)
			resultsChan <- result
		}(checker)
	}

	// Aguardar todas as verificações
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Coletar resultados
	components := make(map[string]ComponentHealth)
	overallStatus := HealthStatusHealthy
	unhealthyCount := 0

	for result := range resultsChan {
		components[result.Name] = result

		if result.Status == HealthStatusUnhealthy {
			unhealthyCount++
			overallStatus = HealthStatusUnhealthy
			hm.totalFailures.Add(1)
		} else if result.Status == HealthStatusDegraded && overallStatus == HealthStatusHealthy {
			overallStatus = HealthStatusDegraded
		}
	}

	// Métricas do sistema
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	response := map[string]interface{}{
		"status":     overallStatus,
		"timestamp":  time.Now().UTC(),
		"duration":   time.Since(start).Milliseconds(),
		"components": components,
		"metrics": map[string]interface{}{
			"total_checks":      hm.totalChecks.Load(),
			"total_failures":    hm.totalFailures.Load(),
			"unhealthy_count":   unhealthyCount,
			"goroutines":        runtime.NumGoroutine(),
			"memory_alloc_mb":   memStats.Alloc / 1024 / 1024,
			"memory_sys_mb":     memStats.Sys / 1024 / 1024,
			"gc_pause_ms":       float64(memStats.PauseNs[(memStats.NumGC+255)%256]) / 1000000,
			"num_gc":            memStats.NumGC,
		},
	}

	// Atualizar cache
	hm.updateCache(components)
	hm.lastCheckTime.Store(time.Now())

	return response
}

// QuickCheck retorna status rápido (usa cache se disponível)
func (hm *HealthMonitor) QuickCheck() map[string]interface{} {
	if hm.isCacheValid() {
		return hm.getCachedResponse()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return hm.CheckAll(ctx)
}

// isCacheValid verifica se o cache ainda é válido
func (hm *HealthMonitor) isCacheValid() bool {
	hm.cacheResultsMutex.RLock()
	defer hm.cacheResultsMutex.RUnlock()
	return time.Now().Before(hm.cacheExpiry) && len(hm.cachedResults) > 0
}

// getCachedResponse retorna resposta em cache
func (hm *HealthMonitor) getCachedResponse() map[string]interface{} {
	hm.cacheResultsMutex.RLock()
	defer hm.cacheResultsMutex.RUnlock()

	// Calcular status geral
	overallStatus := HealthStatusHealthy
	unhealthyCount := 0

	for _, result := range hm.cachedResults {
		if result.Status == HealthStatusUnhealthy {
			unhealthyCount++
			overallStatus = HealthStatusUnhealthy
		} else if result.Status == HealthStatusDegraded && overallStatus == HealthStatusHealthy {
			overallStatus = HealthStatusDegraded
		}
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return map[string]interface{}{
		"status":     overallStatus,
		"timestamp":  time.Now().UTC(),
		"cached":     true,
		"components": hm.cachedResults,
		"metrics": map[string]interface{}{
			"total_checks":    hm.totalChecks.Load(),
			"total_failures":  hm.totalFailures.Load(),
			"unhealthy_count": unhealthyCount,
			"goroutines":      runtime.NumGoroutine(),
			"memory_alloc_mb": memStats.Alloc / 1024 / 1024,
		},
	}
}

// updateCache atualiza o cache de resultados
func (hm *HealthMonitor) updateCache(results map[string]ComponentHealth) {
	hm.cacheResultsMutex.Lock()
	defer hm.cacheResultsMutex.Unlock()

	hm.cachedResults = results
	hm.cacheExpiry = time.Now().Add(hm.cacheDuration)
}

// DatabaseHealthChecker verifica a saúde da conexão com banco de dados
type DatabaseHealthChecker struct {
	name          string
	pingFn        func(ctx context.Context) error
	getStatsFn    func() map[string]interface{}
	failureStatus HealthStatus
}

// NewDatabaseHealthChecker cria um novo checker de banco de dados. Falha de
// ping reporta HealthStatusUnhealthy — use para dependências obrigatórias
// (ex.: service_db, emulator_db) cuja indisponibilidade de fato impede a
// aplicação de funcionar.
func NewDatabaseHealthChecker(name string, pingFn func(ctx context.Context) error, getStatsFn func() map[string]interface{}) *DatabaseHealthChecker {
	return NewDatabaseHealthCheckerWithFailureStatus(name, pingFn, getStatsFn, HealthStatusUnhealthy)
}

// NewDatabaseHealthCheckerWithFailureStatus é como NewDatabaseHealthChecker,
// mas permite escolher o status reportado quando o ping falha. Use
// HealthStatusDegraded para dependências opcionais (ex.: wxs_db) cuja
// indisponibilidade não deve derrubar o status geral da aplicação para
// unhealthy (e, por consequência, o HTTP 503 de /monitoring/health/quick) —
// a aplicação continua funcionando, só as features que dependem dela ficam
// degradadas.
func NewDatabaseHealthCheckerWithFailureStatus(name string, pingFn func(ctx context.Context) error, getStatsFn func() map[string]interface{}, failureStatus HealthStatus) *DatabaseHealthChecker {
	return &DatabaseHealthChecker{
		name:          name,
		pingFn:        pingFn,
		getStatsFn:    getStatsFn,
		failureStatus: failureStatus,
	}
}

func (d *DatabaseHealthChecker) Name() string {
	return d.name
}

func (d *DatabaseHealthChecker) Check(ctx context.Context) ComponentHealth {
	start := time.Now()
	health := ComponentHealth{
		Name:      d.name,
		LastCheck: start,
	}

	// Testar ping
	err := d.pingFn(ctx)
	duration := time.Since(start)
	health.Duration = duration

	if err != nil {
		health.Status = d.failureStatus
		health.Message = fmt.Sprintf("Database ping failed: %v", err)
		health.ErrorCount = 1
		return health
	}

	// Obter estatísticas
	stats := d.getStatsFn()
	health.Metadata = stats

	// Avaliar status baseado em pool stats
	if activeConns, ok := stats["active_connections"].(int32); ok {
		if maxConns, ok := stats["max_connections"].(int32); ok {
			usage := float64(activeConns) / float64(maxConns) * 100

			if usage > 90 {
				health.Status = HealthStatusDegraded
				health.Message = fmt.Sprintf("High connection usage: %.1f%%", usage)
			} else if usage > 50 {
				health.Status = HealthStatusHealthy
				health.Message = fmt.Sprintf("Normal operation: %.1f%% usage", usage)
			} else {
				health.Status = HealthStatusHealthy
				health.Message = "Optimal"
			}
		}
	}

	// Avaliar latência
	if duration > 100*time.Millisecond {
		health.Status = HealthStatusDegraded
		health.Message = fmt.Sprintf("High latency: %dms", duration.Milliseconds())
	}

	return health
}

// EmulatorHealthChecker verifica a saúde dos emuladores
type EmulatorHealthChecker struct {
	getDevicesFn func() ([]map[string]interface{}, error)
}

func NewEmulatorHealthChecker(getDevicesFn func() ([]map[string]interface{}, error)) *EmulatorHealthChecker {
	return &EmulatorHealthChecker{
		getDevicesFn: getDevicesFn,
	}
}

func (e *EmulatorHealthChecker) Name() string {
	return "emulators"
}

func (e *EmulatorHealthChecker) Check(ctx context.Context) ComponentHealth {
	start := time.Now()
	health := ComponentHealth{
		Name:      "emulators",
		LastCheck: start,
		Metadata:  make(map[string]interface{}),
	}

	devices, err := e.getDevicesFn()
	duration := time.Since(start)
	health.Duration = duration

	if err != nil {
		health.Status = HealthStatusUnhealthy
		health.Message = fmt.Sprintf("Failed to get devices: %v", err)
		health.ErrorCount = 1
		return health
	}

	// Contar status
	totalDevices := len(devices)
	runningCount := 0
	stoppedCount := 0
	errorCount := 0

	for _, device := range devices {
		if status, ok := device["status"].(string); ok {
			switch status {
			case "running":
				runningCount++
			case "stopped":
				stoppedCount++
			case "error":
				errorCount++
			}
		}
	}

	health.Metadata["total_devices"] = totalDevices
	health.Metadata["running"] = runningCount
	health.Metadata["stopped"] = stoppedCount
	health.Metadata["errors"] = errorCount

	// Avaliar status
	if errorCount > totalDevices/10 { // Mais de 10% com erro
		health.Status = HealthStatusDegraded
		health.Message = fmt.Sprintf("%d devices with errors", errorCount)
		health.ErrorCount = int64(errorCount)
	} else if runningCount == 0 && totalDevices > 0 {
		health.Status = HealthStatusDegraded
		health.Message = "No emulators running"
	} else {
		health.Status = HealthStatusHealthy
		health.Message = fmt.Sprintf("%d/%d emulators running", runningCount, totalDevices)
	}

	return health
}
