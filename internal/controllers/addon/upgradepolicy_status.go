package addon

import (
	"context"
	"fmt"

	"github.com/openshift/addon-operator/internal/ocm"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

func (r *AddonReconciler) handleUpgradePolicyStatusReporting(
	ctx context.Context,
	log logr.Logger,
	addon *addonsv1alpha1.Addon,
) error {
	if !requiresReporting(addon) {
		return nil
	}

	r.ocmClientMux.RLock()
	defer r.ocmClientMux.RUnlock()

	if r.ocmClient == nil {
		// OCM Client is not initialized.
		// Either the AddonOperatorReconciler did not yet create and inject the client or
		// the AddonOperator CR is not configured for OCM status reporting.
		//
		// All Addons will be requeued when the client becomes available for the first time.
		log.Info("delaying Addon status reporting to UpgradePolicy endpoint until OCM client is initialized")

		return nil
	}

	log = log.WithValues(
		"UpgradePolicy", addon.Spec.UpgradePolicy,
		"Version", addon.Spec.Version,
	)

	if addon.Status.UpgradePolicy == nil {
		log.Info("UpgradePolicy status unknown; reporting upgrade as started")

		return r.reportUpgradeStarted(ctx, addon)
	}
	if addon.Status.UpgradePolicy.Version == "" {
		log.Info("previous upgrade version unknown; retrieving status from OCM")

		if complete, err := r.handleUnknownPreviousVersion(ctx, log, addon); err != nil {
			return fmt.Errorf("handling UpgradePolicy with unknown previous version: %w", err)
		} else if complete {
			return nil
		}
	}
	if addon.Status.UpgradePolicy.Version != addon.Spec.Version {
		prevVer := addon.Status.UpgradePolicy.Version

		log.Info(
			fmt.Sprintf(
				"version %q from UpgradePolicy status is stale; reporting upgrade for version %q as started",
				prevVer,
				addon.Spec.Version,
			),
		)

		return r.reportUpgradeStarted(ctx, addon)
	}
	if addon.IsAvailable() {
		log.Info("reporting upgrade as completed")

		return r.reportUpgradeCompleted(ctx, addon)
	}

	return nil
}

func requiresReporting(addon *addonsv1alpha1.Addon) bool {
	return addon.Spec.Version != "" &&
		addon.Spec.UpgradePolicy != nil &&
		!addon.UpgradeCompleteForCurrentVersion()
}

func (r *AddonReconciler) handleUnknownPreviousVersion(ctx context.Context, log logr.Logger, addon *addonsv1alpha1.Addon) (bool, error) {
	policyID := addon.Spec.UpgradePolicy.ID
	req := ocm.UpgradePolicyGetRequest{
		ID: policyID,
	}

	res, err := r.handleGetUpgradePolicyState(ctx, req)
	if err != nil {
		return false, fmt.Errorf(
			"getting UpgradePolicy %q state: %w", policyID, err,
		)
	}

	if res.Value == ocm.UpgradePolicyValueCompleted {
		log.Info("previous upgrade completed; setting UpgradePolicy status to complete")
		// When the upgrade policy is "completed" in the OCM API but we don't have
		// a version in our status, we just have to populate the current version to our
		// status as the "version" was just recently introduced
		addon.SetUpgradePolicyStatus(addonsv1alpha1.AddonUpgradePolicyValueCompleted)

		return true, nil
	}

	log.Info(fmt.Sprintf("found current upgrade with state %q", string(res.Value)))

	return false, nil
}

func (r *AddonReconciler) reportUpgradeStarted(ctx context.Context, addon *addonsv1alpha1.Addon) error {
	var (
		policyID = addon.Spec.UpgradePolicy.ID
		version  = addon.Spec.Version
	)

	req := ocm.UpgradePolicyPatchRequest{
		ID:          policyID,
		Value:       ocm.UpgradePolicyValueStarted,
		Description: fmt.Sprintf("Upgrading addon to version %q.", version),
	}

	if err := r.handlePatchUpgradePolicy(ctx, req); err != nil {
		return fmt.Errorf(
			"patching UpgradePolicy %q at version %q as 'Started': %w", policyID, version, err,
		)
	}

	addon.SetUpgradePolicyStatus(addonsv1alpha1.AddonUpgradePolicyValueStarted)

	return nil
}

func (r *AddonReconciler) reportUpgradeCompleted(ctx context.Context, addon *addonsv1alpha1.Addon) error {
	var (
		policyID = addon.Spec.UpgradePolicy.ID
		version  = addon.Spec.Version
	)

	req := ocm.UpgradePolicyPatchRequest{
		ID:          policyID,
		Value:       ocm.UpgradePolicyValueCompleted,
		Description: fmt.Sprintf("Addon was healthy at least once at version %q.", version),
	}

	if err := r.handlePatchUpgradePolicy(ctx, req); err != nil {
		return fmt.Errorf(
			"patching UpgradePolicy %q at version %q to 'Completed': %w", policyID, version, err,
		)
	}

	addon.SetUpgradePolicyStatus(addonsv1alpha1.AddonUpgradePolicyValueCompleted)

	return nil
}

func (r *AddonReconciler) handlePatchUpgradePolicy(ctx context.Context,
	req ocm.UpgradePolicyPatchRequest) (err error) {
	r.recordOCMRequestDuration(func() {
		_, err = r.ocmClient.PatchUpgradePolicy(ctx, req)
	})

	return
}

func (r *AddonReconciler) handleGetUpgradePolicyState(ctx context.Context,
	req ocm.UpgradePolicyGetRequest) (res ocm.UpgradePolicyGetResponse, err error) {
	r.recordOCMRequestDuration(func() {
		res, err = r.ocmClient.GetUpgradePolicy(ctx, req)
	})

	return
}

func (r *AddonReconciler) recordOCMRequestDuration(reqFunc func()) {
	if r.Recorder != nil {
		// TODO: do not count metrics when API returns 5XX response
		timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
			us := v * 1000000 // convert to microseconds
			r.Recorder.RecordOCMAPIRequests(us)
		}))
		defer timer.ObserveDuration()
	}

	reqFunc()
}
