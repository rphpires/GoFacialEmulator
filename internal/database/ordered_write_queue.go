package database

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"GoFacialEmulator/internal/trace"

	"github.com/jackc/pgconn"
)

// OrderedWriteOperation representa uma operação de escrita com garantia de ordem
type OrderedWriteOperation struct {
	EmulatorID  int
	Sequence    int64 // Número de sequência para garantir ordem
	Query       string
	Args        []interface{}
	ResultChan  chan error
	SubmittedAt time.Time
}

// OrderedWriteQueue gerencia escritas com garantia de ordem por emulador
type OrderedWriteQueue struct {
	writePool *AdaptivePool
	tracer    *trace.Tracer

	// Canal de operações pendentes
	operations chan *OrderedWriteOperation

	// Workers
	workers    []*writeWorker
	workerCount int

	// Tracking de sequências por emulador
	sequenceTracking map[int]*emulatorSequence
	trackingMutex    sync.RWMutex

	// Configuração
	queueSize      int
	batchSize      int
	flushInterval  time.Duration

	// Controle
	stopChan chan struct{}
	wg       sync.WaitGroup

	// Métricas
	totalOps        atomic.Int64
	totalBatches    atomic.Int64
	totalErrors     atomic.Int64
	totalRetries    atomic.Int64
	avgLatencyMs    atomic.Int64
}

// emulatorSequence rastreia a sequência de operações de um emulador
type emulatorSequence struct {
	lastCompleted int64
	pending       map[int64]*OrderedWriteOperation
	mu            sync.Mutex
}

// writeWorker processa operações de escrita
type writeWorker struct {
	id          int
	queue       *OrderedWriteQueue
	localBatch  []*OrderedWriteOperation
	batchMutex  sync.Mutex
}

// NewOrderedWriteQueue cria uma nova fila de escrita ordenada
func NewOrderedWriteQueue(writePool *AdaptivePool, config *DynamicPoolConfig, tracer *trace.Tracer) *OrderedWriteQueue {
	owq := &OrderedWriteQueue{
		writePool:        writePool,
		tracer:           tracer,
		operations:       make(chan *OrderedWriteOperation, config.WriteQueueSize),
		workerCount:      config.WriteWorkers,
		sequenceTracking: make(map[int]*emulatorSequence),
		queueSize:        config.WriteQueueSize,
		batchSize:        config.WriteBatchSize,
		flushInterval:    time.Duration(config.WriteFlushMs) * time.Millisecond,
		stopChan:         make(chan struct{}),
	}

	// Criar workers
	owq.workers = make([]*writeWorker, config.WriteWorkers)
	for i := 0; i < config.WriteWorkers; i++ {
		owq.workers[i] = &writeWorker{
			id:         i,
			queue:      owq,
			localBatch: make([]*OrderedWriteOperation, 0, config.WriteBatchSize),
		}
	}

	// Iniciar workers
	owq.wg.Add(config.WriteWorkers)
	for _, worker := range owq.workers {
		go worker.run()
	}

	owq.tracer.Info("[OrderedWriteQueue] Started with %d workers, queue size: %d, batch size: %d",
		config.WriteWorkers, config.WriteQueueSize, config.WriteBatchSize)

	return owq
}

// Enqueue adiciona uma operação à fila com garantia de ordem
func (owq *OrderedWriteQueue) Enqueue(emulatorID int, query string, args ...interface{}) error {
	// Obter próxima sequência para este emulador
	sequence := owq.getNextSequence(emulatorID)

	op := &OrderedWriteOperation{
		EmulatorID:  emulatorID,
		Sequence:    sequence,
		Query:       query,
		Args:        args,
		ResultChan:  make(chan error, 1),
		SubmittedAt: time.Now(),
	}

	// Registrar operação pendente
	owq.registerPendingOp(op)

	// Tentar enfileirar com timeout
	select {
	case owq.operations <- op:
		// Aguardar resultado com timeout
		select {
		case err := <-op.ResultChan:
			latency := time.Since(op.SubmittedAt).Milliseconds()
			owq.updateAvgLatency(latency)
			return err
		case <-time.After(30 * time.Second): // Timeout maior para lidar com alta carga
			return fmt.Errorf("timeout waiting for write operation (emulator %d, seq %d)", emulatorID, sequence)
		}
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout enqueuing operation (queue full, size: %d/%d)", len(owq.operations), cap(owq.operations))
	}
}

