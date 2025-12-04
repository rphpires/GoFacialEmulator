package utils

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// CircuitState representa o estado do circuit breaker
type CircuitState int32

const (
	StateClosed   CircuitState = 0 // Normal operation
	StateOpen     CircuitState = 1 // Failing, reject requests
	StateHalfOpen CircuitState = 2 // Testing if recovered
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

// CircuitBreaker implementa o pattern Circuit Breaker
type CircuitBreaker struct {
	name string

	// Estado atual
	state atomic.Int32 // CircuitState

	// Contadores
	consecutiveFailures atomic.Int32
	consecutiveSuccesses atomic.Int32
	totalRequests       atomic.Int64
	totalFailures       atomic.Int64
	totalSuccesses      atomic.Int64

	// Configuração
	maxFailures       int32         // Falhas consecutivas para abrir circuito
	resetTimeout      time.Duration // Tempo para tentar HALF_OPEN
	halfOpenSuccesses int32         // Sucessos necessários em HALF_OPEN para fechar

	// Timestamp da última mudança de estado
	lastStateChange atomic.Int64

	// Mutex para mudanças de estado
	mu sync.RWMutex
}

// CircuitBreakerConfig configuração do circuit breaker
type CircuitBreakerConfig struct {
	Name              string
	MaxFailures       int32         // Default: 5
	ResetTimeout      time.Duration // Default: 60s
	HalfOpenSuccesses int32         // Default: 3
}

// NewCircuitBreaker cria um novo circuit breaker
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	if config.MaxFailures <= 0 {
		config.MaxFailures = 5
	}
	if config.ResetTimeout <= 0 {
		config.ResetTimeout = 60 * time.Second
	}
	if config.HalfOpenSuccesses <= 0 {
		config.HalfOpenSuccesses = 3
	}

	cb := &CircuitBreaker{
		name:              config.Name,
		maxFailures:       config.MaxFailures,
		resetTimeout:      config.ResetTimeout,
		halfOpenSuccesses: config.HalfOpenSuccesses,
	}

	cb.state.Store(int32(StateClosed))
	cb.lastStateChange.Store(time.Now().Unix())

	return cb
}

// Execute executa função com proteção do circuit breaker
func (cb *CircuitBreaker) Execute(fn func() error) error {
	// Verificar estado antes de executar
	if err := cb.beforeRequest(); err != nil {
		return err
	}

	// Executar função
	err := fn()

	// Processar resultado
	cb.afterRequest(err)

	return err
}

// beforeRequest verifica se request pode ser executado
func (cb *CircuitBreaker) beforeRequest() error {
	cb.totalRequests.Add(1)

	currentState := CircuitState(cb.state.Load())

	switch currentState {
	case StateClosed:
		// Normal operation, allow request
		return nil

	case StateOpen:
		// Check if we should transition to HALF_OPEN
		if cb.shouldAttemptReset() {
			cb.transitionToHalfOpen()
			return nil
		}
		// Circuit is open, reject request
		return &CircuitOpenError{
			Name:            cb.name,
			OpenedAt:        time.Unix(cb.lastStateChange.Load(), 0),
			NextRetryAfter:  cb.nextRetryTime(),
		}

	case StateHalfOpen:
		// Allow limited testing
		return nil

	default:
		return fmt.Errorf("unknown circuit state: %d", currentState)
	}
}

// afterRequest processa resultado do request
func (cb *CircuitBreaker) afterRequest(err error) {
	currentState := CircuitState(cb.state.Load())

	if err == nil {
		// Success
		cb.onSuccess(currentState)
	} else {
		// Failure
		cb.onFailure(currentState, err)
	}
}

// onSuccess processa sucesso
func (cb *CircuitBreaker) onSuccess(state CircuitState) {
	cb.totalSuccesses.Add(1)
	failures := cb.consecutiveFailures.Load()

	// Reset consecutive failures on any success
	if failures > 0 {
		cb.consecutiveFailures.Store(0)
	}

	switch state {
	case StateClosed:
		// Continue normal operation

	case StateHalfOpen:
		// Count successes in HALF_OPEN
		successes := cb.consecutiveSuccesses.Add(1)
		if successes >= cb.halfOpenSuccesses {
			cb.transitionToClosed()
		}

	case StateOpen:
		// Should not happen, but transition to HALF_OPEN if it does
		cb.transitionToHalfOpen()
	}
}

// onFailure processa falha
func (cb *CircuitBreaker) onFailure(state CircuitState, err error) {
	cb.totalFailures.Add(1)
	failures := cb.consecutiveFailures.Add(1)

	// Reset consecutive successes on any failure
	if cb.consecutiveSuccesses.Load() > 0 {
		cb.consecutiveSuccesses.Store(0)
	}

	switch state {
	case StateClosed:
		// Check if we should open circuit
		if failures >= cb.maxFailures {
			cb.transitionToOpen()
		}

	case StateHalfOpen:
		// Any failure in HALF_OPEN reopens circuit
		cb.transitionToOpen()

	case StateOpen:
		// Already open, just record failure
	}
}

