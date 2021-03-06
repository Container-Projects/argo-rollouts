package rollout

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/argoproj/argo-rollouts/rollout/trafficrouting/istio"
	replicasetutil "github.com/argoproj/argo-rollouts/utils/replicaset"
)

// TrafficRoutingReconciler common function across all TrafficRouting implementation
type TrafficRoutingReconciler interface {
	Reconcile(desiredWeight int32) error
	Type() string
}

// NewTrafficRoutingReconciler identifies return the TrafficRouting Plugin that the rollout wants to modify
func (c *RolloutController) NewTrafficRoutingReconciler(roCtx rolloutContext) TrafficRoutingReconciler {
	rollout := roCtx.Rollout()
	if rollout.Spec.Strategy.Canary.TrafficRouting == nil {
		return nil
	}
	if rollout.Spec.Strategy.Canary.TrafficRouting.Istio != nil {
		return istio.NewReconciler(rollout, c.dynamicclientset, c.recorder, c.defaultIstioVersion)
	}
	return nil
}

func (c *RolloutController) reconcileTrafficRouting(roCtx *canaryContext) error {
	rollout := roCtx.Rollout()
	reconciler := c.newTrafficRoutingReconciler(roCtx)
	if reconciler == nil {
		return nil
	}
	roCtx.Log().Infof("Reconciling TrafficRouting with type '%s'", reconciler.Type())
	newRS := roCtx.NewRS()
	stableRS := roCtx.StableRS()
	olderRS := roCtx.OlderRSs()

	_, index := replicasetutil.GetCurrentCanaryStep(rollout)
	desiredWeight := int32(0)
	if index != nil {
		atDesiredReplicaCount := replicasetutil.AtDesiredReplicaCountsForCanary(rollout, newRS, stableRS, olderRS)
		if !atDesiredReplicaCount {
			// Use the previous weight since the new RS is not ready for a new weight
			for i := *index - 1; i >= 0; i-- {
				step := rollout.Spec.Strategy.Canary.Steps[i]
				if step.SetWeight != nil {
					desiredWeight = *step.SetWeight
					break
				}
			}
		} else if *index != int32(len(rollout.Spec.Strategy.Canary.Steps)) {
			// This if statement prevents the desiredWeight from being set to 100
			// when the rollout has progressed through all the steps. The rollout
			// should send all traffic to the stable service by using a weight of
			// 0. If the rollout is progressing through the steps, the desired
			// weight of the traffic routing service should be at the value of the
			// last setWeight step, which is set by GetCurrentSetWeight.
			desiredWeight = replicasetutil.GetCurrentSetWeight(rollout)
		}
	}

	err := reconciler.Reconcile(desiredWeight)
	if err != nil {
		c.recorder.Event(rollout, corev1.EventTypeWarning, "TrafficRoutingError", err.Error())
	}
	return err
}
