package staging

import (
	"context"
	"fmt"
	"time"

	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	stageDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "staging_stage_duration_seconds",
		Help:    "Duration of each reconciliation stage",
		Buckets: prometheus.ExponentialBuckets(0.01, 2, 10), // 10ms to ~10s
	}, []string{"stage"})

	batchProcessingDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "staging_batch_processing_duration_seconds",
		Help:    "Total duration to process a batch through all stages",
		Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 100ms to ~100s
	})

	batchesProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "staging_batches_processed_total",
		Help: "Total number of batches processed",
	})

	batchesFailedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "staging_batches_failed_total",
		Help: "Total number of batches that failed during processing",
	})
)

type Stage interface {
	Name() string
	Execute(ctx context.Context, batch *Batch) error
}

type StageOrchestrator struct {
	stages          []Stage
	collector       *EventCollector
	engine          *rules.InMemoryEngine
	stageTimeout    time.Duration
	shutdownTimeout time.Duration
}

func NewStageOrchestrator(
	stages []Stage,
	collector *EventCollector,
	engine *rules.InMemoryEngine,
	stageTimeout time.Duration,
) *StageOrchestrator {
	return &StageOrchestrator{
		stages:          stages,
		collector:       collector,
		engine:          engine,
		stageTimeout:    stageTimeout,
		shutdownTimeout: 30 * time.Second,
	}
}

func (so *StageOrchestrator) Start(ctx context.Context) error {
	log.Info("StageOrchestrator started", "stages", len(so.stages))

	collectorCtx, collectorCancel := context.WithCancel(ctx)
	defer collectorCancel()

	go func() {
		if err := so.collector.Start(collectorCtx); err != nil {
			log.Error(err, "EventCollector failed")
		}
	}()

	batchChan := so.collector.BatchChannel()

	for {
		select {
		case <-ctx.Done():
			log.Info("StageOrchestrator shutting down")
			return nil
		case events, ok := <-batchChan:
			if !ok {
				log.Info("Batch channel closed, exiting")
				return nil
			}

			if len(events) == 0 {
				continue
			}

			batch := NewBatch(events)
			if err := so.processBatch(ctx, batch); err != nil {
				log.Error(err, "Failed to process batch", "batchID", batch.ID, "resourceCount", len(batch.Resources))
				batchesFailedTotal.Inc()
			} else {
				batchesProcessed.Inc()
			}
		}
	}
}

func (so *StageOrchestrator) processBatch(ctx context.Context, batch *Batch) error {
	batchStart := time.Now()
	defer func() {
		duration := time.Since(batchStart).Seconds()
		batchProcessingDuration.Observe(duration)
	}()

	log.Info("Processing batch", "batchID", batch.ID, "resourceCount", len(batch.Resources))

	for _, stage := range so.stages {
		stageCtx, cancel := context.WithTimeout(ctx, so.stageTimeout)
		defer cancel()

		stageStart := time.Now()
		err := stage.Execute(stageCtx, batch)
		stageDuration.WithLabelValues(stage.Name()).Observe(time.Since(stageStart).Seconds())

		if err != nil {
			return fmt.Errorf("stage %s failed: %w", stage.Name(), err)
		}

		log.V(1).Info("Stage completed", "stage", stage.Name(), "batchID", batch.ID, "duration", time.Since(stageStart))
	}

	log.Info("Batch processed successfully", "batchID", batch.ID, "totalDuration", time.Since(batchStart))
	return nil
}
