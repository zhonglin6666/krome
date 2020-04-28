package deployment

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	kromeclient "krome.io/krome/pkg/client/clientset/versioned"
)

// RealRSControl is the default implementation of RSControllerInterface.
type RealRSControl struct {
	KubeClient *kromeclient.Clientset
	Recorder   record.EventRecorder
}

func (r RealRSControl) PatchReplicaSet(namespace, name string, data []byte) error {
	_, err := r.KubeClient.AppsV1().ReplicaSets(namespace).Patch(context.TODO(), name, types.StrategicMergePatchType, data, metav1.PatchOptions{})
	return err
}
