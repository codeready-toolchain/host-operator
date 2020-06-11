package notification

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"testing"
)

const (
	operatorNamespace = "toolchain-host-operator"
)

func TestNotificationContextExtractedFromUserSignupOk(t *testing.T) {
	// given
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: newObjectMeta("john", "jsmith@redhat.com"),
		Spec: v1alpha1.UserSignupSpec{
			Username:      "jsmith@redhat.com",
			Approved:      true,
			TargetCluster: "east",
			FamilyName:    "Smith",
			GivenName:     "John",
		},
	}

	client := prepareReconcile(t, userSignup.Name, userSignup)

	// when
	notificationCtx, err := NewNotificationContext(context.Background(), client, userSignup.Name, operatorNamespace)

	// then
	require.NoError(t, err)

	require.Equal(t, userSignup.Name, notificationCtx.UserID)
	require.Equal(t, userSignup.Spec.GivenName, notificationCtx.FirstName)
	require.Equal(t, userSignup.Spec.FamilyName, notificationCtx.LastName)
	require.Equal(t, userSignup.Annotations[toolchainv1alpha1.UserSignupUserEmailAnnotationKey], notificationCtx.Email)
	require.Equal(t, userSignup.Spec.Company, notificationCtx.CompanyName)
}

func TestNotificationContextNoUserSignupFails(t *testing.T) {
	// given
	userSignup := &v1alpha1.UserSignup{
		ObjectMeta: newObjectMeta("john", "jsmith@redhat.com"),
		Spec: v1alpha1.UserSignupSpec{
			Username:      "jsmith@redhat.com",
			Approved:      true,
			TargetCluster: "east",
			FamilyName:    "Smith",
			GivenName:     "John",
		},
	}

	client := prepareReconcile(t, userSignup.Name, userSignup)

	// when
	_, err := NewNotificationContext(context.Background(), client, "other", operatorNamespace)

	// then
	require.Error(t, err)
}

func newObjectMeta(name, email string) metav1.ObjectMeta {
	if name == "" {
		name = uuid.NewV4().String()
	}

	md5hash := md5.New()
	// Ignore the error, as this implementation cannot return one
	_, _ = md5hash.Write([]byte(email))
	emailHash := hex.EncodeToString(md5hash.Sum(nil))

	return metav1.ObjectMeta{
		Name:      name,
		Namespace: operatorNamespace,
		Annotations: map[string]string{
			toolchainv1alpha1.UserSignupUserEmailAnnotationKey: email,
		},
		Labels: map[string]string{
			toolchainv1alpha1.UserSignupUserEmailHashLabelKey: emailHash,
		},
	}
}

// ReconcileUserSignup reconciles a UserSignup object
type ReconcileUserSignup struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

func prepareReconcile(t *testing.T, name string, initObjs ...runtime.Object) *test.FakeClient {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)

	initObjs = append(initObjs)

	client := test.NewFakeClient(t, initObjs...)

	return client
}
