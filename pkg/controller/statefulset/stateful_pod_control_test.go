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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakek8sclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	mgrclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	kromev1 "krome.io/krome/pkg/apis/apps/v1"
)

var (
	ss = &kromev1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps.krome.io/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-statefulset",
			Namespace: "ns1",
		},
	}
)

func TestStatefulPodControlCreatePods(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	fakeClient := &fakek8sclient.Clientset{}
	fakeMgrClient := mgrclient.NewFakeClient(ss)

	podControl := NewRealStatefulPodControl(fakeClient, fakeMgrClient, recorder)
	pod := newStatefulSetPod(ss, 2)

	if err := podControl.CreateStatefulPod(ss, pod); err != nil {
		t.Errorf("StatefulPodControl failed to create Pod error: %s", err)
	}
}
