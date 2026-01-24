package monitoring

import (
	"bytes"
	"fmt"
	"runtime"
	"time"

	"GoFacialEmulator/internal/trace"

	"github.com/gin-gonic/gin"
)

// responseWriter wrapper para capturar status e body
type responseWriter struct {
	gin.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

func (w *responseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	// Capturar body apenas para erros
	if w.statusCode >= 400 {
		w.body.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

// MetricsMiddleware middleware para coletar métricas de requisições
func MetricsMiddleware(metrics *Metrics, tracer *trace.Tracer) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		// Wrapper para capturar resposta
		writer := &responseWriter{
			ResponseWriter: c.Writer,
			statusCode:     200, // Default
			body:           bytes.NewBuffer(nil),
		}
		c.Writer = writer

		// Processar request
		c.Next()

		// Calcular duração
		duration := time.Since(start)
		statusCode := writer.statusCode

		// Registrar métricas
		var err error
		if statusCode >= 500 {
			err = fmt.Errorf("HTTP %d", statusCode)
		}
		metrics.RecordRequest(path, statusCode, duration, err)

		// Log detalhado para erros 5xx
		if statusCode >= 500 {
			tracer.Error("[HTTP ERROR] %s %s - Status: %d - Duration: %dms - Body: %s",
				c.Request.Method,
				path,
				statusCode,
				duration.Milliseconds(),
				writer.body.String())

			// Log request details para debug
			tracer.Error("[HTTP ERROR DETAILS] IP: %s, User-Agent: %s, Content-Type: %s",
				c.ClientIP(),
				c.Request.UserAgent(),
				c.ContentType())
		} else if statusCode >= 400 {
			// Log warning para 4xx
			tracer.Warning("[HTTP WARN] %s %s - Status: %d - Duration: %dms",
				c.Request.Method,
				path,
				statusCode,
				duration.Milliseconds())
		}

		// Log lento (> 1s)
		if duration > time.Second {
			tracer.Warning("[HTTP SLOW] %s %s - Duration: %dms",
				c.Request.Method,
				path,
				duration.Milliseconds())
		}
	}
}

// RecoveryMiddleware middleware personalizado para recuperação de panics
func RecoveryMiddleware(metrics *Metrics, tracer *trace.Tracer) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Log do panic
				tracer.Error("[PANIC] %s %s - Error: %v",
					c.Request.Method,
					c.Request.URL.Path,
					err)

				// Stack trace
				buf := make([]byte, 4096)
				n := runtime.Stack(buf, false)
				tracer.Error("[PANIC STACK] %s", string(buf[:n]))

				// Registrar métrica
				metrics.RecordRequest(c.Request.URL.Path, 500, 0, fmt.Errorf("panic: %v", err))

				// Retornar erro genérico
				c.JSON(500, gin.H{
					"error": "Internal server error",
					"message": "An unexpected error occurred",
				})

				c.Abort()
			}
		}()

		c.Next()
	}
}
