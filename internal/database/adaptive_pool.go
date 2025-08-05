// internal/database/adaptive_pool.go
package database

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// AdaptivePool - Pool que se adapta automaticamente à carga
type AdaptivePool struct {
	pool    *pgxpool.Pool
	metrics *PoolMetrics
	config  *AdaptiveConfig
	mu      sync.RWMutex
}

// PoolMetrics coleta estatísticas de uso
type PoolMetrics struct {
	queriesPerSecond int64
	avgQueryDuration int64 // em milliseconds
	peakConnections  int32
	lastAdjustment   time.Time
	connectionsPeak  int32
}

// AdaptiveConfig configuração do pool adaptativo
type AdaptiveConfig struct {
	MinConns           int32
	MaxConns           int32
	BaseMaxConns       int32 // Máximo inicial conservador
	AbsoluteMaxConns   int32 // Limite absoluto (nunca ultrapassar)
	AdjustmentInterval time.Duration
	ShrinkThreshold    float64 // % de uso abaixo do qual reduzir conexões
	GrowthThreshold    float64 // % de uso acima do qual aumentar conexões
}

// NewAdaptivePool cria um pool que se adapta à carga
func NewAdaptivePool(dbURL string, emulatorCount int) (*AdaptivePool, error) {
	// Calcular configuração inicial baseada no número de emuladores
	config := calculateInitialConfig(emulatorCount)

	poolConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return nil, err
	}

	// Configuração inicial conservadora
	poolConfig.MinConns = config.MinConns
	poolConfig.MaxConns = config.BaseMaxConns
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.MaxConnIdleTime = 5 * time.Minute // Menor idle time
	poolConfig.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.ConnectConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, err
	}

	ap := &AdaptivePool{
		pool:   pool,
		config: config,
		metrics: &PoolMetrics{
			lastAdjustment: time.Now(),
		},
	}

	// Iniciar monitoramento em background
	go ap.startMonitoring()

	return ap, nil
}

// calculateInitialConfig calcula configuração baseada no número de emuladores
func calculateInitialConfig(emulatorCount int) *AdaptiveConfig {
	// Fórmula simples: cada emulador pode usar no máximo 2 conexões simultâneas
	// + algumas conexões para operações administrativas
	estimatedPeak := int32(emulatorCount*2 + 5)

	return &AdaptiveConfig{
		MinConns:           2,                       // Sempre manter pelo menos 2
		BaseMaxConns:       max(5, estimatedPeak/2), // Começar conservador
		MaxConns:           estimatedPeak,
		AbsoluteMaxConns:   estimatedPeak * 2, // Buffer de segurança
		AdjustmentInterval: 2 * time.Minute,
		ShrinkThreshold:    0.3, // Se usar menos de 30%, reduzir
		GrowthThreshold:    0.8, // Se usar mais de 80%, aumentar
	}
}

// Implementar interface DBInterface
func (ap *AdaptivePool) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	start := time.Now()
	rows, err := ap.pool.Query(ctx, query, args...)
	ap.recordQuery(time.Since(start))
	return rows, err
}

func (ap *AdaptivePool) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	start := time.Now()
	row := ap.pool.QueryRow(ctx, query, args...)
	ap.recordQuery(time.Since(start))
	return row
}

func (ap *AdaptivePool) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	start := time.Now()
	tag, err := ap.pool.Exec(ctx, query, args...)
	ap.recordQuery(time.Since(start))
	return tag, err
}

func (ap *AdaptivePool) Begin(ctx context.Context) (pgx.Tx, error) {
	return ap.pool.Begin(ctx)
}

func (ap *AdaptivePool) Ping(ctx context.Context) error {
	return ap.pool.Ping(ctx)
}

func (ap *AdaptivePool) Close() {
	ap.pool.Close()
}

