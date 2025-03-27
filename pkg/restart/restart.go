package restart

import (
	"context"
	"errors"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	errRestartNeeded  = errors.New("restart needed called")
	errAlreadyRunning = errors.New("controller manager already running")

	_ Restarter = (*StartManager)(nil)
)

type InitializeManagerFunc func(context.Context) (manager.Manager, error)

// Restarter is a thing that can be told that a restart is needed.
// The StartManager is the sole implementation at runtime, but this interface
// should be used at use-sites to help with unit testing that a restart was requested.
type Restarter interface {
	RestartNeeded()
}

// StartManager manages the lifecycle of the controller manager.
// It it supplied a function that initializes the manager (and which MUST NOT start it).
// StartManager then can be used to start the controller manager and can also
// be asked to restart it.
type StartManager struct {
	InitializeManager InitializeManagerFunc

	running        bool
	lock           sync.Mutex
	runningContext context.Context
	cancelFunc     context.CancelCauseFunc
}

func debug(message string) {
	// fmt.Println(message)
}

func (s *StartManager) Start(ctx context.Context) error {
	for {
		debug("Start: start loop enter")
		startErr, finished := s.doStart(ctx)

		if s.runningContext == nil {
			// this actually means that the start failed, but we need a non-nil value for the runningContext
			// so that we can select on it below. We need to reach the select so that we can "catch" the potential
			// startErr.
			s.runningContext = context.TODO()
		}

		debug("Start: selecting on cancellation and error channels")
		select {
		case <-s.runningContext.Done():
			{
				debug("Start: context cancelled, waiting for finish signal")
				// The context is cancelled so the manager is shutting down. Let's wait for it to actually finish running before allowing
				// ourselves to continue and eventually allowing another round of manager run.
				<-finished

				debug("Start: context cancelled, finish signal received")

				// the manager is not running anymore, now we can be sure of it. Let's record that fact in our state to keep the records
				// straight.
				debug("Start: context cancelled, about to set running=false")
				s.lock.Lock()
				debug("Start: locked")
				s.running = false
				s.lock.Unlock()
				debug("Start: unlocked")
				debug("Start: context cancelled, running=false is set")

				// now, let's see what caused the cancellation

				if !errors.Is(context.Cause(s.runningContext), errRestartNeeded) {
					// this can only happen if the passed-in context is cancellable and is itself cancelled.
					// In this case, the error, if any, is retrievable from the context and therefore should NOT be returned
					// from this function.
					debug("Start: context cancelled, cancellation from above, quitting")
					return nil
				}

				// The conext has been cancelled and therefore the controller manager gracefully shut down.
				// We actually waited for the start function to finish and cleared our running state.
				// We also detected that the restart is needed by inspecting the cancellation cause.
				//
				// All the conditions for the restart are satisfied, so we can just start another loop that will start the manager
				// anew.
				debug("Start: context cancelled, restart request detected, entering another start loop")
			}
		case err := <-startErr:
			debug("Start: error received, quitting")
			return err
		}
	}
}

func (s *StartManager) doStart(ctx context.Context) (<-chan error, <-chan struct{}) {
	debug("doStart: enter")
	s.lock.Lock()
	defer func() {
		s.lock.Unlock()
		debug("doStart: unlocked")
	}()

	debug("doStart: locked")

	s.runningContext = nil
	s.cancelFunc = nil

	// we cannot guarantee the order in which the caller will wait for these channels vs when the channels are pushed to
	// in this method (because we start the manager in a co-routine, or, actually, when the error is sent to before the
	// control flow returns to the caller).
	// We can guarantee, that the caller will eventually expect a message from these channels.
	// Therefore, make the channels with a buffer of size 1, so that we can send a message here before the caller
	// starts listening.
	err := make(chan error, 1)
	finished := make(chan struct{}, 1)

	if s.running {
		debug("doStart: already running")
		err <- errAlreadyRunning
		return err, finished
	}

	s.runningContext, s.cancelFunc = context.WithCancelCause(ctx)

	debug("doStart: calling InitializeManager")
	mgr, initErr := s.InitializeManager(s.runningContext)
	if initErr != nil {
		debug("doStart: InitializeManager failed")
		err <- initErr
		return err, finished
	}
	go func() {
		debug("doStart: about to invoke mgr.Start()")
		if startErr := mgr.Start(s.runningContext); startErr != nil {
			debug("doStart: mgr.Start() returned error. Sending it.")
			err <- startErr
		}
		debug("doStart: sending finished message")
		finished <- struct{}{}
	}()

	debug("doStart: setting running=true")
	s.running = true
	return err, finished
}

func (s *StartManager) RestartNeeded() {
	go func() {
		debug("restartNeeded: enter")
		s.lock.Lock()
		defer func() {
			s.lock.Unlock()
			debug("restartNeeded: unlocked")
		}()

		debug("restartNeeded: locked")

		if s.cancelFunc == nil {
			// we're not running yet
			debug("restartNeeded: not running yet")
			return
		}

		debug("restartNeeded: calling restart function")
		s.cancelFunc(errRestartNeeded)
	}()
}
