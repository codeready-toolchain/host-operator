package predicate

import (
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestToolchainConditionChangedPredicate(t *testing.T) {
	// given
	blueprint := &toolchainv1alpha1.ToolchainCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tc",
			Namespace: test.HostOperatorNs,
		},
		Spec: toolchainv1alpha1.ToolchainClusterSpec{
			SecretRef: toolchainv1alpha1.LocalSecretReference{
				Name: "secret",
			},
		},
		Status: toolchainv1alpha1.ToolchainClusterStatus{
			APIEndpoint:       "https://endpoint",
			OperatorNamespace: "member-op-ns",
			Conditions: []toolchainv1alpha1.Condition{
				{
					Type:    toolchainv1alpha1.ConditionReady,
					Status:  corev1.ConditionTrue,
					Reason:  "because",
					Message: "I CAN",
				},
			},
		},
	}

	predicate := ToolchainConditionChanged{
		GetConditions: func(o client.Object) []toolchainv1alpha1.Condition {
			return o.(*toolchainv1alpha1.ToolchainCluster).Status.Conditions
		},
		Type: toolchainv1alpha1.ConditionReady,
	}

	test := func(t *testing.T, expectedResult bool, updateObjects func(old, new *toolchainv1alpha1.ToolchainCluster)) {
		t.Helper()

		// given
		tcOld := blueprint.DeepCopy()
		tcNew := blueprint.DeepCopy()

		updateObjects(tcOld, tcNew)

		ev := event.UpdateEvent{
			ObjectOld: tcOld,
			ObjectNew: tcNew,
		}

		t.Run("update", func(t *testing.T) {
			// when
			result := predicate.Update(ev)

			// then
			assert.Equal(t, expectedResult, result)
		})

		t.Run("create", func(t *testing.T) {
			// when
			result := predicate.Create(event.CreateEvent{Object: tcNew})

			// then
			assert.True(t, result)
		})

		t.Run("delete", func(t *testing.T) {
			// when
			result := predicate.Delete(event.DeleteEvent{Object: tcNew})

			// then
			assert.True(t, result)
		})

		t.Run("generic", func(t *testing.T) {
			// when
			result := predicate.Generic(event.GenericEvent{Object: tcNew})

			// then
			assert.True(t, result)
		})
	}

	t.Run("ignores no change", func(t *testing.T) {
		test(t, false, func(old, new *toolchainv1alpha1.ToolchainCluster) {})
	})

	t.Run("ignores changes in the object meta", func(t *testing.T) {
		test(t, false, func(old, new *toolchainv1alpha1.ToolchainCluster) {
			new.Annotations = map[string]string{"k": "v"}
		})
	})

	t.Run("ignores changes in the spec", func(t *testing.T) {
		test(t, false, func(old, new *toolchainv1alpha1.ToolchainCluster) {
			new.Spec.SecretRef.Name = "other"
		})
	})

	t.Run("ignores changes in the status outside of conditions", func(t *testing.T) {
		test(t, false, func(old, new *toolchainv1alpha1.ToolchainCluster) {
			new.Status.OperatorNamespace = "other"
		})
	})

	t.Run("ignores change in other condition types", func(t *testing.T) {
		test(t, false, func(old, new *toolchainv1alpha1.ToolchainCluster) {
			new.Status.Conditions = append(new.Status.Conditions, toolchainv1alpha1.Condition{Type: toolchainv1alpha1.ConditionType("other")})
		})
	})

	t.Run("ignores changes in the last transition time", func(t *testing.T) {
		test(t, false, func(old, new *toolchainv1alpha1.ToolchainCluster) {
			new.Status.Conditions[0].LastTransitionTime = metav1.Now()
		})
	})

	t.Run("ignores changes in the updated time", func(t *testing.T) {
		test(t, false, func(old, new *toolchainv1alpha1.ToolchainCluster) {
			time := metav1.Now()
			new.Status.Conditions[0].LastUpdatedTime = &time
		})
	})

	t.Run("detects when condition appears", func(t *testing.T) {
		test(t, true, func(old, new *toolchainv1alpha1.ToolchainCluster) {
			new.Status.Conditions = old.Status.Conditions
			old.Status.Conditions = nil
		})
	})
	t.Run("ignores missing conditions", func(t *testing.T) {
		test(t, false, func(old, new *toolchainv1alpha1.ToolchainCluster) {
			new.Status.Conditions = nil
			old.Status.Conditions = nil
		})
	})
	t.Run("detects change when condition disappears", func(t *testing.T) {
		test(t, true, func(old, new *toolchainv1alpha1.ToolchainCluster) {
			new.Status.Conditions = nil
		})
	})

	t.Run("detects change in the status", func(t *testing.T) {
		test(t, true, func(old, new *toolchainv1alpha1.ToolchainCluster) {
			new.Status.Conditions[0].Status = corev1.ConditionFalse
		})
	})

	t.Run("detects change in the reason", func(t *testing.T) {
		test(t, true, func(old, new *toolchainv1alpha1.ToolchainCluster) {
			new.Status.Conditions[0].Reason = "other"
		})
	})

	t.Run("detects change in the message", func(t *testing.T) {
		test(t, true, func(old, new *toolchainv1alpha1.ToolchainCluster) {
			new.Status.Conditions[0].Message = "other"
		})
	})
}
