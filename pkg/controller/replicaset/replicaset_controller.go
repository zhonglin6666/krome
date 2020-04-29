package replicaset

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/utils/integer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	kromev1 "krome.io/krome/pkg/apis/apps/v1"
	kromeclient "krome.io/krome/pkg/client/clientset/versioned"
	kromeutils "krome.io/krome/pkg/utils"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// blank assignment to verify that ReconcileReplicaSet implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileReplicaSet{}

var (
	// controllerKind contains the schema.GroupVersionKind for StatefulSet type.
	controllerKind = kromev1.SchemeGroupVersion.WithKind("Deployment")
)

const (
	// Realistic value of the burstReplica field for the replica set manager based off
	// performance requirements for kubernetes 1.0.
	BurstReplicas = 500

	// The number of times we retry updating a ReplicaSet's status.
	statusUpdateRetries = 1
)

// ReconcileReplicaSet reconciles a ReplicaSet object
type ReconcileReplicaSet struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme

	kromeClient *kromeclient.Clientset
	recorder    record.EventRecorder
	podControl  kubecontroller.RealPodControl

	// A TTLCache of pod creates/deletes each rc expects to see.
	expectations *kubecontroller.UIDTrackingControllerExpectations

	// A ReplicaSet is temporarily suspended after creating/deleting these many replicas.
	// It resumes normal action after observing the watch events for them.
	burstReplicas int
}

