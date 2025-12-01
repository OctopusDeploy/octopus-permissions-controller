package controller

import (
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"k8s.io/apimachinery/pkg/types"
)

// GCTrackerInterface defines the interface for tracking resources being garbage collected.
// This allows predicates to filter out delete events for resources the controller is actively removing.
type GCTrackerInterface interface {
	MarkForDeletion(uid types.UID)
	IsBeingDeleted(uid types.UID) bool
}

// GCTracker tracks resources being actively garbage collected by the controller.
// It uses an expirable LRU cache to automatically clean up entries after a TTL.
type GCTracker struct {
	cache *expirable.LRU[types.UID, struct{}]
}

// NewGCTracker creates a new GCTracker with the specified TTL for entries.
// The TTL should be long enough to cover the delay between the delete call
// and the watch event being processed by the predicate.
func NewGCTracker(ttl time.Duration) *GCTracker {
	cache := expirable.NewLRU[types.UID, struct{}](0, nil, ttl)
	return &GCTracker{cache: cache}
}

// MarkForDeletion records that a resource with the given UID is being deleted by the controller.
// This should be called immediately before the delete operation.
func (t *GCTracker) MarkForDeletion(uid types.UID) {
	t.cache.Add(uid, struct{}{})
}

// IsBeingDeleted returns true if the resource with the given UID is being actively
// deleted by the controller (was recently marked for deletion and hasn't expired).
func (t *GCTracker) IsBeingDeleted(uid types.UID) bool {
	_, exists := t.cache.Get(uid)
	return exists
}
