package templateupdaterequest

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SingleTemplateUpdateRequestAssertion an assertion on a single TemplateUpdateRequest
type SingleTemplateUpdateRequestAssertion struct {
	templateUpdateRequest *toolchainv1alpha1.TemplateUpdateRequest
	client                client.Client
	namespacedName        types.NamespacedName
	t                     test.T
}

func (a *SingleTemplateUpdateRequestAssertion) loadResource() error {
	tur := &toolchainv1alpha1.TemplateUpdateRequest{}
	err := a.client.Get(context.TODO(), a.namespacedName, tur)
	a.templateUpdateRequest = tur
	if err != nil {
		a.t.Logf("unable to find TemplateUpdateRequest '%v'", a.namespacedName)
	}
	return err
}

// AssertThatTemplateUpdateRequest helper func to begin with the assertions on a single TemplateUpdateRequest
func AssertThatTemplateUpdateRequest(t test.T, name string, client client.Client) *SingleTemplateUpdateRequestAssertion {
	return &SingleTemplateUpdateRequestAssertion{
		client:         client,
		namespacedName: test.NamespacedName(test.HostOperatorNs, name),
		t:              t,
	}
}

// HasConditions verifies that the TemplateUpdateRequest has ALL the given conditions in its status
func (a *SingleTemplateUpdateRequestAssertion) HasConditions(expected ...toolchainv1alpha1.Condition) *SingleTemplateUpdateRequestAssertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	test.AssertConditionsMatch(a.t, a.templateUpdateRequest.Status.Conditions, expected...)
	return a
}

// HasSameConditions verifies that the TemplateUpdateRequest has ALL the given conditions in its status
// (even with duplicate types)
func (a *SingleTemplateUpdateRequestAssertion) HasSameConditions(expected ...toolchainv1alpha1.Condition) *SingleTemplateUpdateRequestAssertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	require.Equal(a.t, len(expected), len(a.templateUpdateRequest.Status.Conditions))
	for i, c := range expected {
		assert.Equal(a.t, c.Type, a.templateUpdateRequest.Status.Conditions[i].Type)
		assert.Equal(a.t, c.Status, a.templateUpdateRequest.Status.Conditions[i].Status)
		assert.Equal(a.t, c.Reason, a.templateUpdateRequest.Status.Conditions[i].Reason)
		assert.Equal(a.t, c.Message, a.templateUpdateRequest.Status.Conditions[i].Message)
	}
	return a
}

// HasSyncIndexes verifies that the TemplateUpdateRequest has the given sync indexes in its status
func (a *SingleTemplateUpdateRequestAssertion) HasSyncIndexes(indexes map[string]string) *SingleTemplateUpdateRequestAssertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.Equal(a.t, indexes, a.templateUpdateRequest.Status.SyncIndexes)
	return a
}

// Exists verifies that the TemplateUpdateRequest exists
func (a *SingleTemplateUpdateRequestAssertion) Exists() *SingleTemplateUpdateRequestAssertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.Nil(a.t, a.templateUpdateRequest.DeletionTimestamp) // resource is not even marked for deletion
	return a
}

// HasFinalizer verifies that the TemplateUpdateRequest has the given finalizer
func (a *SingleTemplateUpdateRequestAssertion) HasFinalizer(finalizer string) *SingleTemplateUpdateRequestAssertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.Contains(a.t, a.templateUpdateRequest.Finalizers, finalizer)
	return a
}

// HasNotFinalizer verifies that the TemplateUpdateRequest has NOT the given finalizer
func (a *SingleTemplateUpdateRequestAssertion) HasNotFinalizer(finalizer string) *SingleTemplateUpdateRequestAssertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.NotContains(a.t, a.templateUpdateRequest.Finalizers, finalizer)
	return a
}

// HasDeletionTimestamp verifies that the TemplateUpdateRequest has a deletion timestamp
func (a *SingleTemplateUpdateRequestAssertion) HasDeletionTimestamp() *SingleTemplateUpdateRequestAssertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.NotNil(a.t, a.templateUpdateRequest.DeletionTimestamp)
	return a
}

// DoesNotExist verifies that the TemplateUpdateRequest does not exist
func (a *SingleTemplateUpdateRequestAssertion) DoesNotExist() *SingleTemplateUpdateRequestAssertion {
	err := a.loadResource()
	require.Error(a.t, err)
	assert.IsType(a.t, metav1.StatusReasonNotFound, errors.ReasonForError(err))
	return a
}

// HasOwnerReference verifies that the TemplateUpdateRequest has an owner reference
func (a *SingleTemplateUpdateRequestAssertion) HasOwnerReference() *SingleTemplateUpdateRequestAssertion {
	err := a.loadResource()
	require.NoError(a.t, err)
	assert.IsType(a.t, metav1.StatusReasonNotFound, errors.ReasonForError(err))
	assert.NotEmpty(a.t, a.templateUpdateRequest.OwnerReferences)
	return a
}

// ------------------------------------------------------------------------------------

// MultipleTemplateUpdateRequestsAssertion an assertion on a set of TemplateUpdateRequests
type MultipleTemplateUpdateRequestsAssertion struct {
	templateUpdateRequests []toolchainv1alpha1.TemplateUpdateRequest
	client                 client.Client
	t                      test.T
}

func (a *MultipleTemplateUpdateRequestsAssertion) loadResources() error {
	templateUpdateRequests := toolchainv1alpha1.TemplateUpdateRequestList{}
	err := a.client.List(context.TODO(), &templateUpdateRequests)
	require.NoError(a.t, err)
	a.templateUpdateRequests = templateUpdateRequests.Items
	return err
}

// AssertThatTemplateUpdateRequests helper func to begin with the assertions on a multiple TemplateUpdateRequests
func AssertThatTemplateUpdateRequests(t test.T, client client.Client) *MultipleTemplateUpdateRequestsAssertion {
	return &MultipleTemplateUpdateRequestsAssertion{
		client: client,
		t:      t,
	}
}

// TotalCount verifies the TOTAL number of TemplateUpdateRequests in the cluster (no matter which namespace and NSTemplateTier owner)
func (a *MultipleTemplateUpdateRequestsAssertion) TotalCount(c int) *MultipleTemplateUpdateRequestsAssertion {
	err := a.loadResources()
	require.NoError(a.t, err)
	require.Len(a.t, a.templateUpdateRequests, c)
	return a
}
