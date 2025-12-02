package reconciliation

import (
	"time"

	"github.com/google/uuid"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type BatchID uuid.UUID

func NewBatchID() BatchID {
	return BatchID(uuid.New())
}

func (id BatchID) String() string {
	return uuid.UUID(id).String()
}

type EventType string

const (
	EventTypeCreate EventType = "Create"
	EventTypeUpdate EventType = "Update"
	EventTypeDelete EventType = "Delete"
)

type EventInfo struct {
	Resource        rules.WSAResource
	EventType       EventType
	Generation      int64
	ResourceVersion string
	Timestamp       time.Time
}

func NewEventInfo(resource rules.WSAResource, eventType EventType) *EventInfo {
	return &EventInfo{
		Resource:        resource,
		EventType:       eventType,
		Generation:      resource.GetGeneration(),
		ResourceVersion: resource.GetResourceVersion(),
		Timestamp:       time.Now(),
	}
}

func NewCreateOrUpdateEventInfo(resource rules.WSAResource) *EventInfo {
	eventType := EventTypeUpdate
	if resource.GetGeneration() == 1 {
		eventType = EventTypeCreate
	}

	return NewEventInfo(resource, eventType)
}

type Batch struct {
	ID               BatchID
	Resources        []rules.WSAResource
	StartTime        time.Time
	Plan             *Plan
	ValidationResult *ValidationResult
	LastError        error
	RequeueAfter     time.Duration
}

func NewBatch(events []*EventInfo) *Batch {
	resources := make([]rules.WSAResource, len(events))
	for i, event := range events {
		resources[i] = event.Resource
	}

	return &Batch{
		ID:        NewBatchID(),
		Resources: resources,
		StartTime: time.Now(),
	}
}

type Plan struct {
	ScopeToSA        map[rules.Scope]rules.ServiceAccountName
	SAToWSAMap       map[rules.ServiceAccountName]map[types.NamespacedName]rules.WSAResource
	WSAToSANames     map[types.NamespacedName][]rules.ServiceAccountName
	Vocabulary       *rules.GlobalVocabulary
	UniqueAccounts   []*v1.ServiceAccount
	AllResources     []rules.WSAResource
	TargetNamespaces []string
}

func (rp *Plan) GetScopeToSA() map[rules.Scope]rules.ServiceAccountName {
	return rp.ScopeToSA
}

func (rp *Plan) GetSAToWSAMap() map[rules.ServiceAccountName]map[types.NamespacedName]rules.WSAResource {
	return rp.SAToWSAMap
}

func (rp *Plan) GetVocabulary() *rules.GlobalVocabulary {
	return rp.Vocabulary
}

type ValidationResult struct {
	Valid    bool
	Errors   []ValidationError
	Warnings []ValidationWarning
}

func NewValidationResult() *ValidationResult {
	return &ValidationResult{
		Valid:    true,
		Errors:   []ValidationError{},
		Warnings: []ValidationWarning{},
	}
}

func (vr *ValidationResult) AddError(errorType, resource, message string) {
	vr.Valid = false
	vr.Errors = append(vr.Errors, ValidationError{
		Type:     errorType,
		Resource: resource,
		Message:  message,
	})
}

func (vr *ValidationResult) AddWarning(warningType, resource, message string) {
	vr.Warnings = append(vr.Warnings, ValidationWarning{
		Type:     warningType,
		Resource: resource,
		Message:  message,
	})
}

type ValidationError struct {
	Type     string
	Resource string
	Message  string
}

type ValidationWarning struct {
	Type     string
	Resource string
	Message  string
}