// Add creates a new ReplicaSet Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	r, err := newReconciler(mgr)
	if err != nil {
		return err
	}

	return add(mgr, r)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) (reconcile.Reconciler, error) {
	k8sClient, err := k8sclient.NewForConfig(mgr.GetConfig())
	if err != nil {
		return nil, err
	}

	kromeClient, err := kromeclient.NewForConfig(mgr.GetConfig())
	if err != nil {
		return nil, err
	}

	r := &ReconcileReplicaSet{
		client:        mgr.GetClient(),
		scheme:        mgr.GetScheme(),
		kromeClient:   kromeClient,
		recorder:      mgr.GetEventRecorderFor("replicasets"),
		expectations:  kubecontroller.NewUIDTrackingControllerExpectations(kubecontroller.NewControllerExpectations()),
		burstReplicas: BurstReplicas,
	}
	r.podControl = kubecontroller.RealPodControl{
		KubeClient: k8sClient,
		Recorder:   r.recorder,
	}

	return r, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("replicaset-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ReplicaSet
	err = c.Watch(&source.Kind{Type: &kromev1.ReplicaSet{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner ReplicaSet
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &kromev1.ReplicaSet{},
	})
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a ReplicaSet object and makes changes based on the state read
// and what is in the ReplicaSet.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileReplicaSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logrus.Infof("Reconciling ReplicaSet namespace: %s name: %s", request.Namespace, request.Name)

	// Fetch the ReplicaSet instance
	instance := &kromev1.ReplicaSet{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	selector, err := metav1.LabelSelectorAsSelector(instance.Spec.Selector)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("error converting pod selector to selector: %v", err))
		return reconcile.Result{}, nil
	}

	var podList = &corev1.PodList{}
	if err := r.client.List(context.TODO(), podList, &client.ListOptions{LabelSelector: selector}); err != nil {
		return reconcile.Result{}, err
	}

	items := make([]*corev1.Pod, len(podList.Items))
	for i, pod := range podList.Items {
		items[i] = pod.DeepCopy()
	}

	// Ignore inactive pods.
	filteredPods := kromeutils.FilterActivePods(items)

	// NOTE: filteredPods are pointing to objects from cache - if you need to
	// modify them, you need to copy it first.
	filteredPods, err = r.claimsPods(instance, selector, filteredPods)
	if err != nil {
		return reconcile.Result{}, err
	}

	var manageReplicasErr error
	if instance.DeletionTimestamp == nil {
		manageReplicasErr = r.manageReplicas(filteredPods, instance)
	}
	rs := instance.DeepCopy()
	newStatus := calculateStatus(rs, filteredPods, manageReplicasErr)

	// Always updates status as pods come up or die.
	updatedRS, err := updateReplicaSetStatus(r.client, r.kromeClient, rs, newStatus)
	if err != nil {
		// Multiple things could lead to this update failing. Requeuing the replica set ensures
		// Returning an error causes a requeue without forcing a hotloop
		return reconcile.Result{}, err
	}

	// Resync the ReplicaSet after MinReadySeconds as a last line of defense to guard against clock-skew.
	if manageReplicasErr == nil {
		if updatedRS.Status.ReadyReplicas != *(updatedRS.Spec.Replicas) ||
			updatedRS.Status.AvailableReplicas != *(updatedRS.Spec.Replicas) {
			return reconcile.Result{Requeue: true}, nil
		}
	}

	return reconcile.Result{}, manageReplicasErr
}

// manageReplicas checks and updates replicas for the given ReplicaSet.
func (r *ReconcileReplicaSet) manageReplicas(filteredPods []*corev1.Pod, rs *kromev1.ReplicaSet) error {
	diff := len(filteredPods) - int(*(rs.Spec.Replicas))
	rsKey, err := kubecontroller.KeyFunc(rs)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Couldn't get key for %v %#v: %v", controllerKind, rs, err))
		return nil
	}
	if diff < 0 {
		diff *= -1
		if diff > r.burstReplicas {
			diff = r.burstReplicas
		}
		// TODO: Track UIDs of creates just like deletes. The problem currently
		// is we'd need to wait on the result of a create to record the pod's
		// UID, which would require locking *across* the create, which will turn
		// into a performance bottleneck. We should generate a UID for the pod
		// beforehand and store it via ExpectCreations.
		r.expectations.ExpectCreations(rsKey, diff)
		logrus.Infof("Too few replicas for %v %s/%s, need %d, creating %d", controllerKind, rs.Namespace, rs.Name, *(rs.Spec.Replicas), diff)
		// Batch the pod creates. Batch sizes start at SlowStartInitialBatchSize
		// and double with each successful iteration in a kind of "slow start".
		// This handles attempts to start large numbers of pods that would
		// likely all fail with the same error. For example a project with a
		// low quota that attempts to create a large number of pods will be
		// prevented from spamming the API service with the pod create requests
		// after one of its pods fails.  Conveniently, this also prevents the
		// event spam that those failures would generate.
		successfulCreations, err := slowStartBatch(diff, kubecontroller.SlowStartInitialBatchSize, func() error {
			err := r.podControl.CreatePodsWithControllerRef(rs.Namespace, &rs.Spec.Template, rs, metav1.NewControllerRef(rs, controllerKind))
			if err != nil && errors.IsTimeout(err) {
				// Pod is created but its initialization has timed out.
				// If the initialization is successful eventually, the
				// controller will observe the creation via the informer.
				// If the initialization fails, or if the pod keeps
				// uninitialized for a long time, the informer will not
				// receive any update, and the controller will create a new
				// pod when the expectation expires.
				return nil
			}
			return err
		})

		// Any skipped pods that we never attempted to start shouldn't be expected.
		// The skipped pods will be retried later. The next controller resync will
		// retry the slow start process.
		if skippedPods := diff - successfulCreations; skippedPods > 0 {
			logrus.Infof("Slow-start failure. Skipping creation of %d pods, decrementing expectations for %v %v/%v", skippedPods, controllerKind, rs.Namespace, rs.Name)
			for i := 0; i < skippedPods; i++ {
				// Decrement the expected number of creates because the informer won't observe this pod
				r.expectations.CreationObserved(rsKey)
			}
		}
		return err
	} else if diff > 0 {
		if diff > r.burstReplicas {
			diff = r.burstReplicas
		}
		logrus.Infof("Too many replicas for %v %s/%s, need %d, deleting %d", controllerKind, rs.Namespace, rs.Name, *(rs.Spec.Replicas), diff)

		// Choose which Pods to delete, preferring those in earlier phases of startup.
		podsToDelete := getPodsToDelete(filteredPods, diff)

		// Snapshot the UIDs (ns/name) of the pods we're expecting to see
		// deleted, so we know to record their expectations exactly once either
		// when we see it as an update of the deletion timestamp, or as a delete.
		// Note that if the labels on a pod/rs change in a way that the pod gets
		// orphaned, the rs will only wake up after the expectations have
		// expired even if other pods are deleted.
		r.expectations.ExpectDeletions(rsKey, getPodKeys(podsToDelete))

		errCh := make(chan error, diff)
		var wg sync.WaitGroup
		wg.Add(diff)
		for _, pod := range podsToDelete {
			go func(targetPod *corev1.Pod) {
				defer wg.Done()
				if err := r.podControl.DeletePod(rs.Namespace, targetPod.Name, rs); err != nil {
					// Decrement the expected number of deletes because the informer won't observe this deletion
					podKey := kubecontroller.PodKey(targetPod)
					logrus.Infof("Failed to delete %v, decrementing expectations for %v %s/%s", podKey, controllerKind, rs.Namespace, rs.Name)
					r.expectations.DeletionObserved(rsKey, podKey)
					errCh <- err
				}
			}(pod)
		}
		wg.Wait()

		select {
		case err := <-errCh:
			// all errors have been reported before and they're likely to be the same, so we'll only return the first one we hit.
			if err != nil {
				return err
			}
		default:
		}
	}

	return nil
}

