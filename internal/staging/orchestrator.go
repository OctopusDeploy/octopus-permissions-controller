package staging

import (
	"context"
	"fmt"
	"time"

	agentoctopuscomv1beta1 "github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

	retryQueueDroppedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "staging_retry_queue_dropped_total",
		Help: "Total number of batches dropped due to full retry queue",
	})
)

const (
	defaultRetryQueueSize = 100
	retryQueueSendTimeout = 30 * time.Second
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
	retryDelay      time.Duration
	retryQueue      chan *retryItem
	triggerCh       chan struct{}
}

type retryItem struct {
	batch      *Batch
	retryAfter time.Time
}

func NewStageOrchestrator(
	stages []Stage,
	collector *EventCollector,
	engine *rules.InMemoryEngine,
	c client.Client,
	stageTimeout time.Duration,
) *StageOrchestrator {
	return &StageOrchestrator{
		stages:          stages,
		collector:       collector,
		engine:          engine,
		client:          c,
		stageTimeout:    stageTimeout,
		shutdownTimeout: 30 * time.Second,
		maxRetries:      3,
		retryDelay:      time.Second,
		retryQueue:      make(chan *retryItem, defaultRetryQueueSize),
		triggerCh:       make(chan struct{}, 10),
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

	go so.processRetryQueue(ctx)

	batchChan := so.collector.BatchChannel()

	for {
		select {
		case <-ctx.Done():
			log.Info("StageOrchestrator shutting down")
			return nil
		case <-so.triggerCh:
			log.V(1).Info("Received reconciliation trigger, flushing pending events")
			so.collector.Flush()
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
	defer func() {
		if r := recover(); r != nil {
			log.Error(fmt.Errorf("panic in retry queue processor: %v", r), "Retry queue processor crashed, restarting")
			go so.processRetryQueue(ctx)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			log.Info("Retry queue processor shutting down")
			return
		case item := <-so.retryQueue:
			waitTime := time.Until(item.retryAfter)
			if waitTime > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(waitTime):
				}
			}

			select {
			case <-ctx.Done():
				return
			default:
				batchesRetriedTotal.Inc()
				so.processBatchWithRetry(ctx, item.batch)
			}
		}
	}
}

func (so *StageOrchestrator) processBatchWithRetry(ctx context.Context, batch *Batch) {
	err := so.processBatch(ctx, batch)

	if err != nil {
		batch.LastError = err
		batch.RetryCount++

		if batch.RetryCount <= so.maxRetries {
			retryDelay := so.retryDelay * time.Duration(1<<uint(batch.RetryCount-1))
			log.Info("Batch processing failed, queueing for retry",
				"batchID", batch.ID,
				"retryCount", batch.RetryCount,
				"maxRetries", so.maxRetries,
				"retryAfter", retryDelay,
				"error", err.Error())

			item := &retryItem{
				batch:      batch,
				retryAfter: time.Now().Add(retryDelay),
			}

			select {
			case so.retryQueue <- item:
				log.V(1).Info("Batch queued for retry", "batchID", batch.ID)
			case <-time.After(retryQueueSendTimeout):
				log.Error(nil, "Retry queue blocked, triggering state rebuild",
					"batchID", batch.ID,
					"timeout", retryQueueSendTimeout)
				retryQueueDroppedTotal.Inc()
				so.handlePermanentFailure(ctx, batch, fmt.Errorf("retry queue timeout after %v", retryQueueSendTimeout))
			case <-ctx.Done():
				log.Info("Context cancelled while queueing retry", "batchID", batch.ID)
			}
		} else {
			so.handlePermanentFailure(ctx, batch, err)
		}
	} else {
		batchesProcessed.Inc()
	}
}

func (so *StageOrchestrator) handlePermanentFailure(ctx context.Context, batch *Batch, err error) {
	log.Error(err, "Batch processing failed permanently",
		"batchID", batch.ID,
		"resourceCount", len(batch.Resources),
		"retryCount", batch.RetryCount)
	batchesFailedTotal.Inc()

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

		if so.collector.AddEvent(event) {
			requeued++
		}
	}

	if requeued > 0 {
		log.Info("Re-queued failed resources for reconciliation", "batchID", batch.ID, "requeued", requeued)
	}
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

	completedStages := make([]string, 0, len(so.stages))
	var failedStage string
	var stageErr error

	for _, stage := range so.stages {
		stageCtx, cancel := context.WithTimeout(ctx, so.stageTimeout)

		stageStart := time.Now()
		err := stage.Execute(stageCtx, batch)
		cancel()

		stageDuration.WithLabelValues(stage.Name()).Observe(time.Since(stageStart).Seconds())

		if err != nil {
			failedStage = stage.Name()
			stageErr = err
			break
		}

		completedStages = append(completedStages, stage.Name())
		log.V(1).Info("Stage completed", "stage", stage.Name(), "batchID", batch.ID, "duration", time.Since(stageStart))
	}

	so.updateFinalConditions(ctx, batch, completedStages, failedStage, stageErr)

	if stageErr != nil {
		return fmt.Errorf("stage %s failed: %w", failedStage, stageErr)
	}

	if batch.RequeueAfter > 0 {
		log.Info("Scheduling follow-up reconciliation", "batchID", batch.ID, "requeueAfter", batch.RequeueAfter)
		time.AfterFunc(batch.RequeueAfter, func() {
			so.triggerReconcile()
		})
	}

	log.Info("Batch processed successfully", "batchID", batch.ID, "totalDuration", time.Since(batchStart))
	return nil
}

