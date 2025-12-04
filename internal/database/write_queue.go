package database

import (
	"context"
	"fmt"
	"sync"
	"time"

	"GoFacialEmulator/internal/trace"
)

// WriteOperation representa uma operação de escrita pendente
type WriteOperation struct {
	Query      string
	Args       []interface{}
	ResultChan chan error // Canal para retornar resultado
}

// WriteQueue gerencia operações de escrita em batch para otimizar performance
type WriteQueue struct {
	pool   *AdaptivePool
	tracer *trace.Tracer

	// Canal de operações pendentes
	operations chan *WriteOperation

	// Configurações de batch
	batchSize     int
	flushInterval time.Duration

	// Controle
	stopChan chan struct{}
	wg       sync.WaitGroup

	// Métricas
	totalOps      int64
	totalBatches  int64
	metricsMutex  sync.RWMutex
}

// NewWriteQueue cria uma nova fila de escrita com batching
func NewWriteQueue(pool *AdaptivePool, tracer *trace.Tracer) *WriteQueue {
	wq := &WriteQueue{
		pool:          pool,
		tracer:        tracer,
		operations:    make(chan *WriteOperation, 1000), // Buffer de 1000 ops
		batchSize:     50,                                // Batch de 50 operações
		flushInterval: 2 * time.Second,                   // Flush a cada 2s
		stopChan:      make(chan struct{}),
	}

	// Iniciar worker em background
	wq.wg.Add(1)
	go wq.worker()

	return wq
}

// Enqueue adiciona uma operação à fila (não bloqueante)
func (wq *WriteQueue) Enqueue(query string, args ...interface{}) error {
	op := &WriteOperation{
		Query:      query,
		Args:       args,
		ResultChan: make(chan error, 1),
	}

	select {
	case wq.operations <- op:
		// Aguardar resultado (com timeout)
		select {
		case err := <-op.ResultChan:
			return err
		case <-time.After(10 * time.Second):
			return fmt.Errorf("timeout waiting for write operation")
		}
	default:
		return fmt.Errorf("write queue is full")
	}
}

// EnqueueAsync adiciona uma operação à fila sem aguardar resultado
func (wq *WriteQueue) EnqueueAsync(query string, args ...interface{}) {
	op := &WriteOperation{
		Query:      query,
		Args:       args,
		ResultChan: nil, // Sem canal de resultado para async
	}

	select {
	case wq.operations <- op:
		// OK, enfileirado
	default:
		wq.tracer.Warning("Write queue full, dropping async operation")
	}
}

// worker processa operações em batches
func (wq *WriteQueue) worker() {
	defer wq.wg.Done()

	batch := make([]*WriteOperation, 0, wq.batchSize)
	flushTimer := time.NewTimer(wq.flushInterval)
	defer flushTimer.Stop()

	for {
		select {
		case op := <-wq.operations:
			batch = append(batch, op)

			// Flush se batch estiver cheio
			if len(batch) >= wq.batchSize {
				wq.flushBatch(batch)
				batch = batch[:0] // Limpar batch
				flushTimer.Reset(wq.flushInterval)
			}

		case <-flushTimer.C:
			// Flush periódico
			if len(batch) > 0 {
				wq.flushBatch(batch)
				batch = batch[:0]
			}
			flushTimer.Reset(wq.flushInterval)

		case <-wq.stopChan:
			// Flush final antes de parar
			if len(batch) > 0 {
				wq.flushBatch(batch)
			}
			return
		}
	}
}

// flushBatch executa um batch de operações
func (wq *WriteQueue) flushBatch(batch []*WriteOperation) {
	if len(batch) == 0 {
		return
	}

	wq.tracer.Info("[WriteQueue] Flushing batch of %d operations", len(batch))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Executar cada operação no batch
	for _, op := range batch {
		_, err := wq.pool.Exec(ctx, op.Query, op.Args...)

		// Enviar resultado se houver canal
		if op.ResultChan != nil {
			select {
			case op.ResultChan <- err:
			default:
				// Canal pode estar fechado
			}
		}

		if err != nil {
			wq.tracer.Error("[WriteQueue] Failed to execute operation: %v", err)
		}
	}

	// Atualizar métricas
	wq.metricsMutex.Lock()
	wq.totalOps += int64(len(batch))
	wq.totalBatches++
	wq.metricsMutex.Unlock()
}

// Stop para a WriteQueue gracefully
func (wq *WriteQueue) Stop() {
	close(wq.stopChan)
	wq.wg.Wait()

	wq.tracer.Info("[WriteQueue] Stopped. Total ops: %d, Total batches: %d",
		wq.totalOps, wq.totalBatches)
}

// GetMetrics retorna métricas da WriteQueue
func (wq *WriteQueue) GetMetrics() map[string]interface{} {
	wq.metricsMutex.RLock()
	defer wq.metricsMutex.RUnlock()

	return map[string]interface{}{
		"total_operations": wq.totalOps,
		"total_batches":    wq.totalBatches,
		"queue_length":     len(wq.operations),
		"queue_capacity":   cap(wq.operations),
	}
}
