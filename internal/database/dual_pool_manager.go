package database

import (
	"context"
	"fmt"
	"time"

	"GoFacialEmulator/internal/trace"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
)

// DualPoolManager gerencia pools separados para reads e writes
type DualPoolManager struct {
	readPool   *AdaptivePool
	writePool  *AdaptivePool
	writeQueue *OrderedWriteQueue

	config *DynamicPoolConfig
	tracer *trace.Tracer

	// Contexto do emulador atual (thread-local simulation)
	// Em Go usamos context.Context para passar emulatorID
}

// NewDualPoolManager cria um gerenciador com pools separados
func NewDualPoolManager(dbURL string, emulatorCount int, tracer *trace.Tracer) (*DualPoolManager, error) {
	// Calcular configuração dinâmica
	config := CalculateDynamicConfig(emulatorCount)

	tracer.Info("[DualPoolManager] Calculated configuration for %d emulators:", emulatorCount)
	tracer.Info(config.String())
	tracer.Info("[DualPoolManager] Recommended PostgreSQL max_connections: %d",
		config.GetRecommendedPostgresMaxConnections())

	// Criar Read Pool (grande, para muitas queries simultâneas)
	readPool, err := createPoolWithConfig(dbURL, config, true, tracer)
	if err != nil {
		return nil, fmt.Errorf("failed to create read pool: %w", err)
	}

	// Criar Write Pool (pequeno, usado apenas pela fila)
	writePool, err := createPoolWithConfig(dbURL, config, false, tracer)
	if err != nil {
		readPool.Close()
		return nil, fmt.Errorf("failed to create write pool: %w", err)
	}

	// Criar Write Queue
	writeQueue := NewOrderedWriteQueue(writePool, config, tracer)

	dpm := &DualPoolManager{
		readPool:   readPool,
		writePool:  writePool,
		writeQueue: writeQueue,
		config:     config,
		tracer:     tracer,
	}

	tracer.Info("[DualPoolManager] Initialization complete")
	return dpm, nil
}

// createPoolWithConfig cria um pool com configuração específica
func createPoolWithConfig(dbURL string, config *DynamicPoolConfig, isReadPool bool, tracer *trace.Tracer) (*AdaptivePool, error) {
	var poolType string
	var adaptiveConfig *AdaptiveConfig

	if isReadPool {
		poolType = "READ"
		adaptiveConfig = &AdaptiveConfig{
			MinConns:           config.ReadPoolMin,
			BaseMaxConns:       config.ReadPoolInitial,
			MaxConns:           config.ReadPoolMax,
			AbsoluteMaxConns:   config.ReadPoolMax,
			AdjustmentInterval: 2 * time.Minute,
			ShrinkThreshold:    0.3,
			GrowthThreshold:    0.85,
			ExpansionStep:      10,
		}
	} else {
		poolType = "WRITE"
		adaptiveConfig = &AdaptiveConfig{
			MinConns:           config.WritePoolMin,
			BaseMaxConns:       config.WritePoolMax,
			MaxConns:           config.WritePoolMax,
			AbsoluteMaxConns:   config.WritePoolMax,
			AdjustmentInterval: 5 * time.Minute,
			ShrinkThreshold:    0.2,
			GrowthThreshold:    0.9,
			ExpansionStep:      2,
		}
	}

	tracer.Info("[DualPoolManager] Creating %s pool (min: %d, initial: %d, max: %d)",
		poolType, adaptiveConfig.MinConns, adaptiveConfig.BaseMaxConns, adaptiveConfig.MaxConns)

	pool, err := NewAdaptivePoolWithConfig(dbURL, adaptiveConfig)
	if err != nil {
		return nil, err
	}

	return pool, nil
}

// ============ DBInterface Implementation ============

// Query executa query de leitura através do Read Pool
func (dpm *DualPoolManager) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	return dpm.readPool.Query(ctx, query, args...)
}

// QueryRow executa query de leitura de linha única através do Read Pool
func (dpm *DualPoolManager) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	return dpm.readPool.QueryRow(ctx, query, args...)
}

