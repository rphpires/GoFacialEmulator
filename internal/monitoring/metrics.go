package monitoring

import (
	"sync"
	"sync/atomic"
	"time"
)

// Metrics armazena métricas da aplicação
type Metrics struct {
	// HTTP Requests
	totalRequests       atomic.Int64
	totalErrors         atomic.Int64
	total4xx            atomic.Int64
	total5xx            atomic.Int64
	requestDurations    []time.Duration
	durationsMutex      sync.RWMutex

	// Endpoint específico
	endpointMetrics map[string]*EndpointMetrics
	endpointMutex   sync.RWMutex

	// Emuladores
	totalStartAttempts  atomic.Int64
	totalStartFailures  atomic.Int64
	totalStopAttempts   atomic.Int64
	totalStopFailures   atomic.Int64

	// Database
	totalDBQueries      atomic.Int64
	totalDBErrors       atomic.Int64
	dbQueryDurations    []time.Duration
	dbDurationsMutex    sync.RWMutex

	// System
	startTime time.Time
}

// EndpointMetrics métricas por endpoint
type EndpointMetrics struct {
	Path          string
	TotalRequests atomic.Int64
	TotalErrors   atomic.Int64
	Total4xx      atomic.Int64
	Total5xx      atomic.Int64
	AvgDuration   atomic.Value // float64
	LastError     atomic.Value // string
	LastErrorTime atomic.Value // time.Time
}

// NewMetrics cria uma nova instância de métricas
func NewMetrics() *Metrics {
	m := &Metrics{
		requestDurations: make([]time.Duration, 0, 1000),
		dbQueryDurations: make([]time.Duration, 0, 1000),
		endpointMetrics:  make(map[string]*EndpointMetrics),
		startTime:        time.Now(),
	}
	return m
}

// RecordRequest registra uma requisição HTTP
func (m *Metrics) RecordRequest(path string, statusCode int, duration time.Duration, err error) {
	m.totalRequests.Add(1)

	// Classificar por status code
	if statusCode >= 500 {
		m.total5xx.Add(1)
		m.totalErrors.Add(1)
	} else if statusCode >= 400 {
		m.total4xx.Add(1)
	}

	// Registrar duração
	m.durationsMutex.Lock()
	if len(m.requestDurations) < 1000 {
		m.requestDurations = append(m.requestDurations, duration)
	} else {
		// Manter apenas últimas 1000 (sliding window)
		m.requestDurations = append(m.requestDurations[1:], duration)
	}
	m.durationsMutex.Unlock()

	// Métricas por endpoint
	m.recordEndpointMetrics(path, statusCode, duration, err)
}

// recordEndpointMetrics registra métricas para um endpoint específico
func (m *Metrics) recordEndpointMetrics(path string, statusCode int, duration time.Duration, err error) {
	m.endpointMutex.Lock()
	em, exists := m.endpointMetrics[path]
	if !exists {
		em = &EndpointMetrics{Path: path}
		em.AvgDuration.Store(0.0)
		em.LastError.Store("")
		em.LastErrorTime.Store(time.Time{})
		m.endpointMetrics[path] = em
	}
	m.endpointMutex.Unlock()

	em.TotalRequests.Add(1)

	if statusCode >= 500 {
		em.Total5xx.Add(1)
		em.TotalErrors.Add(1)
		if err != nil {
			em.LastError.Store(err.Error())
			em.LastErrorTime.Store(time.Now())
		}
	} else if statusCode >= 400 {
		em.Total4xx.Add(1)
	}

	// Atualizar média móvel de duração
	currentAvg := em.AvgDuration.Load().(float64)
	count := float64(em.TotalRequests.Load())
	newAvg := (currentAvg*(count-1) + float64(duration.Milliseconds())) / count
	em.AvgDuration.Store(newAvg)
}

// RecordEmulatorStart registra tentativa de início de emulador
func (m *Metrics) RecordEmulatorStart(success bool) {
	m.totalStartAttempts.Add(1)
	if !success {
		m.totalStartFailures.Add(1)
	}
}

// RecordEmulatorStop registra tentativa de parada de emulador
func (m *Metrics) RecordEmulatorStop(success bool) {
	m.totalStopAttempts.Add(1)
	if !success {
		m.totalStopFailures.Add(1)
	}
}

// RecordDBQuery registra uma query de banco de dados
func (m *Metrics) RecordDBQuery(duration time.Duration, err error) {
	m.totalDBQueries.Add(1)
	if err != nil {
		m.totalDBErrors.Add(1)
	}

	m.dbDurationsMutex.Lock()
	if len(m.dbQueryDurations) < 1000 {
		m.dbQueryDurations = append(m.dbQueryDurations, duration)
	} else {
		m.dbQueryDurations = append(m.dbQueryDurations[1:], duration)
	}
	m.dbDurationsMutex.Unlock()
}

