package v1

import (
	v1 "k8s.io/api/core/v1"
)

const (
	// InPlaceUpdateReady must be added into template.spec.readinessGates when pod podUpdatePolicy
	// is InPlaceIfPossible or InPlaceOnly. The condition in podStatus will be updated to False before in-place
	// updating and updated to True after the update is finished. This ensures pod to remain at NotReady state while
	// in-place update is happening.
	InPlaceUpdateReady v1.PodConditionType = "InPlaceUpdateReady"

	// InPlaceUpdateStateKey records the state of inplace-update.
	// The value of annotation is InPlaceUpdateState.
	InPlaceUpdateStateKey string = "inplace-update-state"
)
