package database

import (
	"fmt"
	"math"
	"time"
)

// DynamicPoolConfig calcula configurações otimizadas baseadas no número de emuladores
type DynamicPoolConfig struct {
	EmulatorCount int

	// Read Pool Configuration
	ReadPoolMin     int32
	ReadPoolMax     int32
	ReadPoolInitial int32

	// Write Pool Configuration
	WritePoolMin     int32
	WritePoolMax     int32
	WriteWorkers     int
	WriteQueueSize   int
	WriteBatchSize   int
	WriteFlushMs     int

	// Connection Settings
	MaxConnLifetime  time.Duration
	MaxConnIdleTime  time.Duration
	HealthCheckPeriod time.Duration

	// Timeouts
	QueryTimeout     time.Duration
	TransactionTimeout time.Duration
}

// CalculateDynamicConfig calcula a melhor configuração baseada no número de emuladores
func CalculateDynamicConfig(emulatorCount int) *DynamicPoolConfig {
	if emulatorCount <= 0 {
		emulatorCount = 10 // Mínimo razoável
	}

	config := &DynamicPoolConfig{
		EmulatorCount: emulatorCount,
	}

	// ============ READ POOL ============
	// Fórmula: cada emulador pode fazer ~2-3 reads simultâneos durante sync
	// + buffer de 20% para operações administrativas e watchdog
	estimatedReadConcurrency := float64(emulatorCount) * 2.5
	readPoolEstimate := int32(math.Ceil(estimatedReadConcurrency * 1.2))

	config.ReadPoolMin = maxInt32(5, readPoolEstimate/10)           // 10% como mínimo
	config.ReadPoolInitial = maxInt32(10, readPoolEstimate/2)       // Começar com 50%
	config.ReadPoolMax = maxInt32(20, readPoolEstimate)             // Máximo calculado

	// ============ WRITE POOL ============
	// Write pool é MUITO menor porque usa fila centralizada
	// Fórmula: sqrt(emulators) workers, mas limitado entre 5 e 50
	writersFloat := math.Sqrt(float64(emulatorCount))
	config.WriteWorkers = clampInt(int(math.Ceil(writersFloat)), 5, 50)

	// Conexões = workers + buffer de 20%
	config.WritePoolMin = int32(config.WriteWorkers)
	config.WritePoolMax = int32(math.Ceil(float64(config.WriteWorkers) * 1.2))

	// ============ WRITE QUEUE ============
	// Fórmula: cada emulador pode ter ~5 operações pendentes durante sync massivo
	// Queue = emulators * 5, limitado entre 1000 e 50000
	queueSizeFloat := float64(emulatorCount) * 5
	config.WriteQueueSize = clampInt(int(queueSizeFloat), 1000, 50000)

	// Batch size cresce logaritmicamente com número de emuladores
	// Fórmula: 10 * log10(emulators), limitado entre 10 e 500
	batchSizeFloat := 10 * math.Log10(float64(emulatorCount))
	config.WriteBatchSize = clampInt(int(math.Ceil(batchSizeFloat)), 10, 500)

	// Flush interval decresce com mais emuladores (mais atividade = flush mais rápido)
	// Fórmula: 5000ms / log10(emulators), limitado entre 100ms e 5000ms
	flushMsFloat := 5000 / math.Max(1, math.Log10(float64(emulatorCount)))
	config.WriteFlushMs = clampInt(int(math.Ceil(flushMsFloat)), 100, 5000)

	// ============ CONNECTION SETTINGS ============
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 10 * time.Minute
	config.HealthCheckPeriod = 1 * time.Minute

	// ============ TIMEOUTS ============
	// Timeouts crescem com número de emuladores (mais contenção = precisa mais tempo)
	baseQueryTimeout := 5.0 // segundos
	timeoutMultiplier := 1.0 + (math.Log10(float64(emulatorCount)) / 10)
	queryTimeoutSeconds := baseQueryTimeout * timeoutMultiplier

	config.QueryTimeout = time.Duration(queryTimeoutSeconds * float64(time.Second))
	config.TransactionTimeout = config.QueryTimeout * 2 // Transações podem ser mais longas

	return config
}

// String retorna uma representação legível da configuração
func (c *DynamicPoolConfig) String() string {
	totalConns := c.ReadPoolMax + c.WritePoolMax
	avgQueueUtilization := float64(c.WriteQueueSize) / float64(c.EmulatorCount)

	return fmt.Sprintf(`
╔════════════════════════════════════════════════════════════════════╗
║           DYNAMIC DATABASE POOL CONFIGURATION                      ║
╠════════════════════════════════════════════════════════════════════╣
║ Target Emulators: %-4d                                             ║
║                                                                    ║
║ READ POOL:                                                         ║
║   - Min Connections:     %-4d                                      ║
║   - Initial Connections: %-4d                                      ║
║   - Max Connections:     %-4d                                      ║
║                                                                    ║
║ WRITE POOL:                                                        ║
║   - Min Connections:     %-4d                                      ║
║   - Max Connections:     %-4d                                      ║
║   - Workers:             %-4d                                      ║
║   - Queue Size:          %-6d ops                                  ║
║   - Batch Size:          %-4d ops/batch                            ║
║   - Flush Interval:      %-4d ms                                   ║
║                                                                    ║
║ TOTALS:                                                            ║
║   - Total Max Conns:     %-4d                                      ║
║   - Queue/Emulator:      %.1f ops/emulator                         ║
║                                                                    ║
║ TIMEOUTS:                                                          ║
║   - Query Timeout:       %v                                        ║
║   - Transaction Timeout: %v                                        ║
╚════════════════════════════════════════════════════════════════════╝`,
		c.EmulatorCount,
		c.ReadPoolMin, c.ReadPoolInitial, c.ReadPoolMax,
		c.WritePoolMin, c.WritePoolMax, c.WriteWorkers,
		c.WriteQueueSize, c.WriteBatchSize, c.WriteFlushMs,
		totalConns, avgQueueUtilization,
		c.QueryTimeout, c.TransactionTimeout,
	)
}

// GetRecommendedPostgresMaxConnections retorna o max_connections recomendado para o PostgreSQL
func (c *DynamicPoolConfig) GetRecommendedPostgresMaxConnections() int {
	// Total de conexões + 20% de buffer para admin/superuser + outras aplicações
	totalAppConns := c.ReadPoolMax + c.WritePoolMax
	recommended := int(math.Ceil(float64(totalAppConns) * 1.3))

	// PostgreSQL default é 100, nunca recomendar menos que isso
	if recommended < 100 {
		recommended = 100
	}

	return recommended
}

// Utility functions
func maxInt32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// GetScalingFactors retorna fatores de escala para análise
func (c *DynamicPoolConfig) GetScalingFactors() map[string]float64 {
	return map[string]float64{
		"read_pool_per_emulator":    float64(c.ReadPoolMax) / float64(c.EmulatorCount),
		"write_workers_per_100_emu": (float64(c.WriteWorkers) / float64(c.EmulatorCount)) * 100,
		"queue_depth_per_emulator":  float64(c.WriteQueueSize) / float64(c.EmulatorCount),
		"total_conns_per_emulator":  float64(c.ReadPoolMax+c.WritePoolMax) / float64(c.EmulatorCount),
	}
}