// Exec executa operação de escrita através da Write Queue
func (dpm *DualPoolManager) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	// Extrair emulatorID do contexto
	emulatorID := GetEmulatorIDFromContext(ctx)
	if emulatorID == 0 {
		// Se não houver emulatorID, executar diretamente (operações administrativas)
		return dpm.writePool.Exec(ctx, query, args...)
	}

	// Executar através da fila ordenada
	return dpm.writeQueue.Exec(ctx, emulatorID, query, args...)
}

// Begin inicia transação no Write Pool
func (dpm *DualPoolManager) Begin(ctx context.Context) (pgx.Tx, error) {
	// Transações precisam de controle direto, não podem usar fila
	// Usar write pool diretamente
	return dpm.writePool.Begin(ctx)
}

// Ping verifica saúde de ambos os pools
func (dpm *DualPoolManager) Ping(ctx context.Context) error {
	if err := dpm.readPool.Ping(ctx); err != nil {
		return fmt.Errorf("read pool ping failed: %w", err)
	}
	if err := dpm.writePool.Ping(ctx); err != nil {
		return fmt.Errorf("write pool ping failed: %w", err)
	}
	return nil
}

// Close fecha ambos os pools e a fila
func (dpm *DualPoolManager) Close() {
	dpm.tracer.Info("[DualPoolManager] Shutting down...")

	// Parar write queue primeiro (flush pendentes)
	dpm.writeQueue.Stop()

	// Fechar pools
	dpm.writePool.Close()
	dpm.readPool.Close()

	dpm.tracer.Info("[DualPoolManager] Shutdown complete")
}

// ============ Métodos Específicos ============

// ExecDirect executa escrita diretamente no pool (bypass da fila)
// Usado para operações administrativas que não precisam de ordenação
func (dpm *DualPoolManager) ExecDirect(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	return dpm.writePool.Exec(ctx, query, args...)
}

// GetStats retorna estatísticas de ambos os pools e da fila
func (dpm *DualPoolManager) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"read_pool":   dpm.readPool.GetStats(),
		"write_pool":  dpm.writePool.GetStats(),
		"write_queue": dpm.writeQueue.GetMetrics(),
		"config":      dpm.getConfigSummary(),
		"timestamp":   time.Now(),
	}
}

// getConfigSummary retorna resumo da configuração
func (dpm *DualPoolManager) getConfigSummary() map[string]interface{} {
	return map[string]interface{}{
		"emulator_count":       dpm.config.EmulatorCount,
		"read_pool_max":        dpm.config.ReadPoolMax,
		"write_pool_max":       dpm.config.WritePoolMax,
		"write_workers":        dpm.config.WriteWorkers,
		"write_queue_size":     dpm.config.WriteQueueSize,
		"write_batch_size":     dpm.config.WriteBatchSize,
		"query_timeout":        dpm.config.QueryTimeout.String(),
		"scaling_factors":      dpm.config.GetScalingFactors(),
	}
}

// ============ Context Helpers ============

type contextKey int

const emulatorIDKey contextKey = 1

// WithEmulatorID adiciona emulatorID ao contexto
func WithEmulatorID(ctx context.Context, emulatorID int) context.Context {
	return context.WithValue(ctx, emulatorIDKey, emulatorID)
}

// GetEmulatorIDFromContext extrai emulatorID do contexto
func GetEmulatorIDFromContext(ctx context.Context) int {
	if id, ok := ctx.Value(emulatorIDKey).(int); ok {
		return id
	}
	return 0
}

// ============ Adaptive Pool Helper ============

// NewAdaptivePoolWithConfig cria AdaptivePool com configuração customizada
// Esta função usa NewAdaptivePoolWithCustomConfig do adaptive_pool.go
func NewAdaptivePoolWithConfig(dbURL string, config *AdaptiveConfig) (*AdaptivePool, error) {
	return NewAdaptivePoolWithCustomConfig(dbURL, config)
}
