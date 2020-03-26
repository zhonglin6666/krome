/*
Copyright 2017 The Kubernetes Authors.
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

package statefulset

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/util/retry"

	kromev1 "krome/pkg/apis/apps/v1"
	kromeclient "krome/pkg/client"
)

// StatefulSetStatusUpdaterInterface is an interface used to update the StatefulSetStatus associated with a StatefulSet.
// For any use other than testing, clients should create an instance using NewRealStatefulSetStatusUpdater.
type StatefulSetStatusUpdaterInterface interface {
	// UpdateStatefulSetStatus sets the set's Status to status. Implementations are required to retry on conflicts,
	// but fail on other errors. If the returned error is nil set's Status has been successfully set to status.
	UpdateStatefulSetStatus(set *kromev1.Statefulset, status *kromev1.StatefulsetStatus) error
}

// NewRealStatefulSetStatusUpdater returns a StatefulSetStatusUpdaterInterface that updates the Status of a StatefulSet,
// using the supplied client and setLister.
func NewRealStatefulSetStatusUpdater(client *kromeclient.Client, mgr manager.Manager) StatefulSetStatusUpdaterInterface {
	return &realStatefulSetStatusUpdater{client, mgr}
}

type realStatefulSetStatusUpdater struct {
	client *kromeclient.Client
	mgr    manager.Manager
}

func (ssu *realStatefulSetStatusUpdater) UpdateStatefulSetStatus(
	set *kromev1.Statefulset,
	status *kromev1.StatefulsetStatus) error {
	// don't wait due to limited number of clients, but backoff after the default number of steps
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		set.Status = *status
		_, updateErr := ssu.client.KromeClient.AppsV1().Statefulsets(set.Namespace).UpdateStatus(context.TODO(), set, metav1.UpdateOptions{})
		if updateErr == nil {
			return nil
		}

		var updated = &kromev1.Statefulset{}
		if err := ssu.mgr.GetClient().Get(context.TODO(), types.NamespacedName{set.Namespace, set.Name}, updated); err != nil {
			// make a copy so we don't mutate the shared cache
			set = updated.DeepCopy()
		} else {
			utilruntime.HandleError(fmt.Errorf("error getting updated StatefulSet %s/%s from cache: %v", set.Namespace, set.Name, err))
		}

		return updateErr
	})
}

var _ StatefulSetStatusUpdaterInterface = &realStatefulSetStatusUpdater{}
