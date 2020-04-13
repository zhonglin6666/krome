package statefulset

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	kromev1 "krome/pkg/apis/apps/v1"
	kromeclient "krome/pkg/client"
)

var (
	// controllerKind contains the schema.GroupVersionKind for StatefulSet type.
	controllerKind = kromev1.SchemeGroupVersion.WithKind("StatefulSet")
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new StatefulSet Controller and adds it to the Manager. The Manager will set fields on the Controller
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
	kromeClient, err := kromeclient.NewClient(mgr)
	if err != nil {
		return nil, err
	}

	return &ReconcileStatefulSet{
		mgr:         mgr,
		kromeClient: kromeClient,
		control: NewDefaultStatefulSetControl(
			NewRealStatefulPodControl(kromeClient.K8sClient, mgr, mgr.GetEventRecorderFor("realPodControl")),
			NewRealStatefulSetStatusUpdater(kromeClient, mgr),
			mgr.GetEventRecorderFor("control"),
			kromeClient.K8sClient,
			mgr,
		),
		podControl: kubecontroller.RealPodControl{
			KubeClient: kromeClient.K8sClient,
			Recorder:   mgr.GetEventRecorderFor("podControl"),
		},
	}, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("StatefulSet-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource StatefulSet
	err = c.Watch(&source.Kind{Type: &kromev1.StatefulSet{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to resource Pods and requeue the owner StatefulSet
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &kromev1.StatefulSet{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to resource ControllerRevisions and requeue the owner StatefulSet
	err = c.Watch(&source.Kind{Type: &appsv1.ControllerRevision{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &kromev1.StatefulSet{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileStatefulSet implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileStatefulSet{}

// ReconcileStatefulSet reconciles a StatefulSet object
type ReconcileStatefulSet struct {
	mgr         manager.Manager
	scheme      *runtime.Scheme
	kromeClient *kromeclient.Client

	control    StatefulSetControlInterface
	podControl kubecontroller.PodControlInterface
}

// syncStatefulSet syncs a tuple of (StatefulSet, []*v1.Pod).
func (r *ReconcileStatefulSet) syncStatefulSet(set *kromev1.StatefulSet, pods []*corev1.Pod) error {
	logrus.Infof("Syncing StatefulSet %v/%v with %d pods", set.Namespace, set.Name, len(pods))
	if err := r.control.UpdateStatefulSet(set.DeepCopy(), pods); err != nil {
		return err
	}

	logrus.Infof("Successfully synced StatefulSet %s/%s successful", set.Namespace, set.Name)
	return nil
}

// Reconcile reads that state of the cluster for a StatefulSet object and makes changes based on the state read
// and what is in the StatefulSet.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileStatefulSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logrus.Infof("Reconcile StatefulSet request namespace: %s, name: %s", request.Namespace, request.Name)

	// Fetch the StatefulSet instance
	set := &kromev1.StatefulSet{}
	err := r.mgr.GetClient().Get(context.TODO(), request.NamespacedName, set)
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

	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("error converting StatefulSet %v selector: %v", request.String(), err))
		// This is a non-transient error, so don't retry.
		return reconcile.Result{}, nil
	}

	if err := r.adoptOrphanRevisions(set); err != nil {
		return reconcile.Result{}, err
	}

	pods, err := r.getPodsForStatefulSet(set, selector)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, r.syncStatefulSet(set, pods)
}

// adoptOrphanRevisions adopts any orphaned ControllerRevisions matched by set's Selector.
func (r *ReconcileStatefulSet) adoptOrphanRevisions(set *kromev1.StatefulSet) error {
	revisions, err := r.control.ListRevisions(set)
	if err != nil {
		return err
	}
	hasOrphans := false
	for i := range revisions {
		if metav1.GetControllerOf(revisions[i]) == nil {
			hasOrphans = true
			break
		}
	}
	if hasOrphans {
		fresh, err := r.kromeClient.KromeClient.AppsV1().StatefulSets(set.Namespace).Get(context.TODO(), set.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if fresh.UID != set.UID {
			return fmt.Errorf("original StatefulSet %v/%v is gone: got uid %v, wanted %v", set.Namespace, set.Name, fresh.UID, set.UID)
		}
		return r.control.AdoptOrphanRevisions(set, revisions)
	}
	return nil
}

// getPodsForStatefulSet returns the Pods that a given StatefulSet should manage.
// It also reconciles ControllerRef by adopting/orphaning.
//
// NOTE: Returned Pods are pointers to objects from the cache.
//       If you need to modify one, you need to copy it first.
func (r *ReconcileStatefulSet) getPodsForStatefulSet(set *kromev1.StatefulSet, selector labels.Selector) ([]*corev1.Pod, error) {
	// List all pods to include the pods that don't match the selector anymore but
	// has a ControllerRef pointing to this StatefulSet.
	var pods = corev1.PodList{}
	if err := r.mgr.GetClient().List(context.TODO(), &pods, &client.ListOptions{LabelSelector: selector}); err != nil {
		return nil, err
	}

	filter := func(pod *corev1.Pod) bool {
		// Only claim if it matches our StatefulSet name. Otherwise release/ignore.
		return isMemberOf(set, pod)
	}

	// If any adoptions are attempted, we should first recheck for deletion with
	// an uncached quorum read sometime after listing Pods (see #42639).
	canAdoptFunc := kubecontroller.RecheckDeletionTimestamp(func() (metav1.Object, error) {
		fresh, err := r.kromeClient.KromeClient.AppsV1().StatefulSets(set.Namespace).Get(context.TODO(), set.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if fresh.UID != set.UID {
			return nil, fmt.Errorf("original StatefulSet %v/%v is gone: got uid %v, wanted %v", set.Namespace, set.Name, fresh.UID, set.UID)
		}
		return fresh, nil
	})

	items := make([]*corev1.Pod, len(pods.Items))
	for i, p := range pods.Items {
		items[i] = p.DeepCopy()
	}

	cm := kubecontroller.NewPodControllerRefManager(r.podControl, set, selector, controllerKind, canAdoptFunc)
	return cm.ClaimPods(items, filter)
}
