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
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	core "k8s.io/client-go/testing"
	mgrclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	kromev1 "krome.io/krome/pkg/apis/apps/v1"
	fakekrome "krome.io/krome/pkg/client/clientset/versioned/fake"
)

func TestStatefulSetUpdaterUpdatesSetStatus(t *testing.T) {
	set := newStatefulSet(3)
	status := kromev1.StatefulSetStatus{
		ObservedGeneration: 1,
		Replicas:           2,
	}
	fakeClient := &fakekrome.Clientset{}
	sch := scheme.Scheme
	sb := kromev1.SchemeBuilder
	sb.AddToScheme(sch)
	ss := newStatefulSet(3)
	fakeMgrClient := mgrclient.NewFakeClientWithScheme(sch, ss)

	updater := NewRealStatefulSetStatusUpdater(fakeClient, fakeMgrClient)

	fakeClient.AddReactor("update", "statefulsets", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		return true, update.GetObject(), nil
	})
	if err := updater.UpdateStatefulSetStatus(set, &status); err != nil {
		t.Errorf("Error returned on successful status update: %s", err)
	}
	if set.Status.Replicas != 2 {
		t.Errorf("UpdateStatefulSetStatus mutated the sets replicas %d", set.Status.Replicas)
	}
}

func TestStatefulSetStatusUpdaterUpdatesObservedGeneration(t *testing.T) {
	set := newStatefulSet(3)
	status := kromev1.StatefulSetStatus{ObservedGeneration: 3, Replicas: 2}
	fakeClient := &fakekrome.Clientset{}
	sch := scheme.Scheme
	sb := kromev1.SchemeBuilder
	sb.AddToScheme(sch)
	ss := newStatefulSet(3)
	fakeMgrClient := mgrclient.NewFakeClientWithScheme(sch, ss)

	updater := NewRealStatefulSetStatusUpdater(fakeClient, fakeMgrClient)
	fakeClient.AddReactor("update", "statefulsets", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		sts := update.GetObject().(*kromev1.StatefulSet)
		if sts.Status.ObservedGeneration != 3 {
			t.Errorf("expected observedGeneration to be synced with generation for statefulset %q", sts.Name)
		}
		return true, sts, nil
	})
	if err := updater.UpdateStatefulSetStatus(set, &status); err != nil {
		t.Errorf("Error returned on successful status update: %s", err)
	}
}

func TestStatefulSetStatusUpdaterUpdateReplicasFailure(t *testing.T) {
	set := newStatefulSet(3)
	status := kromev1.StatefulSetStatus{ObservedGeneration: 3, Replicas: 2}
	fakeClient := &fakekrome.Clientset{}
	sch := scheme.Scheme
	sb := kromev1.SchemeBuilder
	sb.AddToScheme(sch)
	ss := newStatefulSet(3)
	fakeMgrClient := mgrclient.NewFakeClientWithScheme(sch, ss)

	updater := NewRealStatefulSetStatusUpdater(fakeClient, fakeMgrClient)
	fakeClient.AddReactor("update", "statefulsets", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(errors.New("API server down"))
	})
	if err := updater.UpdateStatefulSetStatus(set, &status); err == nil {
		t.Error("Failed update did not return error")
	}
}

func TestStatefulSetStatusUpdaterUpdateReplicasConflict(t *testing.T) {
	set := newStatefulSet(3)
	status := kromev1.StatefulSetStatus{ObservedGeneration: 3, Replicas: 2}
	conflict := false
	fakeClient := &fakekrome.Clientset{}
	sch := scheme.Scheme
	sb := kromev1.SchemeBuilder
	sb.AddToScheme(sch)
	ss := newStatefulSet(3)
	fakeMgrClient := mgrclient.NewFakeClientWithScheme(sch, ss)

	updater := NewRealStatefulSetStatusUpdater(fakeClient, fakeMgrClient)
	fakeClient.AddReactor("update", "statefulsets", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		if !conflict {
			conflict = true
			return true, update.GetObject(), apierrors.NewConflict(action.GetResource().GroupResource(), set.Name, errors.New("object already exists"))
		}
		return true, update.GetObject(), nil

	})
	if err := updater.UpdateStatefulSetStatus(set, &status); err != nil {
		t.Errorf("UpdateStatefulSetStatus returned an error: %s", err)
	}
	if set.Status.Replicas != 2 {
		t.Errorf("UpdateStatefulSetStatus mutated the sets replicas %d", set.Status.Replicas)
	}
}

func TestStatefulSetStatusUpdaterUpdateReplicasConflictFailure(t *testing.T) {
	set := newStatefulSet(3)
	status := kromev1.StatefulSetStatus{ObservedGeneration: 3, Replicas: 2}

	fakeClient := &fakekrome.Clientset{}
	sch := scheme.Scheme
	sb := kromev1.SchemeBuilder
	sb.AddToScheme(sch)
	ss := newStatefulSet(3)
	fakeMgrClient := mgrclient.NewFakeClientWithScheme(sch, ss)

	updater := NewRealStatefulSetStatusUpdater(fakeClient, fakeMgrClient)
	fakeClient.AddReactor("update", "statefulsets", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		return true, update.GetObject(), apierrors.NewConflict(action.GetResource().GroupResource(), set.Name, errors.New("object already exists"))
	})
	if err := updater.UpdateStatefulSetStatus(set, &status); err == nil {
		t.Error("UpdateStatefulSetStatus failed to return an error on get failure")
	}
}
