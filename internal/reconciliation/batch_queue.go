package reconciliation

import (
	"sync"

	"k8s.io/client-go/util/workqueue"
)

type BatchQueue interface {
	Add(batch *Batch)
	AddRateLimited(batch *Batch)
	Get() (*Batch, bool)
	Done(batch *Batch)
	Forget(batch *Batch)
	NumRequeues(batch *Batch) int
	ShutDown()
}

type batchQueue struct {
	queue   workqueue.TypedRateLimitingInterface[BatchID]
	batches map[BatchID]*Batch
	mu      sync.RWMutex
}

func NewBatchQueue() BatchQueue {
	return &batchQueue{
		queue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[BatchID](),
			workqueue.TypedRateLimitingQueueConfig[BatchID]{
				Name: "batch-retry",
			},
		),
		batches: make(map[BatchID]*Batch),
	}
}

func (q *batchQueue) Add(batch *Batch) {
	if batch == nil {
		return
	}
	q.mu.Lock()
	q.batches[batch.ID] = batch
	q.mu.Unlock()
	q.queue.Add(batch.ID)
}

func (q *batchQueue) AddRateLimited(batch *Batch) {
	if batch == nil {
		return
	}
	q.mu.Lock()
	q.batches[batch.ID] = batch
	q.mu.Unlock()
	q.queue.AddRateLimited(batch.ID)
}

func (q *batchQueue) Get() (*Batch, bool) {
	id, shutdown := q.queue.Get()
	if shutdown {
		return nil, true
	}

	q.mu.RLock()
	batch, ok := q.batches[id]
	q.mu.RUnlock()

	if !ok {
		q.queue.Done(id)
		q.queue.Forget(id)
		return nil, false
	}

	return batch, false
}

func (q *batchQueue) Done(batch *Batch) {
	if batch == nil {
		return
	}
	q.queue.Done(batch.ID)
}

func (q *batchQueue) Forget(batch *Batch) {
	if batch == nil {
		return
	}
	q.mu.Lock()
	delete(q.batches, batch.ID)
	q.mu.Unlock()
	q.queue.Forget(batch.ID)
}

func (q *batchQueue) NumRequeues(batch *Batch) int {
	if batch == nil {
		return 0
	}
	return q.queue.NumRequeues(batch.ID)
}

func (q *batchQueue) ShutDown() {
	q.queue.ShutDown()
}
