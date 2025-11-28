package condition

import (
	"context"
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Updatable[T client.Object] interface {
	GetConditions() []metav1.Condition
	SetConditions([]metav1.Condition)
	SetResourceVersion(string)
	GetAPIVersion() string
	GetKind() string
	GetObject() T
}

func Apply[T client.Object](
	ctx context.Context,
	c client.Client,
	res Updatable[T],
	conditionType string,
	status metav1.ConditionStatus,
	reason, message string,
) error {
	conditions := res.GetConditions()
	newCondition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}

	updated := false
	for i, cond := range conditions {
		if cond.Type == conditionType {
			if cond.Status == status && cond.Reason == reason {
				newCondition.LastTransitionTime = cond.LastTransitionTime
			}
			conditions[i] = newCondition
			updated = true
			break
		}
	}
	if !updated {
		conditions = append(conditions, newCondition)
	}

	obj := res.GetObject()
	patch := map[string]interface{}{
		"apiVersion": res.GetAPIVersion(),
		"kind":       res.GetKind(),
		"metadata":   map[string]interface{}{"name": obj.GetName(), "namespace": obj.GetNamespace()},
		"status":     map[string]interface{}{"conditions": conditions},
	}

	data, err := json.Marshal(patch)
	if err != nil {
		return err
	}

	if err := c.Status().Patch(ctx, obj, client.RawPatch(types.ApplyPatchType, data),
		client.ForceOwnership, client.FieldOwner("octopus-permissions-controller")); err != nil {
		return err
	}

	res.SetConditions(conditions)
	res.SetResourceVersion(obj.GetResourceVersion())
	return nil
}