// State transitions
func (cb *CircuitBreaker) transitionToOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state.Store(int32(StateOpen))
	cb.lastStateChange.Store(time.Now().Unix())
	cb.consecutiveSuccesses.Store(0)
}

func (cb *CircuitBreaker) transitionToHalfOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state.Store(int32(StateHalfOpen))
	cb.lastStateChange.Store(time.Now().Unix())
	cb.consecutiveSuccesses.Store(0)
	cb.consecutiveFailures.Store(0)
}

func (cb *CircuitBreaker) transitionToClosed() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state.Store(int32(StateClosed))
	cb.lastStateChange.Store(time.Now().Unix())
	cb.consecutiveFailures.Store(0)
	cb.consecutiveSuccesses.Store(0)
}

// shouldAttemptReset verifica se deve tentar resetar circuito
func (cb *CircuitBreaker) shouldAttemptReset() bool {
	lastChange := time.Unix(cb.lastStateChange.Load(), 0)
	return time.Since(lastChange) >= cb.resetTimeout
}

// nextRetryTime retorna quando próximo retry pode ser tentado
func (cb *CircuitBreaker) nextRetryTime() time.Time {
	lastChange := time.Unix(cb.lastStateChange.Load(), 0)
	return lastChange.Add(cb.resetTimeout)
}

// GetState retorna estado atual
func (cb *CircuitBreaker) GetState() CircuitState {
	return CircuitState(cb.state.Load())
}

// GetMetrics retorna métricas do circuit breaker
func (cb *CircuitBreaker) GetMetrics() CircuitBreakerMetrics {
	return CircuitBreakerMetrics{
		Name:                 cb.name,
		State:                cb.GetState(),
		TotalRequests:        cb.totalRequests.Load(),
		TotalSuccesses:       cb.totalSuccesses.Load(),
		TotalFailures:        cb.totalFailures.Load(),
		ConsecutiveFailures:  cb.consecutiveFailures.Load(),
		ConsecutiveSuccesses: cb.consecutiveSuccesses.Load(),
		LastStateChange:      time.Unix(cb.lastStateChange.Load(), 0),
		NextRetryTime:        cb.nextRetryTime(),
	}
}

// Reset força reset do circuit breaker (uso em testes ou admin)
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state.Store(int32(StateClosed))
	cb.consecutiveFailures.Store(0)
	cb.consecutiveSuccesses.Store(0)
	cb.lastStateChange.Store(time.Now().Unix())
}

// CircuitBreakerMetrics métricas do circuit breaker
type CircuitBreakerMetrics struct {
	Name                 string
	State                CircuitState
	TotalRequests        int64
	TotalSuccesses       int64
	TotalFailures        int64
	ConsecutiveFailures  int32
	ConsecutiveSuccesses int32
	LastStateChange      time.Time
	NextRetryTime        time.Time
}

// CircuitOpenError erro quando circuito está aberto
type CircuitOpenError struct {
	Name           string
	OpenedAt       time.Time
	NextRetryAfter time.Time
}

func (e *CircuitOpenError) Error() string {
	waitTime := time.Until(e.NextRetryAfter).Round(time.Second)
	return fmt.Sprintf("circuit breaker '%s' is OPEN (opened at %s, retry after %v)",
		e.Name, e.OpenedAt.Format("15:04:05"), waitTime)
}

// IsCircuitOpenError verifica se erro é de circuito aberto
func IsCircuitOpenError(err error) bool {
	_, ok := err.(*CircuitOpenError)
	return ok
}

// ============ Circuit Breaker Manager ============

// CircuitBreakerManager gerencia múltiplos circuit breakers
type CircuitBreakerManager struct {
	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex
}

// NewCircuitBreakerManager cria novo manager
func NewCircuitBreakerManager() *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]*CircuitBreaker),
	}
}

// GetOrCreate obtém ou cria circuit breaker
func (m *CircuitBreakerManager) GetOrCreate(name string, config CircuitBreakerConfig) *CircuitBreaker {
	m.mu.RLock()
	cb, exists := m.breakers[name]
	m.mu.RUnlock()

	if exists {
		return cb
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check
	if cb, exists := m.breakers[name]; exists {
		return cb
	}

	config.Name = name
	cb = NewCircuitBreaker(config)
	m.breakers[name] = cb
	return cb
}

// Get obtém circuit breaker existente
func (m *CircuitBreakerManager) Get(name string) (*CircuitBreaker, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cb, exists := m.breakers[name]
	return cb, exists
}

// GetAllMetrics retorna métricas de todos os circuit breakers
func (m *CircuitBreakerManager) GetAllMetrics() map[string]CircuitBreakerMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metrics := make(map[string]CircuitBreakerMetrics, len(m.breakers))
	for name, cb := range m.breakers {
		metrics[name] = cb.GetMetrics()
	}
	return metrics
}

// ResetAll reseta todos os circuit breakers
func (m *CircuitBreakerManager) ResetAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, cb := range m.breakers {
		cb.Reset()
	}
}