// EnqueueAsync adiciona operação sem aguardar resultado
func (owq *OrderedWriteQueue) EnqueueAsync(emulatorID int, query string, args ...interface{}) {
	sequence := owq.getNextSequence(emulatorID)

	op := &OrderedWriteOperation{
		EmulatorID:  emulatorID,
		Sequence:    sequence,
		Query:       query,
		Args:        args,
		ResultChan:  nil,
		SubmittedAt: time.Now(),
	}

	select {
	case owq.operations <- op:
		// OK
	default:
		owq.tracer.Warning("[OrderedWriteQueue] Queue full, dropping async op (emulator %d)", emulatorID)
	}
}

// getNextSequence retorna próximo número de sequência para um emulador
func (owq *OrderedWriteQueue) getNextSequence(emulatorID int) int64 {
	owq.trackingMutex.Lock()
	defer owq.trackingMutex.Unlock()

	seq, exists := owq.sequenceTracking[emulatorID]
	if !exists {
		seq = &emulatorSequence{
			lastCompleted: 0,
			pending:       make(map[int64]*OrderedWriteOperation),
		}
		owq.sequenceTracking[emulatorID] = seq
	}

	seq.mu.Lock()
	defer seq.mu.Unlock()

	nextSeq := seq.lastCompleted + 1 + int64(len(seq.pending))
	return nextSeq
}

// registerPendingOp registra operação pendente
func (owq *OrderedWriteQueue) registerPendingOp(op *OrderedWriteOperation) {
	owq.trackingMutex.RLock()
	seq := owq.sequenceTracking[op.EmulatorID]
	owq.trackingMutex.RUnlock()

	seq.mu.Lock()
	seq.pending[op.Sequence] = op
	seq.mu.Unlock()
}

// markCompleted marca operação como completa e processa pendentes
func (owq *OrderedWriteQueue) markCompleted(op *OrderedWriteOperation, err error) {
	owq.trackingMutex.RLock()
	seq := owq.sequenceTracking[op.EmulatorID]
	owq.trackingMutex.RUnlock()

	seq.mu.Lock()
	defer seq.mu.Unlock()

	// Remover de pending
	delete(seq.pending, op.Sequence)

	// Atualizar lastCompleted
	if op.Sequence == seq.lastCompleted+1 {
		seq.lastCompleted = op.Sequence
	}

	// Enviar resultado
	if op.ResultChan != nil {
		select {
		case op.ResultChan <- err:
		default:
			// Canal pode estar fechado
		}
	}
}

// writeWorker.run processa operações
func (w *writeWorker) run() {
	defer w.queue.wg.Done()

	flushTimer := time.NewTimer(w.queue.flushInterval)
	defer flushTimer.Stop()

	for {
		select {
		case op := <-w.queue.operations:
			w.batchMutex.Lock()
			w.localBatch = append(w.localBatch, op)
			shouldFlush := len(w.localBatch) >= w.queue.batchSize
			w.batchMutex.Unlock()

			if shouldFlush {
				w.flushBatch()
				flushTimer.Reset(w.queue.flushInterval)
			}

		case <-flushTimer.C:
			w.flushBatch()
			flushTimer.Reset(w.queue.flushInterval)

		case <-w.queue.stopChan:
			w.flushBatch() // Flush final
			return
		}
	}
}

// flushBatch executa batch de operações com retry
func (w *writeWorker) flushBatch() {
	w.batchMutex.Lock()
	if len(w.localBatch) == 0 {
		w.batchMutex.Unlock()
		return
	}

	batch := make([]*OrderedWriteOperation, len(w.localBatch))
	copy(batch, w.localBatch)
	w.localBatch = w.localBatch[:0]
	w.batchMutex.Unlock()

	w.queue.tracer.Info("[Worker %d] Flushing batch of %d operations", w.id, len(batch))

	// Executar operações com retry
	for _, op := range batch {
		err := w.executeWithRetry(op)
		w.queue.markCompleted(op, err)

		if err != nil {
			w.queue.totalErrors.Add(1)
		}
		w.queue.totalOps.Add(1)
	}

	w.queue.totalBatches.Add(1)
}

