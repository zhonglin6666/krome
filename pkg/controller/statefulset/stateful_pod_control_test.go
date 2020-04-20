/*
Copyright 2020 The Krome Authors.

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
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	fakek8sclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"
	mgrclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	kromev1 "krome.io/krome/pkg/apis/apps/v1"
)

func TestStatefulPodControlCreatePods(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	fakeClient := &fakek8sclient.Clientset{}
	sch := scheme.Scheme
	sb := kromev1.SchemeBuilder
	sb.AddToScheme(sch)
	ss := newStatefulSet(3)
	pod := newStatefulSetPod(ss, 0)
	fakeMgrClient := mgrclient.NewFakeClientWithScheme(sch, ss)

	podControl := NewRealStatefulPodControl(fakeClient, fakeMgrClient, recorder)

	fakeClient.AddReactor("get", "persistentvolumeclaims", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(action.GetResource().GroupResource(), action.GetResource().Resource)
	})
	fakeClient.AddReactor("create", "persistentvolumeclaims", func(action core.Action) (bool, runtime.Object, error) {
		create := action.(core.CreateAction)
		return true, create.GetObject(), nil
	})
	fakeClient.AddReactor("create", "pods", func(action core.Action) (bool, runtime.Object, error) {
		create := action.(core.CreateAction)
		return true, create.GetObject(), nil
	})

	if err := podControl.CreateStatefulPod(ss, pod); err != nil {
		t.Errorf("StatefulPodControl failed to create Pod error: %s", err)
	}

	t.Logf("TestStatefulPodControlCreatePods succeed")

	events := collectEvents(recorder.Events)
	if eventCount := len(events); eventCount != 2 {
		t.Errorf("Expected 2 events for successful create found %d", eventCount)
	}
	for i := range events {
		if !strings.Contains(events[i], v1.EventTypeNormal) {
			t.Errorf("Found unexpected non-normal event %s", events[i])
		}
	}
}

func TestStatefulPodControlCreatePodExists(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	fakeClient := &fakek8sclient.Clientset{}
	sch := scheme.Scheme
	sb := kromev1.SchemeBuilder
	sb.AddToScheme(sch)
	ss := newStatefulSet(3)
	pod := newStatefulSetPod(ss, 0)
	fakeMgrClient := mgrclient.NewFakeClientWithScheme(sch, ss)

	podControl := NewRealStatefulPodControl(fakeClient, fakeMgrClient, recorder)
	fakeClient.AddReactor("create", "persistentvolumeclaims", func(action core.Action) (bool, runtime.Object, error) {
		create := action.(core.CreateAction)
		return true, create.GetObject(), nil
	})
	fakeClient.AddReactor("create", "pods", func(action core.Action) (bool, runtime.Object, error) {
		return true, pod, apierrors.NewAlreadyExists(action.GetResource().GroupResource(), pod.Name)
	})
	if err := podControl.CreateStatefulPod(ss, pod); !apierrors.IsAlreadyExists(err) {
		t.Errorf("Failed to create Pod error: %s", err)
	}

	events := collectEvents(recorder.Events)
	if eventCount := len(events); eventCount != 0 {
		t.Errorf("Pod and PVC exist: got %d events, but want 0", eventCount)
		for i := range events {
			t.Log(events[i])
		}
	}
}

func collectEvents(source <-chan string) []string {
	done := false
	events := make([]string, 0)
	for !done {
		select {
		case event := <-source:
			events = append(events, event)
		default:
			done = true
		}
	}
	return events
}