// GetSnapshot retorna um snapshot das métricas
func (m *Metrics) GetSnapshot() map[string]interface{} {
	// Calcular percentis de duração HTTP
	m.durationsMutex.RLock()
	httpP50, httpP95, httpP99 := calculatePercentiles(m.requestDurations)
	avgHTTPDuration := calculateAverage(m.requestDurations)
	m.durationsMutex.RUnlock()

	// Calcular percentis de duração DB
	m.dbDurationsMutex.RLock()
	dbP50, dbP95, dbP99 := calculatePercentiles(m.dbQueryDurations)
	avgDBDuration := calculateAverage(m.dbQueryDurations)
	m.dbDurationsMutex.RUnlock()

	uptime := time.Since(m.startTime)

	return map[string]interface{}{
		"uptime_seconds": uptime.Seconds(),
		"http": map[string]interface{}{
			"total_requests":      m.totalRequests.Load(),
			"total_errors":        m.totalErrors.Load(),
			"total_4xx":           m.total4xx.Load(),
			"total_5xx":           m.total5xx.Load(),
			"error_rate":          m.calculateErrorRate(),
			"avg_duration_ms":     avgHTTPDuration,
			"p50_duration_ms":     httpP50,
			"p95_duration_ms":     httpP95,
			"p99_duration_ms":     httpP99,
			"requests_per_second": m.calculateRPS(uptime),
		},
		"emulators": map[string]interface{}{
			"total_start_attempts": m.totalStartAttempts.Load(),
			"total_start_failures": m.totalStartFailures.Load(),
			"total_stop_attempts":  m.totalStopAttempts.Load(),
			"total_stop_failures":  m.totalStopFailures.Load(),
			"start_success_rate":   m.calculateStartSuccessRate(),
		},
		"database": map[string]interface{}{
			"total_queries":   m.totalDBQueries.Load(),
			"total_errors":    m.totalDBErrors.Load(),
			"error_rate":      m.calculateDBErrorRate(),
			"avg_duration_ms": avgDBDuration,
			"p50_duration_ms": dbP50,
			"p95_duration_ms": dbP95,
			"p99_duration_ms": dbP99,
		},
		"endpoints": m.getEndpointSnapshot(),
	}
}

// getEndpointSnapshot retorna snapshot das métricas por endpoint
func (m *Metrics) getEndpointSnapshot() map[string]interface{} {
	m.endpointMutex.RLock()
	defer m.endpointMutex.RUnlock()

	endpoints := make(map[string]interface{})
	for path, em := range m.endpointMetrics {
		lastErr := em.LastError.Load().(string)
		lastErrTime := em.LastErrorTime.Load().(time.Time)

		endpoints[path] = map[string]interface{}{
			"total_requests": em.TotalRequests.Load(),
			"total_errors":   em.TotalErrors.Load(),
			"total_4xx":      em.Total4xx.Load(),
			"total_5xx":      em.Total5xx.Load(),
			"avg_duration_ms": em.AvgDuration.Load().(float64),
			"last_error":     lastErr,
			"last_error_time": lastErrTime,
		}
	}

	return endpoints
}

// calculateErrorRate calcula taxa de erro HTTP
func (m *Metrics) calculateErrorRate() float64 {
	total := m.totalRequests.Load()
	if total == 0 {
		return 0
	}
	return float64(m.totalErrors.Load()) / float64(total) * 100
}

// calculateDBErrorRate calcula taxa de erro DB
func (m *Metrics) calculateDBErrorRate() float64 {
	total := m.totalDBQueries.Load()
	if total == 0 {
		return 0
	}
	return float64(m.totalDBErrors.Load()) / float64(total) * 100
}

// calculateStartSuccessRate calcula taxa de sucesso de start
func (m *Metrics) calculateStartSuccessRate() float64 {
	total := m.totalStartAttempts.Load()
	if total == 0 {
		return 100
	}
	successes := total - m.totalStartFailures.Load()
	return float64(successes) / float64(total) * 100
}

// calculateRPS calcula requests por segundo
func (m *Metrics) calculateRPS(uptime time.Duration) float64 {
	if uptime.Seconds() == 0 {
		return 0
	}
	return float64(m.totalRequests.Load()) / uptime.Seconds()
}

// GetTopErrorEndpoints retorna endpoints com mais erros
func (m *Metrics) GetTopErrorEndpoints(limit int) []map[string]interface{} {
	m.endpointMutex.RLock()
	defer m.endpointMutex.RUnlock()

	type endpointError struct {
		path   string
		errors int64
		em     *EndpointMetrics
	}

	errors := make([]endpointError, 0, len(m.endpointMetrics))
	for path, em := range m.endpointMetrics {
		totalErrs := em.Total5xx.Load()
		if totalErrs > 0 {
			errors = append(errors, endpointError{
				path:   path,
				errors: totalErrs,
				em:     em,
			})
		}
	}

	// Ordenar por quantidade de erros (bubble sort simples)
	for i := 0; i < len(errors); i++ {
		for j := i + 1; j < len(errors); j++ {
			if errors[j].errors > errors[i].errors {
				errors[i], errors[j] = errors[j], errors[i]
			}
		}
	}

	// Limitar resultado
	if len(errors) > limit {
		errors = errors[:limit]
	}

	result := make([]map[string]interface{}, len(errors))
	for i, e := range errors {
		result[i] = map[string]interface{}{
			"path":           e.path,
			"total_5xx":      e.errors,
			"total_requests": e.em.TotalRequests.Load(),
			"last_error":     e.em.LastError.Load().(string),
			"last_error_time": e.em.LastErrorTime.Load().(time.Time),
		}
	}

	return result
}

// Funções auxiliares para cálculos estatísticos

func calculatePercentiles(durations []time.Duration) (p50, p95, p99 float64) {
	if len(durations) == 0 {
		return 0, 0, 0
	}

	// Criar cópia e ordenar
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)

	// Bubble sort (suficiente para 1000 elementos)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] < sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	p50 = float64(sorted[len(sorted)*50/100].Milliseconds())
	p95 = float64(sorted[len(sorted)*95/100].Milliseconds())
	p99 = float64(sorted[len(sorted)*99/100].Milliseconds())

	return
}

func calculateAverage(durations []time.Duration) float64 {
	if len(durations) == 0 {
		return 0
	}

	var total int64
	for _, d := range durations {
		total += d.Milliseconds()
	}

	return float64(total) / float64(len(durations))
}
