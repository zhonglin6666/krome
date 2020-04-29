/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// If you make changes to this file, you should also make the corresponding change in ReplicationController.

package replicaset

import (
	"context"
	"fmt"
	"reflect"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"

	kromev1 "krome.io/krome/pkg/apis/apps/v1"
	kromeclient "krome.io/krome/pkg/client/clientset/versioned"
)

// updateReplicaSetStatus attempts to update the Status.Replicas of the given ReplicaSet, with a single GET/PUT retry.
func updateReplicaSetStatus(kromeClient *kromeclient.Clientset, rs *kromev1.ReplicaSet, newStatus kromev1.ReplicaSetStatus) (*kromev1.ReplicaSet, error) {
	// This is the steady state. It happens when the ReplicaSet doesn't have any expectations, since
	// we do a periodic relist every 30s. If the generations differ but the replicas are
	// the same, a caller might've resized to the same replica count.
	if rs.Status.Replicas == newStatus.Replicas &&
		rs.Status.FullyLabeledReplicas == newStatus.FullyLabeledReplicas &&
		rs.Status.ReadyReplicas == newStatus.ReadyReplicas &&
		rs.Status.AvailableReplicas == newStatus.AvailableReplicas &&
		rs.Generation == rs.Status.ObservedGeneration &&
		reflect.DeepEqual(rs.Status.Conditions, newStatus.Conditions) {
		return rs, nil
	}

	// Save the generation number we acted on, otherwise we might wrongfully indicate
	// that we've seen a spec update when we retry.
	// TODO: This can clobber an update if we allow multiple agents to write to the
	// same status.
	newStatus.ObservedGeneration = rs.Generation

	var updateErr error
	var updatedRS *kromev1.ReplicaSet
	for i, rs := 0, rs; ; i++ {
		logrus.Infof(fmt.Sprintf("Updating status for %v: %s/%s, ", rs.Kind, rs.Namespace, rs.Name) +
			fmt.Sprintf("replicas %d->%d (need %d), ", rs.Status.Replicas, newStatus.Replicas, *(rs.Spec.Replicas)) +
			fmt.Sprintf("fullyLabeledReplicas %d->%d, ", rs.Status.FullyLabeledReplicas, newStatus.FullyLabeledReplicas) +
			fmt.Sprintf("readyReplicas %d->%d, ", rs.Status.ReadyReplicas, newStatus.ReadyReplicas) +
			fmt.Sprintf("availableReplicas %d->%d, ", rs.Status.AvailableReplicas, newStatus.AvailableReplicas) +
			fmt.Sprintf("sequence No: %v->%v", rs.Status.ObservedGeneration, newStatus.ObservedGeneration))

		rs.Status = newStatus
		updatedRS, updateErr = kromeClient.AppsV1().ReplicaSets(rs.Namespace).Update(context.TODO(), rs, metav1.UpdateOptions{})
		if updateErr == nil {
			return updatedRS, nil
		}
		// Stop retrying if we exceed statusUpdateRetries - the replicaSet will be requeued with a rate limit.
		if i >= statusUpdateRetries {
			break
		}

		// TODO zzlin
		// Update the ReplicaSet with the latest resource version for the next poll
		//var getRS = &kromev1.ReplicaSet{}
		//if rs, getErr = kromeClient.Get(rs.Name, metav1.GetOptions{}); getErr != nil {
		//	// If the GET fails we can't trust status.Replicas anymore. This error
		//	// is bound to be more interesting than the update failure.
		//	return nil, getErr
		//}
	}

	return nil, updateErr
}

func calculateStatus(rs *kromev1.ReplicaSet, filteredPods []*corev1.Pod, manageReplicasErr error) kromev1.ReplicaSetStatus {
	newStatus := rs.Status
	// Count the number of pods that have labels matching the labels of the pod
	// template of the replica set, the matching pods may have more
	// labels than are in the template. Because the label of podTemplateSpec is
	// a superset of the selector of the replica set, so the possible
	// matching pods must be part of the filteredPods.
	fullyLabeledReplicasCount := 0
	readyReplicasCount := 0
	availableReplicasCount := 0
	templateLabel := labels.Set(rs.Spec.Template.Labels).AsSelectorPreValidated()

	for _, pod := range filteredPods {
		if templateLabel.Matches(labels.Set(pod.Labels)) {
			fullyLabeledReplicasCount++
		}
		if podutil.IsPodReady(pod) {
			readyReplicasCount++
			if podutil.IsPodAvailable(pod, rs.Spec.MinReadySeconds, metav1.Now()) {
				availableReplicasCount++
			}
		}
	}
	failureCond := GetCondition(rs.Status, kromev1.ReplicaSetReplicaFailure)
	if manageReplicasErr != nil && failureCond == nil {
		var reason string
		if diff := len(filteredPods) - int(*(rs.Spec.Replicas)); diff < 0 {
			reason = "FailedCreate"
		} else if diff > 0 {
			reason = "FailedDelete"
		}
		cond := NewReplicaSetCondition(kromev1.ReplicaSetReplicaFailure, corev1.ConditionTrue, reason, manageReplicasErr.Error())
		SetCondition(&newStatus, cond)
	} else if manageReplicasErr == nil && failureCond != nil {
		RemoveCondition(&newStatus, kromev1.ReplicaSetReplicaFailure)
	}

	newStatus.Replicas = int32(len(filteredPods))
	newStatus.FullyLabeledReplicas = int32(fullyLabeledReplicasCount)
	newStatus.ReadyReplicas = int32(readyReplicasCount)
	newStatus.AvailableReplicas = int32(availableReplicasCount)
	return newStatus
}

// NewReplicaSetCondition creates a new replicaset condition.
func NewReplicaSetCondition(condType kromev1.ReplicaSetConditionType, status corev1.ConditionStatus, reason, msg string) kromev1.ReplicaSetCondition {
	return kromev1.ReplicaSetCondition{
		Type:               condType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            msg,
	}
}

// GetCondition returns a replicaset condition with the provided type if it exists.
func GetCondition(status kromev1.ReplicaSetStatus, condType kromev1.ReplicaSetConditionType) *kromev1.ReplicaSetCondition {
	for _, c := range status.Conditions {
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

// SetCondition adds/replaces the given condition in the replicaset status. If the condition that we
// are about to add already exists and has the same status and reason then we are not going to update.
func SetCondition(status *kromev1.ReplicaSetStatus, condition kromev1.ReplicaSetCondition) {
	currentCond := GetCondition(*status, condition.Type)
	if currentCond != nil && currentCond.Status == condition.Status && currentCond.Reason == condition.Reason {
		return
	}
	newConditions := filterOutCondition(status.Conditions, condition.Type)
	status.Conditions = append(newConditions, condition)
}

// RemoveCondition removes the condition with the provided type from the replicaset status.
func RemoveCondition(status *kromev1.ReplicaSetStatus, condType kromev1.ReplicaSetConditionType) {
	status.Conditions = filterOutCondition(status.Conditions, condType)
}

// filterOutCondition returns a new slice of replicaset conditions without conditions with the provided type.
func filterOutCondition(conditions []kromev1.ReplicaSetCondition, condType kromev1.ReplicaSetConditionType) []kromev1.ReplicaSetCondition {
	var newConditions []kromev1.ReplicaSetCondition
	for _, c := range conditions {
		if c.Type == condType {
			continue
		}
		newConditions = append(newConditions, c)
	}
	return newConditions
}
