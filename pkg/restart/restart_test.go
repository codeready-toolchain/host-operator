package restart

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

type MockManager struct {
	startFn                   func()
	afterRun                  func()
	afterContextCancelledFn   func()
	waitUntilContextCancelled bool
	errorToReturn             error
}

func TestStartManager(t *testing.T) {
	t.Run("returns no error when manager returns with no error after context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		startManager := &StartManager{InitializeManager: func(context.Context) (manager.Manager, error) {
			return &MockManager{waitUntilContextCancelled: true}, nil
		}}

		doneCh := make(chan struct{})
		returnedError := errors.New("unexpected error")
		go func() {
			returnedError = startManager.Start(ctx)
			doneCh <- struct{}{}
		}()

		cancel()

		<-doneCh

		assert.NoError(t, returnedError)
	})

	t.Run("returns error when manager fails to start", func(t *testing.T) {
		errToReturn := errors.New("an error")
		startManager := &StartManager{InitializeManager: func(context.Context) (manager.Manager, error) {
			return &MockManager{errorToReturn: errToReturn}, nil
		}}

		assert.Same(t, errToReturn, startManager.Start(context.TODO()))
	})

	t.Run("manager can be restarted", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		doneCh := make(chan struct{})
		startCh := make(chan struct{})
		startManager := &StartManager{InitializeManager: func(context.Context) (manager.Manager, error) {
			return &MockManager{
				waitUntilContextCancelled: true,
				startFn: func() {
					startCh <- struct{}{}
				},
				afterContextCancelledFn: func() {
					doneCh <- struct{}{}
				},
			}, nil
		}}

		waitForResult := make(chan struct{})
		returnedError := errors.New("unexpected error")
		go func() {
			returnedError = startManager.Start(ctx)
			waitForResult <- struct{}{}
		}()

		// wait until we know the manager is running
		<-startCh

		// we check that the manager restarted by the virtue of receiving a message to the startCh channel and checking
		// that doneCh receives the message due to the cancellation of the context.
		startManager.RestartNeeded()
		<-doneCh
		<-startCh

		startManager.RestartNeeded()
		<-doneCh
		<-startCh

		startManager.RestartNeeded()
		<-doneCh
		<-startCh

		assert.Empty(t, doneCh)

		cancel()
		<-doneCh

		<-waitForResult
		assert.Empty(t, doneCh)
		assert.Empty(t, startCh)
		assert.NoError(t, returnedError)
	})
}

// Add implements manager.Manager.
func (m *MockManager) Add(manager.Runnable) error {
	return nil
}

// AddHealthzCheck implements manager.Manager.
func (m *MockManager) AddHealthzCheck(name string, check healthz.Checker) error {
	return nil
}

// AddMetricsExtraHandler implements manager.Manager.
func (m *MockManager) AddMetricsExtraHandler(path string, handler http.Handler) error {
	return nil
}

// AddReadyzCheck implements manager.Manager.
func (m *MockManager) AddReadyzCheck(name string, check healthz.Checker) error {
	return nil
}

// Elected implements manager.Manager.
func (m *MockManager) Elected() <-chan struct{} {
	return nil
}

// GetAPIReader implements manager.Manager.
func (m *MockManager) GetAPIReader() client.Reader {
	return nil
}

// GetCache implements manager.Manager.
func (m *MockManager) GetCache() cache.Cache {
	return nil
}

// GetClient implements manager.Manager.
func (m *MockManager) GetClient() client.Client {
	return nil
}

// GetConfig implements manager.Manager.
func (m *MockManager) GetConfig() *rest.Config {
	return nil
}

// GetControllerOptions implements manager.Manager.
func (m *MockManager) GetControllerOptions() config.Controller {
	return config.Controller{}
}

// GetEventRecorderFor implements manager.Manager.
func (m *MockManager) GetEventRecorderFor(name string) record.EventRecorder {
	return nil
}

// GetFieldIndexer implements manager.Manager.
func (m *MockManager) GetFieldIndexer() client.FieldIndexer {
	return nil
}

// GetHTTPClient implements manager.Manager.
func (m *MockManager) GetHTTPClient() *http.Client {
	return nil
}

// GetLogger implements manager.Manager.
func (m *MockManager) GetLogger() logr.Logger {
	return logr.Logger{}
}

// GetRESTMapper implements manager.Manager.
func (m *MockManager) GetRESTMapper() meta.RESTMapper {
	return nil
}

// GetScheme implements manager.Manager.
func (m *MockManager) GetScheme() *runtime.Scheme {
	return nil
}

// GetWebhookServer implements manager.Manager.
func (m *MockManager) GetWebhookServer() webhook.Server {
	return nil
}

// AddMetricsServerExtraHandler implements manager.Manager.
func (m *MockManager) AddMetricsServerExtraHandler(path string, handler http.Handler) error {
	return nil
}

// Start implements manager.Manager.
func (m *MockManager) Start(ctx context.Context) error {
	defer func() {
		if m.afterRun != nil {
			m.afterRun()
		}
	}()

	if m.startFn != nil {
		m.startFn()
	}

	if m.waitUntilContextCancelled {
		<-ctx.Done()
		if m.afterContextCancelledFn != nil {
			m.afterContextCancelledFn()
		}
		// controller manager never returns an error after context cancellation.
		return nil
	}
	return m.errorToReturn
}

var _ manager.Manager = (*MockManager)(nil)
