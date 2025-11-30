package staging

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentoctopuscomv1beta1 "github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ConditionTypeReady      = "Ready"
	ConditionTypePlanning   = "Planning"
	ConditionTypeValidation = "Validation"
	ConditionTypeExecution  = "Execution"

	ReasonInProgress = "InProgress"
	ReasonSucceeded  = "Succeeded"
	ReasonFailed     = "Failed"
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

	batchesRetriedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "staging_batches_retried_total",
		Help: "Total number of batch retry attempts",
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
	client          client.Client
	stageTimeout    time.Duration
	shutdownTimeout time.Duration
	maxRetries      int
	retryQueue      BatchQueue
	triggerCh       chan struct{}
	recorder        record.EventRecorder
}

func NewStageOrchestrator(
	stages []Stage,
	collector *EventCollector,
	engine *rules.InMemoryEngine,
	c client.Client,
	stageTimeout time.Duration,
	recorder record.EventRecorder,
) *StageOrchestrator {
	return &StageOrchestrator{
		stages:          stages,
		collector:       collector,
		engine:          engine,
		client:          c,
		stageTimeout:    stageTimeout,
		shutdownTimeout: 30 * time.Second,
		maxRetries:      3,
		retryQueue:      NewBatchQueue(),
		triggerCh:       make(chan struct{}, 10),
		recorder:        recorder,
	}
}

func (so *StageOrchestrator) NeedLeaderElection() bool {
	return true
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

	go func() {
		<-ctx.Done()
		so.retryQueue.ShutDown()
	}()

	go so.processRetryQueue(ctx)

	batchChan := so.collector.BatchChannel()

	for {
		select {
		case <-ctx.Done():
			log.Info("StageOrchestrator shutting down")
			return nil
		case <-so.triggerCh:
			log.V(1).Info("Received reconciliation trigger, flushing pending events")
			so.collector.FlushEvents()
		case events, ok := <-batchChan:
			if !ok {
				log.Info("Batch channel closed, exiting")
				return nil
			}

			batch := NewBatch(events)
			so.processBatchWithRetry(ctx, batch)
		}
	}
}

func (so *StageOrchestrator) processRetryQueue(ctx context.Context) {
	for {
		batch, shutdown := so.retryQueue.Get()
		if shutdown {
			log.Info("Retry queue shutting down")
			return
		}
		if batch == nil {
			continue
		}

		batchesRetriedTotal.Inc()
		so.processBatchWithRetry(ctx, batch)
		so.retryQueue.Done(batch)
	}
}

func (so *StageOrchestrator) processBatchWithRetry(ctx context.Context, batch *Batch) {
	err := so.processBatch(ctx, batch)

	if err != nil {
		batch.LastError = err
		retryCount := so.retryQueue.NumRequeues(batch) + 1

		if retryCount <= so.maxRetries {
			log.Info("Batch failed, queueing for retry",
				"batchID", batch.ID,
				"retryCount", retryCount,
				"maxRetries", so.maxRetries,
				"error", err.Error())

			so.emitEvent(batch, corev1.EventTypeWarning, "RetryQueued",
				fmt.Sprintf("Queued for retry %d/%d: %s", retryCount, so.maxRetries, err.Error()))

			so.retryQueue.AddRateLimited(batch)
		} else {
			so.retryQueue.Forget(batch)
			so.handlePermanentFailure(ctx, batch, retryCount, err)
		}
	} else {
		so.retryQueue.Forget(batch)
		batchesProcessed.Inc()
	}
}

func (so *StageOrchestrator) handlePermanentFailure(ctx context.Context, batch *Batch, retryCount int, err error) {
	log.Error(err, "Batch processing failed permanently",
		"batchID", batch.ID,
		"resourceCount", len(batch.Resources),
		"retryCount", retryCount)
	batchesFailedTotal.Inc()

	so.emitEvent(batch, corev1.EventTypeWarning, "ReconcileFailed",
		fmt.Sprintf("Permanent failure after %d retries: %s (batch %s)", retryCount, err.Error(), batch.ID))

	log.Info("Rebuilding state from cluster after permanent failure", "batchID", batch.ID)
	if rebuildErr := so.engine.RebuildStateFromCluster(ctx); rebuildErr != nil {
		log.Error(rebuildErr, "Failed to rebuild state after batch failure",
			"batchID", batch.ID)
	} else {
		log.Info("State rebuilt successfully after failure", "batchID", batch.ID)
		so.requeueFailedResources(ctx, batch)
	}
}