// recordQuery registra métricas de uma query
func (ap *AdaptivePool) recordQuery(duration time.Duration) {
	atomic.AddInt64(&ap.metrics.queriesPerSecond, 1)

	// Atualizar duração média (aproximação simples)
	durationMs := duration.Milliseconds()
	atomic.StoreInt64(&ap.metrics.avgQueryDuration, durationMs)

	// Registrar pico de conexões
	stats := ap.pool.Stat()
	currentConns := int32(stats.AcquiredConns())
	if currentConns > atomic.LoadInt32(&ap.metrics.peakConnections) {
		atomic.StoreInt32(&ap.metrics.peakConnections, currentConns)
	}
}

// startMonitoring inicia o monitoramento e ajuste automático
func (ap *AdaptivePool) startMonitoring() {
	ticker := time.NewTicker(ap.config.AdjustmentInterval)
	defer ticker.Stop()

	for range ticker.C {
		ap.adjustPoolSize()
		ap.resetMetrics()
	}
}

// adjustPoolSize ajusta o tamanho do pool baseado nas métricas
func (ap *AdaptivePool) adjustPoolSize() {
	ap.mu.Lock()
	defer ap.mu.Unlock()

	stats := ap.pool.Stat()
	currentMax := int32(stats.MaxConns())
	peakUsed := atomic.LoadInt32(&ap.metrics.peakConnections)

	if peakUsed == 0 {
		return // Sem dados suficientes
	}

	// Calcular utilização como % do máximo atual
	utilization := float64(peakUsed) / float64(currentMax)

	var newMax int32

	switch {
	case utilization > ap.config.GrowthThreshold && currentMax < ap.config.AbsoluteMaxConns:
		// Crescer: aumentar em 50% ou até o pico + buffer
		growth := max(currentMax+currentMax/2, peakUsed+2)
		newMax = min(growth, ap.config.AbsoluteMaxConns)

	case utilization < ap.config.ShrinkThreshold && currentMax > ap.config.MinConns:
		// Encolher: reduzir para pico + pequeno buffer
		newMax = max(peakUsed+1, ap.config.MinConns)

	default:
		return // Não precisa ajustar
	}

	// Aplicar mudança se significativa (diferença de pelo menos 2 conexões)
	if abs(newMax-currentMax) >= 2 {
		fmt.Printf("🔄 Adjusting pool: %d -> %d connections (utilization: %.1f%%, peak: %d)\n",
			currentMax, newMax, utilization*100, peakUsed)

		// Recriar pool com nova configuração
		ap.recreatePoolWithNewLimits(newMax)
	}
}

func min(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

func max(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

func abs(a int32) int32 {
	if a < 0 {
		return -a
	}
	return a
}

// recreatePoolWithNewLimits recria o pool com novos limites
func (ap *AdaptivePool) recreatePoolWithNewLimits(newMax int32) {
	// Obter configuração atual
	oldConfig := ap.pool.Config()

	// Criar nova configuração
	newConfig := oldConfig.Copy()
	newConfig.MaxConns = newMax

	// Criar novo pool
	newPool, err := pgxpool.ConnectConfig(context.Background(), newConfig)
	if err != nil {
		fmt.Printf("Failed to recreate pool: %v\n", err)
		return
	}

	// Substituir pool antigo
	oldPool := ap.pool
	ap.pool = newPool

	// Fechar pool antigo após um delay (permitir queries em andamento)
	go func() {
		time.Sleep(5 * time.Second)
		oldPool.Close()
	}()
}

// resetMetrics reseta métricas para próximo período
func (ap *AdaptivePool) resetMetrics() {
	atomic.StoreInt64(&ap.metrics.queriesPerSecond, 0)
	atomic.StoreInt32(&ap.metrics.peakConnections, 0)
	ap.metrics.lastAdjustment = time.Now()
}

// GetStats retorna estatísticas atuais do pool
func (ap *AdaptivePool) GetStats() map[string]interface{} {
	stats := ap.pool.Stat()
	return map[string]interface{}{
		"max_conns":             stats.MaxConns(),
		"acquired_conns":        stats.AcquiredConns(),
		"idle_conns":            stats.IdleConns(),
		"constructing_conns":    stats.ConstructingConns(),
		"peak_connections":      atomic.LoadInt32(&ap.metrics.peakConnections),
		"avg_query_duration_ms": atomic.LoadInt64(&ap.metrics.avgQueryDuration),
	}
}
