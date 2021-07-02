package notification

import (
	"crypto/md5"
	"encoding/hex"
	"testing"

	"github.com/codeready-toolchain/toolchain-common/pkg/states"

	"github.com/stretchr/testify/assert"

	"github.com/codeready-toolchain/host-operator/pkg/configuration"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/apis"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
)

const (
	operatorNamespace = "toolchain-host-operator"
)

func TestNotificationContext(t *testing.T) {
	// given
	userSignup := &toolchainv1alpha1.UserSignup{
		ObjectMeta: newObjectMeta("john", "jsmith@redhat.com"),
		Spec: toolchainv1alpha1.UserSignupSpec{
			Username:      "jsmith@redhat.com",
			TargetCluster: "east",
			FamilyName:    "Smith",
			GivenName:     "John",
		},
	}
	states.SetApproved(userSignup, true)
	client := prepareReconcile(t, userSignup)
	config, err := configuration.LoadConfig(test.NewFakeClient(t))
	require.NoError(t, err)

	t.Run("user found", func(t *testing.T) {
		// when
		notificationCtx, err := NewUserNotificationContext(client, userSignup.Name, operatorNamespace, config)

		// then
		require.NoError(t, err)

		require.Equal(t, userSignup.Name, notificationCtx.UserID)
		require.Equal(t, userSignup.Spec.GivenName, notificationCtx.FirstName)
		require.Equal(t, userSignup.Spec.FamilyName, notificationCtx.LastName)
		require.Equal(t, userSignup.Annotations[toolchainv1alpha1.UserSignupUserEmailAnnotationKey], notificationCtx.UserEmail)
		require.Equal(t, userSignup.Spec.Company, notificationCtx.CompanyName)
		require.Equal(t, "https://registration.crt-placeholder.com", notificationCtx.RegistrationURL)
	})

	t.Run("user not found", func(t *testing.T) {
		// when
		_, err := NewUserNotificationContext(client, "other", operatorNamespace, config)

		// then
		require.Error(t, err)
		require.Equal(t, "usersignups.toolchain.dev.openshift.com \"other\" not found", err.Error())
	})

	t.Run("email address correct", func(t *testing.T) {
		// when
		notificationCtx, err := NewUserNotificationContext(client, userSignup.Name, operatorNamespace, config)

		// then
		require.NoError(t, err)
		require.Equal(t, "jsmith@redhat.com", notificationCtx.DeliveryEmail())
	})

	t.Run("no configuration provided", func(t *testing.T) {
		// when
		_, err := NewUserNotificationContext(client, userSignup.Name, operatorNamespace, nil)

		// then
		require.Error(t, err)
		assert.Equal(t, "configuration was not provided", err.Error())
	})
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

func prepareReconcile(t *testing.T, initObjs ...runtime.Object) *test.FakeClient {
	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	return test.NewFakeClient(t, initObjs...)
}