func (so *StageOrchestrator) updateFinalConditions(ctx context.Context, batch *Batch, completedStages []string, failedStage string, err error) {
	for _, stageName := range completedStages {
		if conditionType := stageToConditionType[stageName]; conditionType != "" {
			so.updateConditions(ctx, batch, conditionType, metav1.ConditionTrue, ReasonSucceeded, "Complete")
		}
	}

	if failedStage != "" {
		if conditionType := stageToConditionType[failedStage]; conditionType != "" {
			so.updateConditions(ctx, batch, conditionType, metav1.ConditionFalse, ReasonFailed, err.Error())
		}
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

func (so *StageOrchestrator) updateConditions(ctx context.Context, batch *Batch, conditionType string, status metav1.ConditionStatus, reason, message string) {
	for _, res := range batch.Resources {
		key := types.NamespacedName{Namespace: res.GetNamespace(), Name: res.GetName()}
		obj := newWSAClientObject(res)

		if err := so.client.Get(ctx, key, obj); err != nil {
			log.V(1).Info("Failed to get resource for condition update", "key", key, "error", err)
			continue
		}

		var currentConditions []metav1.Condition
		switch o := obj.(type) {
		case *agentoctopuscomv1beta1.WorkloadServiceAccount:
			currentConditions = o.Status.Conditions
		case *agentoctopuscomv1beta1.ClusterWorkloadServiceAccount:
			currentConditions = o.Status.Conditions
		}

		newCondition := metav1.Condition{
			Type:               conditionType,
			Status:             status,
			Reason:             reason,
			Message:            message,
			ObservedGeneration: obj.GetGeneration(),
		}

		if conditionUnchanged(currentConditions, newCondition) {
			log.V(2).Info("Skipping condition update, no change", "key", key, "conditionType", conditionType)
			continue
		}

		var conditions *[]metav1.Condition
		switch o := obj.(type) {
		case *agentoctopuscomv1beta1.WorkloadServiceAccount:
			conditions = &o.Status.Conditions
		case *agentoctopuscomv1beta1.ClusterWorkloadServiceAccount:
			conditions = &o.Status.Conditions
		}

		meta.SetStatusCondition(conditions, newCondition)

		if err := so.client.Status().Update(ctx, obj); err != nil {
			log.V(1).Info("Failed to update condition", "key", key, "conditionType", conditionType, "error", err)
		}
	}
}

func conditionUnchanged(conditions []metav1.Condition, newCondition metav1.Condition) bool {
	for _, c := range conditions {
		if c.Type == newCondition.Type {
			return c.Status == newCondition.Status && c.Reason == newCondition.Reason
		}
	}
	return false
}

func newWSAClientObject(res rules.WSAResource) client.Object {
	if res.IsClusterScoped() {
		return &agentoctopuscomv1beta1.ClusterWorkloadServiceAccount{}
	}
	return &agentoctopuscomv1beta1.WorkloadServiceAccount{}
}