func (so *StageOrchestrator) requeueFailedResources(ctx context.Context, batch *Batch) {
	requeued := 0
	for _, res := range batch.Resources {
		key := types.NamespacedName{Namespace: res.GetNamespace(), Name: res.GetName()}
		obj := newWSAClientObject(res)

		if err := so.client.Get(ctx, key, obj); err != nil {
			log.V(1).Info("Failed to fetch resource for requeue", "key", key, "error", err)
			continue
		}

		var fresh rules.WSAResource
		var generation int64
		var resourceVersion string

		switch o := obj.(type) {
		case *agentoctopuscomv1beta1.ClusterWorkloadServiceAccount:
			fresh = rules.NewClusterWSAResource(o)
			generation = o.Generation
			resourceVersion = o.ResourceVersion
		case *agentoctopuscomv1beta1.WorkloadServiceAccount:
			fresh = rules.NewWSAResource(o)
			generation = o.Generation
			resourceVersion = o.ResourceVersion
		}

		event := &EventInfo{
			Resource:        fresh,
			EventType:       EventTypeUpdate,
			Generation:      generation,
			ResourceVersion: resourceVersion,
			Timestamp:       time.Now(),
		}

		so.collector.AddEvent(event)
		requeued++
	}

	log.Info("Re-queued failed resources for reconciliation", "batchID", batch.ID, "requeued", requeued)
}

var stageToConditionType = map[string]string{
	"planning":   ConditionTypePlanning,
	"validation": ConditionTypeValidation,
	"execution":  ConditionTypeExecution,
}

func (so *StageOrchestrator) processBatch(ctx context.Context, batch *Batch) error {
	batchStart := time.Now()
	defer func() {
		duration := time.Since(batchStart).Seconds()
		batchProcessingDuration.Observe(duration)
	}()

	if len(batch.Resources) == 0 {
		log.V(1).Info("Skipping empty batch", "batchID", batch.ID)
		return nil
	}

	log.Info("Processing batch", "batchID", batch.ID, "resourceCount", len(batch.Resources))

	var failedStage string
	var stageErr error

	for _, stage := range so.stages {
		so.updateStageStatus(ctx, batch, stage.Name(), ReasonInProgress, "Processing")
		so.emitEvent(batch, corev1.EventTypeNormal, "StageStarted",
			fmt.Sprintf("Stage %s started (batch %s)", stage.Name(), batch.ID))

		stageCtx, cancel := context.WithTimeout(ctx, so.stageTimeout)
		stageStart := time.Now()
		err := stage.Execute(stageCtx, batch)
		cancel()

		stageDuration.WithLabelValues(stage.Name()).Observe(time.Since(stageStart).Seconds())

		if err != nil {
			so.updateStageStatus(ctx, batch, stage.Name(), ReasonFailed, err.Error())
			so.emitEvent(batch, corev1.EventTypeWarning, "StageFailed",
				fmt.Sprintf("Stage %s failed: %s (batch %s)", stage.Name(), err.Error(), batch.ID))
			failedStage = stage.Name()
			stageErr = err
			break
		}

		so.updateStageStatus(ctx, batch, stage.Name(), ReasonSucceeded, "Complete")
		so.emitEvent(batch, corev1.EventTypeNormal, "StageCompleted",
			fmt.Sprintf("Stage %s completed (batch %s, duration %v)", stage.Name(), batch.ID, time.Since(stageStart)))
	}

	so.updateFinalConditions(ctx, batch, failedStage, stageErr)

	if stageErr != nil {
		return fmt.Errorf("stage %s failed: %w", failedStage, stageErr)
	}

	so.emitReconcileEvents(batch)

	if batch.RequeueAfter > 0 {
		log.Info("Scheduling follow-up reconciliation", "batchID", batch.ID, "requeueAfter", batch.RequeueAfter)
		time.AfterFunc(batch.RequeueAfter, func() {
			so.triggerReconcile()
		})
	}

	log.Info("Batch processed successfully", "batchID", batch.ID, "totalDuration", time.Since(batchStart))
	return nil
}

