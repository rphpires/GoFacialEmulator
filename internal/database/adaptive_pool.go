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
	pool      *pgxpool.Pool
	metrics   *PoolMetrics
	config    *AdaptiveConfig
	mu        sync.RWMutex
	expanding int32 // Flag atômica para evitar expansões simultâneas
}

// PoolMetrics coleta estatísticas de uso
type PoolMetrics struct {
	queriesPerSecond int64
	avgQueryDuration int64 // em milliseconds
	peakConnections  int32
	lastAdjustment   time.Time
	connectionsPeak  int32
	expansions       int32 // Contador de expansões instantâneas
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
	ExpansionStep      int32   // Quantas conexões adicionar na expansão instantânea
}

// NewAdaptivePool cria um pool que se adapta à carga
func NewAdaptivePool(dbURL string, emulatorCount int) (*AdaptivePool, error) {
	// Calcular configuração inicial baseada no número de emuladores
	config := calculateInitialConfig(emulatorCount)
	return NewAdaptivePoolWithCustomConfig(dbURL, config)
}

// NewAdaptivePoolWithCustomConfig cria pool com configuração customizada
func NewAdaptivePoolWithCustomConfig(dbURL string, config *AdaptiveConfig) (*AdaptivePool, error) {
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
		MinConns:           5,                       // Manter mais conexões mínimas
		BaseMaxConns:       estimatedPeak,           // Iniciar com capacidade total para evitar contenção
		MaxConns:           estimatedPeak * 2,       // Permitir expansão se necessário
		AbsoluteMaxConns:   estimatedPeak * 3,       // Mais margem para picos
		AdjustmentInterval: 2 * time.Minute,
		ShrinkThreshold:    0.3,  // Se usar menos de 30%, reduzir
		GrowthThreshold:    0.85, // Se usar mais de 85%, aumentar
		ExpansionStep:      10,   // Expandir mais rápido (10 em vez de 3)
	}
}

// checkAndExpandIfNeeded verifica se precisa expandir instantaneamente
func (ap *AdaptivePool) checkAndExpandIfNeeded() {
	// Evitar expansões simultâneas
	if !atomic.CompareAndSwapInt32(&ap.expanding, 0, 1) {
		return
	}
	defer atomic.StoreInt32(&ap.expanding, 0)

	stats := ap.pool.Stat()
	currentMax := int32(stats.MaxConns())
	acquired := int32(stats.AcquiredConns())
	constructing := int32(stats.ConstructingConns())

	// Se quase todas as conexões estão em uso e ainda temos espaço para crescer
	totalInUse := acquired + constructing
	utilization := float64(totalInUse) / float64(currentMax)

	if utilization > 0.9 && currentMax < ap.config.AbsoluteMaxConns {
		newMax := min(currentMax+ap.config.ExpansionStep, ap.config.AbsoluteMaxConns)

		fmt.Printf("🚀 INSTANT EXPANSION: %d -> %d connections (utilization: %.1f%%, acquired: %d)\n",
			currentMax, newMax, utilization*100, acquired)

		// Expandir imediatamente
		go ap.recreatePoolWithNewLimits(newMax)
		atomic.AddInt32(&ap.metrics.expansions, 1)
	}
}

// Implementar interface DBInterface com verificação de expansão
func (ap *AdaptivePool) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	// Verificar se precisa expandir antes da query
	ap.checkAndExpandIfNeeded()

	start := time.Now()
	rows, err := ap.pool.Query(ctx, query, args...)
	ap.recordQuery(time.Since(start))
	return rows, err
}

func (ap *AdaptivePool) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	// Verificar se precisa expandir antes da query
	ap.checkAndExpandIfNeeded()

	start := time.Now()
	row := ap.pool.QueryRow(ctx, query, args...)
	ap.recordQuery(time.Since(start))
	return row
}

func (ap *AdaptivePool) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	// Verificar se precisa expandir antes da execução
	ap.checkAndExpandIfNeeded()

	start := time.Now()
	tag, err := ap.pool.Exec(ctx, query, args...)
	ap.recordQuery(time.Since(start))
	return tag, err
}

func (ap *AdaptivePool) Begin(ctx context.Context) (pgx.Tx, error) {
	// Verificar se precisa expandir antes de iniciar transação
	ap.checkAndExpandIfNeeded()

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

// startMonitoring inicia o monitoramento e ajuste automático (para encolhimento)
func (ap *AdaptivePool) startMonitoring() {
	ticker := time.NewTicker(ap.config.AdjustmentInterval)
	defer ticker.Stop()

	for range ticker.C {
		ap.adjustPoolSizeDown() // Apenas para reduzir
		ap.resetMetrics()
	}
}

// adjustPoolSizeDown ajusta o pool para baixo baseado nas métricas
func (ap *AdaptivePool) adjustPoolSizeDown() {
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

	// Apenas encolher se utilização baixa e acima do mínimo
	if utilization < ap.config.ShrinkThreshold && currentMax > ap.config.MinConns {
		// Encolher: reduzir para pico + pequeno buffer, mas não menos que o mínimo
		newMax := max(peakUsed+2, ap.config.MinConns)

		// Aplicar mudança se significativa (diferença de pelo menos 2 conexões)
		if currentMax-newMax >= 2 {
			fmt.Printf("📉 SCHEDULED SHRINK: %d -> %d connections (utilization: %.1f%%, peak: %d)\n",
				currentMax, newMax, utilization*100, peakUsed)

			ap.recreatePoolWithNewLimits(newMax)
		}
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

// recreatePoolWithNewLimits recria o pool com novos limites (sem mutex, já protegido pelos callers)
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
		"instant_expansions":    atomic.LoadInt32(&ap.metrics.expansions),
	}
}
