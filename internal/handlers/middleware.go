package handlers

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"GoFacialEmulator/internal/database"
	"GoFacialEmulator/internal/trace"

	"github.com/gin-gonic/gin"
)

// MiddlewareConfig contém configurações para middleware
type MiddlewareConfig struct {
	RequestTimeout time.Duration
	EnableCORS     bool
	EnableMetrics  bool
}

// DefaultMiddlewareConfig retorna configuração padrão
func DefaultMiddlewareConfig() *MiddlewareConfig {
	return &MiddlewareConfig{
		RequestTimeout: 30 * time.Second,
		EnableCORS:     true,
		EnableMetrics:  true,
	}
}

// SetupMiddlewares configura todos os middlewares
func (h *Handler) SetupMiddlewares(router *gin.Engine, config *MiddlewareConfig) {
	// Middleware de timeout
	router.Use(h.timeoutMiddleware(config.RequestTimeout))

	// Middleware de CORS
	if config.EnableCORS {
		router.Use(h.corsMiddleware())
	}

	// Middleware de logging estruturado
	router.Use(h.structuredLoggingMiddleware())

	// Middleware de recuperação melhorado
	router.Use(h.recoveryMiddleware())

	// Middleware de métricas
	if config.EnableMetrics {
		router.Use(h.metricsMiddleware())
	}

	// Middleware de rate limiting (básico)
	router.Use(h.rateLimitMiddleware())
}

// timeoutMiddleware adiciona timeout às requisições
func (h *Handler) timeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)

		done := make(chan struct{})
		go func() {
			defer close(done)
			c.Next()
		}()

		select {
		case <-done:
			return
		case <-ctx.Done():
			c.JSON(http.StatusRequestTimeout, gin.H{
				"error": "Request timeout",
				"code":  "TIMEOUT",
			})
			c.Abort()
		}
	}
}

// corsMiddleware configura CORS
func (h *Handler) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// structuredLoggingMiddleware registra requisições de forma estruturada
func (h *Handler) structuredLoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method
		clientIP := c.ClientIP()
		userAgent := c.Request.UserAgent()

		c.Next()

		end := time.Now()
		latency := end.Sub(start)
		statusCode := c.Writer.Status()
		bodySize := c.Writer.Size()

		h.tracer.Info("HTTP Request - Method: %s, Path: %s, Status: %d, Latency: %v, Size: %d, IP: %s, UA: %s",
			method, path, statusCode, latency, bodySize, clientIP, userAgent)
	}
}

// recoveryMiddleware recupera de panics de forma mais robusta
func (h *Handler) recoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Log do stack trace
				stack := debug.Stack()
				h.tracer.Error("Panic recovered: %v\nStack:\n%s", err, string(stack))

				// Resposta estruturada de erro
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":     "Internal server error",
					"code":      "INTERNAL_ERROR",
					"timestamp": time.Now().UTC(),
				})

				c.Abort()
			}
		}()

		c.Next()
	}
}

// metricsMiddleware coleta métricas básicas
func (h *Handler) metricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start)
		statusCode := c.Writer.Status()

		// Aqui você pode integrar com sistemas de métricas como Prometheus
		// Por enquanto, apenas log estruturado
		if duration > 1*time.Second {
			h.tracer.Warning("Slow request detected - Path: %s, Duration: %v, Status: %d",
				c.Request.URL.Path, duration, statusCode)
		}
	}
}

// rateLimitMiddleware implementa rate limiting básico por IP
func (h *Handler) rateLimitMiddleware() gin.HandlerFunc {
	// Mapa simples para rate limiting (em produção, usar Redis ou similar)
	requests := make(map[string][]time.Time)
	maxRequests := 100
	window := time.Minute

	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		now := time.Now()

		// Limpar requisições antigas
		if times, exists := requests[clientIP]; exists {
			var validTimes []time.Time
			for _, t := range times {
				if now.Sub(t) < window {
					validTimes = append(validTimes, t)
				}
			}
			requests[clientIP] = validTimes
		}

		// Verificar limite
		if len(requests[clientIP]) >= maxRequests {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Rate limit exceeded",
				"code":  "RATE_LIMIT",
			})
			c.Abort()
			return
		}

		// Adicionar requisição atual
		requests[clientIP] = append(requests[clientIP], now)

		c.Next()
	}
}

// RequestValidator valida campos obrigatórios
type RequestValidator struct {
	tracer *trace.Tracer
}

// NewRequestValidator cria um novo validador
func NewRequestValidator(tracer *trace.Tracer) *RequestValidator {
	return &RequestValidator{tracer: tracer}
}

// ValidateDeviceID valida se o ID do dispositivo é válido
func (v *RequestValidator) ValidateDeviceID(c *gin.Context) (int, error) {
	idStr := c.Param("id")
	if idStr == "" {
		return 0, fmt.Errorf("device ID is required")
	}

	id := 0
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		return 0, fmt.Errorf("invalid device ID format")
	}

	if id <= 0 {
		return 0, fmt.Errorf("device ID must be positive")
	}

	return id, nil
}

// ValidateJSON valida se o body da requisição é um JSON válido
func (v *RequestValidator) ValidateJSON(c *gin.Context, target interface{}) error {
	if err := c.ShouldBindJSON(target); err != nil {
		v.tracer.Warning("Invalid JSON in request: %v", err)
		return fmt.Errorf("invalid JSON format: %v", err)
	}
	return nil
}

// HealthChecker verifica a saúde de componentes
type HealthChecker struct {
	serviceDB *database.ServiceDB
	tracer    *trace.Tracer
}

// NewHealthChecker cria um novo verificador de saúde
func NewHealthChecker(serviceDB *database.ServiceDB, tracer *trace.Tracer) *HealthChecker {
	return &HealthChecker{
		serviceDB: serviceDB,
		tracer:    tracer,
	}
}

// CheckDatabase verifica se o banco está acessível
func (hc *HealthChecker) CheckDatabase(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return hc.serviceDB.Ping(ctx)
}

// GetSystemHealth retorna status detalhado do sistema
func (hc *HealthChecker) GetSystemHealth(ctx context.Context) map[string]interface{} {
	health := map[string]interface{}{
		"timestamp": time.Now().UTC(),
		"status":    "healthy",
		"checks":    map[string]interface{}{},
	}

	// Verificar banco de dados
	if err := hc.CheckDatabase(ctx); err != nil {
		health["status"] = "unhealthy"
		health["checks"].(map[string]interface{})["database"] = map[string]interface{}{
			"status": "failed",
			"error":  err.Error(),
		}
	} else {
		health["checks"].(map[string]interface{})["database"] = map[string]interface{}{
			"status": "ok",
		}
	}

	return health
}