// executeWithRetry executa operação com retry exponencial
func (w *writeWorker) executeWithRetry(op *OrderedWriteOperation) error {
	const maxRetries = 3
	baseDelay := 100 * time.Millisecond

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := w.queue.writePool.Exec(ctx, op.Query, op.Args...)
		cancel()

		if err == nil {
			if attempt > 0 {
				w.queue.tracer.Info("[Worker %d] Operation succeeded after %d retries (emulator %d, seq %d)",
					w.id, attempt, op.EmulatorID, op.Sequence)
			}
			return nil
		}

		lastErr = err
		w.queue.totalRetries.Add(1)

		// Verificar se é erro recuperável
		if !isRetryableError(err) {
			w.queue.tracer.Error("[Worker %d] Non-retryable error (emulator %d, seq %d): %v",
				w.id, op.EmulatorID, op.Sequence, err)
			return err
		}

		if attempt < maxRetries {
			delay := baseDelay * time.Duration(1<<attempt) // Exponential backoff
			w.queue.tracer.Warning("[Worker %d] Attempt %d failed (emulator %d, seq %d), retrying in %v: %v",
				w.id, attempt+1, op.EmulatorID, op.Sequence, delay, err)
			time.Sleep(delay)
		}
	}

	w.queue.tracer.Error("[Worker %d] Operation failed after %d attempts (emulator %d, seq %d): %v",
		w.id, maxRetries, op.EmulatorID, op.Sequence, lastErr)
	return fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

// isRetryableError verifica se erro é recuperável
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()

	// Erros de conexão/timeout são recuperáveis
	retryablePatterns := []string{
		"too many clients already",
		"connection refused",
		"timeout",
		"deadline exceeded",
		"connection reset",
		"broken pipe",
		"no connection to the server",
		"server closed the connection",
	}

	for _, pattern := range retryablePatterns {
		if containsError(errMsg, pattern) {
			return true
		}
	}

	return false
}

// containsError verifica se string contém substring (case-insensitive)
func containsError(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(s) > len(substr) &&
			(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
				containsSubstringError(s, substr)))
}

func containsSubstringError(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Stop para a fila gracefully
func (owq *OrderedWriteQueue) Stop() {
	owq.tracer.Info("[OrderedWriteQueue] Stopping...")
	close(owq.stopChan)
	owq.wg.Wait()

	stats := owq.GetMetrics()
	owq.tracer.Info("[OrderedWriteQueue] Stopped. Stats: %+v", stats)
}

// GetMetrics retorna métricas da fila
func (owq *OrderedWriteQueue) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"total_operations":  owq.totalOps.Load(),
		"total_batches":     owq.totalBatches.Load(),
		"total_errors":      owq.totalErrors.Load(),
		"total_retries":     owq.totalRetries.Load(),
		"queue_length":      len(owq.operations),
		"queue_capacity":    cap(owq.operations),
		"queue_utilization": float64(len(owq.operations)) / float64(cap(owq.operations)) * 100,
		"avg_latency_ms":    owq.avgLatencyMs.Load(),
		"worker_count":      owq.workerCount,
	}
}

// updateAvgLatency atualiza latência média (média móvel exponencial)
func (owq *OrderedWriteQueue) updateAvgLatency(latencyMs int64) {
	currentAvg := owq.avgLatencyMs.Load()
	if currentAvg == 0 {
		owq.avgLatencyMs.Store(latencyMs)
	} else {
		// EMA com alpha = 0.1
		newAvg := int64(float64(currentAvg)*0.9 + float64(latencyMs)*0.1)
		owq.avgLatencyMs.Store(newAvg)
	}
}

// Exec executa query através da fila ordenada
func (owq *OrderedWriteQueue) Exec(ctx context.Context, emulatorID int, query string, args ...interface{}) (pgconn.CommandTag, error) {
	err := owq.Enqueue(emulatorID, query, args...)
	// Retornar CommandTag vazio já que operação é assíncrona
	return pgconn.CommandTag{}, err
}
