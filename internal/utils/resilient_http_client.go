package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ResilientHTTPClient cliente HTTP com circuit breaker e retry
type ResilientHTTPClient struct {
	client         *http.Client
	circuitBreaker *CircuitBreaker
	retryConfig    RetryConfig
}

// RetryConfig configuração de retry
type RetryConfig struct {
	MaxAttempts     int           // Máximo de tentativas
	InitialDelay    time.Duration // Delay inicial
	MaxDelay        time.Duration // Delay máximo
	BackoffFactor   float64       // Fator de multiplicação (exponential backoff)
	RetryableStatus []int          // Status codes que devem ser retried
}

// DefaultRetryConfig retorna configuração padrão
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      5 * time.Second,
		BackoffFactor: 2.0,
		RetryableStatus: []int{
			http.StatusRequestTimeout,      // 408
			http.StatusTooManyRequests,     // 429
			http.StatusInternalServerError, // 500
			http.StatusBadGateway,          // 502
			http.StatusServiceUnavailable,  // 503
			http.StatusGatewayTimeout,      // 504
		},
	}
}

// NewResilientHTTPClient cria novo cliente resiliente
func NewResilientHTTPClient(circuitBreaker *CircuitBreaker, retryConfig *RetryConfig) *ResilientHTTPClient {
	if retryConfig == nil {
		defaultConfig := DefaultRetryConfig()
		retryConfig = &defaultConfig
	}

	return &ResilientHTTPClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		circuitBreaker: circuitBreaker,
		retryConfig:    *retryConfig,
	}
}

// Do executa request com circuit breaker e retry
func (c *ResilientHTTPClient) Do(req *http.Request) (*http.Response, error) {
	// Guardar corpo da request para retry (se necessário)
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
		req.Body.Close()
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	var lastErr error
	var lastResp *http.Response

	for attempt := 0; attempt < c.retryConfig.MaxAttempts; attempt++ {
		// Executar através do circuit breaker
		var resp *http.Response
		var err error

		cbErr := c.circuitBreaker.Execute(func() error {
			// Recriar body para retry
			if bodyBytes != nil {
				req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			}

			resp, err = c.client.Do(req)
			if err != nil {
				return err
			}

			// Verificar se status code indica falha
			if resp.StatusCode >= 500 {
				return fmt.Errorf("server error: status %d", resp.StatusCode)
			}

			return nil
		})

		// Circuit breaker error (circuit aberto)
		if IsCircuitOpenError(cbErr) {
			return nil, cbErr
		}

		// Request executado com sucesso
		if cbErr == nil && err == nil && resp != nil {
			// Verificar se é um status code de sucesso ou erro não retryable
			if !c.shouldRetry(resp.StatusCode) {
				return resp, nil
			}
			// Status é retryable, mas vamos retornar se é a última tentativa
			if attempt == c.retryConfig.MaxAttempts-1 {
				return resp, nil
			}
			lastResp = resp
		}

		// Guardar erro para potencial retry
		if err != nil {
			lastErr = err
		} else if cbErr != nil {
			lastErr = cbErr
		} else if resp != nil && c.shouldRetry(resp.StatusCode) {
			lastErr = fmt.Errorf("retryable status code: %d", resp.StatusCode)
		}

		// Se não é a última tentativa, fazer retry com backoff
		if attempt < c.retryConfig.MaxAttempts-1 {
			delay := c.calculateDelay(attempt)

			// Verificar se request tem context com deadline
			if req.Context() != nil {
				select {
				case <-req.Context().Done():
					return nil, req.Context().Err()
				case <-time.After(delay):
					// Continue to retry
				}
			} else {
				time.Sleep(delay)
			}
		}
	}

	// Todas as tentativas falharam
	if lastResp != nil {
		return lastResp, lastErr
	}
	return nil, lastErr
}

// Post executa POST request
func (c *ResilientHTTPClient) Post(url string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return c.Do(req)
}

// Get executa GET request
func (c *ResilientHTTPClient) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Put executa PUT request
func (c *ResilientHTTPClient) Put(url string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPut, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return c.Do(req)
}

// DoWithContext executa request com context
func (c *ResilientHTTPClient) DoWithContext(ctx context.Context, req *http.Request) (*http.Response, error) {
	req = req.WithContext(ctx)
	return c.Do(req)
}

// shouldRetry verifica se status code deve ser retried
func (c *ResilientHTTPClient) shouldRetry(statusCode int) bool {
	for _, retryableStatus := range c.retryConfig.RetryableStatus {
		if statusCode == retryableStatus {
			return true
		}
	}
	return false
}

// calculateDelay calcula delay para retry com exponential backoff
func (c *ResilientHTTPClient) calculateDelay(attempt int) time.Duration {
	delay := float64(c.retryConfig.InitialDelay) *
		pow(c.retryConfig.BackoffFactor, float64(attempt))

	if delay > float64(c.retryConfig.MaxDelay) {
		delay = float64(c.retryConfig.MaxDelay)
	}

	return time.Duration(delay)
}

// pow calcula potência (simple implementation)
func pow(base, exp float64) float64 {
	result := 1.0
	for i := 0; i < int(exp); i++ {
		result *= base
	}
	return result
}

// GetCircuitBreakerMetrics retorna métricas do circuit breaker
func (c *ResilientHTTPClient) GetCircuitBreakerMetrics() CircuitBreakerMetrics {
	return c.circuitBreaker.GetMetrics()
}

// ============ HTTP Client Factory ============

// HTTPClientFactory cria clientes HTTP resilientes para emuladores
type HTTPClientFactory struct {
	cbManager *CircuitBreakerManager
}

// NewHTTPClientFactory cria nova factory
func NewHTTPClientFactory() *HTTPClientFactory {
	return &HTTPClientFactory{
		cbManager: NewCircuitBreakerManager(),
	}
}

// GetClientForEmulator retorna cliente HTTP para um emulador específico
func (f *HTTPClientFactory) GetClientForEmulator(emulatorID int) *ResilientHTTPClient {
	cbName := fmt.Sprintf("emulator_%d", emulatorID)

	cb := f.cbManager.GetOrCreate(cbName, CircuitBreakerConfig{
		Name:              cbName,
		MaxFailures:       5,  // 5 falhas consecutivas
		ResetTimeout:      30 * time.Second,
		HalfOpenSuccesses: 2,  // 2 sucessos para fechar circuito
	})

	return NewResilientHTTPClient(cb, nil)
}

// GetMetricsForAllEmulators retorna métricas de todos os emuladores
func (f *HTTPClientFactory) GetMetricsForAllEmulators() map[string]CircuitBreakerMetrics {
	return f.cbManager.GetAllMetrics()
}

// ResetAll reseta todos os circuit breakers
func (f *HTTPClientFactory) ResetAll() {
	f.cbManager.ResetAll()
}
