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

// UpdateMetricAndStatus updates the metric measurement and status
func UpdateMetricAndStatus(cl client.Client, reqLogger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, name string, metricUpdater func(userSignup *toolchainv1alpha1.UserSignup), postUpdate func()) error {
	reqLogger.Info("set usersignup metric", "name", name)

	if metricIsSet(userSignup, name) {
		return nil
	}

	// if there's no metrics status then initialize the status
	if userSignup.Status.Metrics == nil {
		metricsRegistry := make(map[string]string, 5)
		userSignup.Status.Metrics = &toolchainv1alpha1.MetricsStatus{MetricsRegistry: metricsRegistry}
	}

	metricUpdater(userSignup)

	if err := cl.Status().Update(context.TODO(), userSignup); err != nil {
		return errs.Wrapf(err, "failed to update status metrics for metric %s", name)
	}

	// only call the metric postUpdate function if the status was updated successfully
	postUpdate()
	reqLogger.Info("metric updated and tracked for usersignup", "name", name)
	return nil
}

func metricIsSet(userSignup *toolchainv1alpha1.UserSignup, name string) bool {
	if userSignup.Status.Metrics == nil || userSignup.Status.Metrics.MetricsRegistry == nil {
		return false
	}
	_, exists := userSignup.Status.Metrics.MetricsRegistry[name]
	return exists
}

// UpdateUserSignupUniqueTotalStatus sets UserSignupUniqueTotal to flag the counter as already counted and indicates the time it was done
func UpdateUserSignupUniqueTotalStatus(userSignup *toolchainv1alpha1.UserSignup) {
	userSignup.Status.Metrics.MetricsRegistry[metrics.UserSignupUniqueTotal.Name] = time.Now().Format(timestampLayout)
}

// UpdateUserSignupProvisionedTotalStatus sets UserSignupProvisionedTotal and removes UserSignupDeactivatedTotal so that reactivation will be counted properly
func UpdateUserSignupProvisionedTotalStatus(userSignup *toolchainv1alpha1.UserSignup) {
	userSignup.Status.Metrics.MetricsRegistry[metrics.UserSignupProvisionedTotal.Name] = time.Now().Format(timestampLayout)
	delete(userSignup.Status.Metrics.MetricsRegistry, metrics.UserSignupDeactivatedTotal.Name)
}

// UpdateUserSignupBannedTotalStatus sets UserSignupBannedTotal to flag the counter as already counted and indicates the time it was done
func UpdateUserSignupBannedTotalStatus(userSignup *toolchainv1alpha1.UserSignup) {
	userSignup.Status.Metrics.MetricsRegistry[metrics.UserSignupBannedTotal.Name] = time.Now().Format(timestampLayout)
}

// UpdateUserSignupDeactivatedTotalStatus sets UserSignupDeactivatedTotal and removes UserSignupProvisionedTotal so that reactivation will be counted properly
func UpdateUserSignupDeactivatedTotalStatus(userSignup *toolchainv1alpha1.UserSignup) {
	userSignup.Status.Metrics.MetricsRegistry[metrics.UserSignupDeactivatedTotal.Name] = time.Now().Format(timestampLayout)
	delete(userSignup.Status.Metrics.MetricsRegistry, metrics.UserSignupProvisionedTotal.Name)
}

// UpdateUserSignupAutoDeactivatedTotalStatus sets UserSignupBannedTotal to flag the counter as already counted and indicates the time it was done
func UpdateUserSignupAutoDeactivatedTotalStatus(userSignup *toolchainv1alpha1.UserSignup) {
	userSignup.Status.Metrics.MetricsRegistry[metrics.UserSignupBannedTotal.Name] = time.Now().Format(timestampLayout)
}
