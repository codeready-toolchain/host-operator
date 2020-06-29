package templateupdaterequest

import (
	"fmt"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// NewTemplateUpdateRequest creates a specified number of TemplateRequestUpdate objects, with options
func NewTemplateUpdateRequest(name string, tier toolchainv1alpha1.NSTemplateTier, options ...Option) *toolchainv1alpha1.TemplateUpdateRequest {
	tur := &toolchainv1alpha1.TemplateUpdateRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: test.HostOperatorNs,
			Labels: map[string]string{
				toolchainv1alpha1.NSTemplateTierNameLabelKey: tier.Name,
			},
		},
		Spec: toolchainv1alpha1.TemplateUpdateRequestSpec{
			TierName:         tier.Name,
			Namespaces:       tier.Spec.Namespaces,
			ClusterResources: tier.Spec.ClusterResources,
		},
		Status: toolchainv1alpha1.TemplateUpdateRequestStatus{},
	}
	for _, opt := range options {
		opt.applyToTemplateUpdateRequest(tur)
	}
	return tur
}

// NewTemplateUpdateRequests creates a specified number of TemplateRequestUpdate objects, with options
func NewTemplateUpdateRequests(size int, nameFmt string, tier toolchainv1alpha1.NSTemplateTier, options ...Option) []runtime.Object {
	templateUpdateRequests := make([]runtime.Object, size)
	for i := 0; i < size; i++ {
		templateUpdateRequests[i] = NewTemplateUpdateRequest(fmt.Sprintf(nameFmt, i), tier, options...)
	}
	return templateUpdateRequests
}

// Option an option to configure a TemplateUpdateRequest
type Option interface {
	applyToTemplateUpdateRequest(*toolchainv1alpha1.TemplateUpdateRequest)
}

// DeletionTimestamp sets a deletion timestamp on the TemplateUpdateRequest with the given name
type DeletionTimestamp string

var _ Option = DeletionTimestamp("")

func (d DeletionTimestamp) applyToTemplateUpdateRequest(r *toolchainv1alpha1.TemplateUpdateRequest) {
	if r.Name == string(d) {
		deletionTS := metav1.NewTime(time.Now())
		r.DeletionTimestamp = &deletionTS
	}
}

// Complete sets the status condition to "complete" on the TemplateUpdateRequest with the given name
type Complete string

var _ Option = Complete("")

func (c Complete) applyToTemplateUpdateRequest(r *toolchainv1alpha1.TemplateUpdateRequest) {
	if r.Name == string(c) {
		r.Status.Conditions = []toolchainv1alpha1.Condition{
			{
				Type:               toolchainv1alpha1.TemplateUpdateRequestComplete,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(time.Now()),
			},
		}
	}
}

// CompleteFor sets the status condition to "complete" on the TemplateUpdateRequest, using the given duration as an offset to time.Now
type CompleteFor time.Duration

var _ Option = CompleteFor(time.Second)

func (c CompleteFor) applyToTemplateUpdateRequest(r *toolchainv1alpha1.TemplateUpdateRequest) {
	r.Status.Conditions = []toolchainv1alpha1.Condition{
		{
			Type:               toolchainv1alpha1.TemplateUpdateRequestComplete,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(time.Now().Add(-time.Duration(c))),
		},
	}
}

// Failed sets the status condition to "failed" on the TemplateUpdateRequest with the given name
type Failed string

var _ Option = Failed("")

func (f Failed) applyToTemplateUpdateRequest(r *toolchainv1alpha1.TemplateUpdateRequest) {
	if r.Name == string(f) {
		r.Status.Conditions = []toolchainv1alpha1.Condition{
			{
				Type:   toolchainv1alpha1.TemplateUpdateRequestComplete,
				Status: corev1.ConditionFalse,
				Reason: toolchainv1alpha1.TemplateUpdateRequestFailedReason,
			},
		}
	}
}

// TierName sets the name of the tier that was updated
type TierName string

var _ Option = TierName("")

func (t TierName) applyToTemplateUpdateRequest(r *toolchainv1alpha1.TemplateUpdateRequest) {
	r.Spec.TierName = string(t)
	r.Labels = map[string]string{
		toolchainv1alpha1.NSTemplateTierNameLabelKey: string(t),
	}
}

// SyncIndexes the Sync Indexes stored in the status before the MasterUserRecord update begins
type SyncIndexes map[string]string

var _ Option = SyncIndexes(map[string]string{})

func (i SyncIndexes) applyToTemplateUpdateRequest(r *toolchainv1alpha1.TemplateUpdateRequest) {
	r.Status.SyncIndexes = map[string]string(i)
}

// Condition adds a condition in the TemplateUpdateRequest status
type Condition toolchainv1alpha1.Condition

var _ Option = Condition(toolchainv1alpha1.Condition{})

func (c Condition) applyToTemplateUpdateRequest(r *toolchainv1alpha1.TemplateUpdateRequest) {
	if r.Status.Conditions == nil {
		r.Status.Conditions = []toolchainv1alpha1.Condition{}
	}
	r.Status.Conditions = append(r.Status.Conditions, toolchainv1alpha1.Condition(c))
}
