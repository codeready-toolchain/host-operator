package usersignup

import (
	"context"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"

	"github.com/go-logr/logr"
	errs "github.com/pkg/errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const timestampLayout = "2006-01-02T15:04:05.000Z07:00"

type metricsUpdater struct {
	client  client.Client
	counter *metrics.Counter
}

func (c metricsUpdater) updateStatus(userSignup *toolchainv1alpha1.UserSignup) error {

	// If the metric is already being tracked then nothing to do
	if _, exists := userSignup.Status.Metrics.MetricsRegistry[c.counter.Name]; exists {
		return nil
	}

	userSignup.Status.Metrics.MetricsRegistry[c.counter.Name] = time.Now().Format(timestampLayout)

	err := c.client.Status().Update(context.TODO(), userSignup)
	if err == nil {
		// only call the metric function if the status was updated successfully
		c.counter.Increment()
	}
	return err
}

// UpdateMetricIfNotSet updates the metric measurement if the metric has not already been set
func UpdateMetricIfNotSet(cl client.Client, reqLogger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, c *metrics.Counter) error {
	m := metricsUpdater{client: cl, counter: c}
	reqLogger.Info("set usersignup metric", "name", c.Name)

	// if there's no metrics status then initialize the status
	if userSignup.Status.Metrics == nil {
		metricsRegistry := make(map[string]string, 5)
		userSignup.Status.Metrics = &toolchainv1alpha1.MetricsStatus{MetricsRegistry: metricsRegistry}
	}

	if err := m.updateStatus(userSignup); err != nil {
		return errs.Wrapf(err, "failed to update status metrics for metric %s", c.Name)
	}

	reqLogger.Info("metric updated and tracked for usersignup", "name", c.Name)
	return nil
}