// slowStartBatch tries to call the provided function a total of 'count' times,
// starting slow to check for errors, then speeding up if calls succeed.
//
// It groups the calls into batches, starting with a group of initialBatchSize.
// Within each batch, it may call the function multiple times concurrently.
//
// If a whole batch succeeds, the next batch may get exponentially larger.
// If there are any failures in a batch, all remaining batches are skipped
// after waiting for the current batch to complete.
//
// It returns the number of successful calls to the function.
func slowStartBatch(count int, initialBatchSize int, fn func() error) (int, error) {
	remaining := count
	successes := 0
	for batchSize := integer.IntMin(remaining, initialBatchSize); batchSize > 0; batchSize = integer.IntMin(2*batchSize, remaining) {
		errCh := make(chan error, batchSize)
		var wg sync.WaitGroup
		wg.Add(batchSize)
		for i := 0; i < batchSize; i++ {
			go func() {
				defer wg.Done()
				if err := fn(); err != nil {
					errCh <- err
				}
			}()
		}

		wg.Wait()
		curSuccesses := batchSize - len(errCh)
		successes += curSuccesses
		if len(errCh) > 0 {
			return successes, <-errCh
		}
		remaining -= batchSize
	}
	return successes, nil
}

func getPodsToDelete(filteredPods []*corev1.Pod, diff int) []*corev1.Pod {
	// No need to sort pods if we are about to delete all of them.
	// diff will always be <= len(filteredPods), so not need to handle > case.
	if diff < len(filteredPods) {
		// Sort the pods in the order such that not-ready < ready, unscheduled
		// < scheduled, and pending < running. This ensures than we delete pods
		// in the earlier stages whenever possible.
		sort.Sort(kubecontroller.ActivePods(filteredPods))
	}
	return filteredPods[:diff]
}

func getPodKeys(pods []*corev1.Pod) []string {
	podKeys := make([]string, 0, len(pods))
	for _, pod := range pods {
		podKeys = append(podKeys, kubecontroller.PodKey(pod))
	}
	return podKeys
}

func (r *ReconcileReplicaSet) claimsPods(rs *kromev1.ReplicaSet, selector labels.Selector, filterPods []*corev1.Pod) ([]*corev1.Pod, error) {
	// If any adoptions are attempted, we should first recheck for deletion with
	// an uncached quorum read sometime after listing Pods (see #42639).
	canAdoptFunc := kubecontroller.RecheckDeletionTimestamp(func() (metav1.Object, error) {
		fresh, err := r.kromeClient.AppsV1().ReplicaSets(rs.Namespace).Get(context.TODO(), rs.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if fresh.UID != rs.UID {
			return nil, fmt.Errorf("original %v %v/%v is gone: got uid %v, wanted %v", controllerKind, rs.Namespace, rs.Name, fresh.UID, rs.UID)
		}
		return fresh, nil
	})

	cm := kubecontroller.NewPodControllerRefManager(r.podControl, rs, selector, controllerKind, canAdoptFunc)
	return cm.ClaimPods(filterPods)
}
