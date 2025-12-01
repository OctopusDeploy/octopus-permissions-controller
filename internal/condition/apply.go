package condition

import (
	"context"
	"encoding/json"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Apply(
	ctx context.Context,
	c client.Client,
	obj client.Object,
	conditions *[]metav1.Condition,
	conditionType string,
	status metav1.ConditionStatus,
	reason, message string,
) error {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: obj.GetGeneration(),
	})

	gvk := obj.GetObjectKind().GroupVersionKind()
	patch := map[string]interface{}{
		"apiVersion": gvk.GroupVersion().String(),
		"kind":       gvk.Kind,
		"metadata":   map[string]interface{}{"name": obj.GetName(), "namespace": obj.GetNamespace()},
		"status":     map[string]interface{}{"conditions": *conditions},
	}

	data, err := json.Marshal(patch)
	if err != nil {
		return err
	}

	return c.Status().Patch(ctx, obj, client.RawPatch(types.ApplyPatchType, data),
		client.ForceOwnership, client.FieldOwner("octopus-permissions-controller"))
}
