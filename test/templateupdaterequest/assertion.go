package templateupdaterequest

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Assertion struct {
	templateUpdateRequest *toolchainv1alpha1.TemplateUpdateRequest
	client                client.Client
	namespacedName        types.NamespacedName
	t                     test.T
}

func (a *Assertion) loadUaAssertion() error {
	tur := &toolchainv1alpha1.TemplateUpdateRequest{}
	err := a.client.Get(context.TODO(), a.namespacedName, tur)
	a.templateUpdateRequest = tur
	return err
}

func AssertThatTemplateUpdateRequest(t test.T, name string, client client.Client) *Assertion {
	return &Assertion{
		client:         client,
		namespacedName: test.NamespacedName(test.HostOperatorNs, name),
		t:              t,
	}
}

func (a *Assertion) HasConditions(expected ...toolchainv1alpha1.Condition) *Assertion {
	err := a.loadUaAssertion()
	require.NoError(a.t, err)
	test.AssertConditionsMatch(a.t, a.templateUpdateRequest.Status.Conditions, expected...)
	return a
}

func (a *Assertion) HasSyncIndexes(indexes map[string]string) *Assertion {
	err := a.loadUaAssertion()
	require.NoError(a.t, err)
	assert.Equal(a.t, indexes, a.templateUpdateRequest.Status.SyncIndexes)
	return a
}

func (a *Assertion) Exists() *Assertion {
	err := a.loadUaAssertion()
	require.NoError(a.t, err)
	return a
}

func (a *Assertion) DoesNotExist() *Assertion {
	err := a.loadUaAssertion()
	require.Error(a.t, err)
	assert.IsType(a.t, metav1.StatusReasonNotFound, errors.ReasonForError(err))
	return a
}