func (so *StageOrchestrator) updateStageStatus(ctx context.Context, batch *Batch, stageName, reason, message string) {
	conditionType := stageToConditionType[stageName]
	if conditionType == "" {
		return
	}

	status := metav1.ConditionFalse
	switch reason {
	case ReasonSucceeded:
		status = metav1.ConditionTrue
	case ReasonInProgress:
		status = metav1.ConditionUnknown
	}

	so.updateConditions(ctx, batch, conditionType, status, reason, message)
}

func (so *StageOrchestrator) emitEvent(batch *Batch, eventType, reason, message string) {
	if so.recorder == nil {
		return
	}
	for _, res := range batch.Resources {
		if obj, ok := res.GetOwnerObject().(runtime.Object); ok {
			so.recorder.Event(obj, eventType, reason, message)
		}
	}
}

func (so *StageOrchestrator) emitReconcileEvents(batch *Batch) {
	if so.recorder == nil || batch.Plan == nil {
		return
	}

	targetNamespaces := batch.Plan.TargetNamespaces

	for _, resource := range batch.Resources {
		obj, ok := resource.GetOwnerObject().(runtime.Object)
		if !ok {
			continue
		}

		wsaKey := resource.GetNamespacedName()
		saNames := batch.Plan.WSAToSANames[wsaKey]
		if len(saNames) == 0 {
			so.recorder.Event(obj, corev1.EventTypeNormal, "Reconciled",
				"No ServiceAccounts generated (no matching scopes)")
			continue
		}

		saNameStrings := make([]string, len(saNames))
		for i, sa := range saNames {
			saNameStrings[i] = string(sa)
		}

		message := fmt.Sprintf("Reconciled %d ServiceAccount(s): %s across %d namespace(s)",
			len(saNames),
			strings.Join(saNameStrings, ", "),
			len(targetNamespaces))

		so.recorder.Event(obj, corev1.EventTypeNormal, "Reconciled", message)
	}
}

func (so *StageOrchestrator) updateFinalConditions(
	ctx context.Context, batch *Batch, failedStage string, err error,
) {
	if failedStage != "" {
		message := fmt.Sprintf("Stage %s failed: %s", failedStage, err.Error())
		so.updateConditions(ctx, batch, ConditionTypeReady, metav1.ConditionFalse, ReasonFailed, message)
	} else {
		so.updateConditions(ctx, batch, ConditionTypeReady, metav1.ConditionTrue, ReasonSucceeded, "All stages completed successfully")
	}
}

func (so *StageOrchestrator) triggerReconcile() {
	select {
	case so.triggerCh <- struct{}{}:
		log.V(1).Info("Queued reconciliation trigger")
	default:
		log.V(1).Info("Trigger channel full, reconciliation already pending")
	}
}

func (so *StageOrchestrator) updateConditions(
	ctx context.Context, batch *Batch, conditionType string, status metav1.ConditionStatus, reason, message string,
) {
	for _, res := range batch.Resources {
		if err := res.UpdateCondition(ctx, so.client, conditionType, status, reason, message); err != nil {
			log.V(1).Info("Failed to update condition",
				"resource", res.GetNamespace()+"/"+res.GetName(),
				"conditionType", conditionType,
				"error", err)
		}
	}
}

func newWSAClientObject(res rules.WSAResource) client.Object {
	if res.IsClusterScoped() {
		return &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{}
	}
	return &agentoctopuscomv1beta1.WorkloadServiceAccount{}
}
