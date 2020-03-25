package statefulset

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/controller/history"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	kromev1 "krome/pkg/apis/apps/v1"
	kromeclient "krome/pkg/client"
)

var (
	log = logf.Log.WithName("controller_statefulset")

	// controllerKind contains the schema.GroupVersionKind for statefulset type.
	controllerKind = kromev1.SchemeGroupVersion.WithKind("Statefulset")

	podKind                = corev1.SchemeGroupVersion.WithKind("Pod")
	pvcKind                = corev1.SchemeGroupVersion.WithKind("PersistentVolumeClaim")
	controllerRevisionKind = appsv1.SchemeGroupVersion.WithKind("ControllerRevision")
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Statefulset Controller and adds it to the Manager. The Manager will set fields on the Controller
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

	k8sInformerFactory := informers.NewSharedInformerFactory(kromeClient.K8sClient, time.Second*30)
	podInformer := k8sInformerFactory.Core().V1().Pods()
	pvcInformer := k8sInformerFactory.Core().V1().PersistentVolumeClaims()
	revInformer := k8sInformerFactory.Apps().V1().ControllerRevisions()

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := make(chan struct{})
	go k8sInformerFactory.Start(stopCh)

	return newReconcileStatefulset(
		mgr,
		podInformer,
		pvcInformer,
		revInformer,
		kromeClient), nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("statefulset-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Statefulset
	err = c.Watch(&source.Kind{Type: &kromev1.Statefulset{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner Statefulset
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &kromev1.Statefulset{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileStatefulset implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileStatefulset{}

// ReconcileStatefulset reconciles a Statefulset object
type ReconcileStatefulset struct {
	mgr         manager.Manager
	scheme      *runtime.Scheme
	kromeClient *kromeclient.Client

	control    StatefulSetControlInterface
	podControl kubecontroller.PodControlInterface
}

func newReconcileStatefulset(
	mgr manager.Manager,
	podInformer coreinformers.PodInformer,
	//setInformer kromeinformers.StatefulsetInformer,
	pvcInformer coreinformers.PersistentVolumeClaimInformer,
	revInformer appsinformers.ControllerRevisionInformer,
	kromeClient *kromeclient.Client,
) *ReconcileStatefulset {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: kromeClient.K8sClient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "statefulset-controller"})

	r := &ReconcileStatefulset{
		mgr:         mgr,
		kromeClient: kromeClient,
		control: NewDefaultStatefulSetControl(
			NewRealStatefulPodControl(
				kromeClient.K8sClient,
				mgr,
				podInformer.Lister(),
				pvcInformer.Lister(),
				recorder,
			),
			NewRealStatefulSetStatusUpdater(
				kromeClient,
				nil,
			),
			history.NewHistory(kromeClient.K8sClient, revInformer.Lister()),
			recorder,
		),
		podControl: kubecontroller.RealPodControl{
			KubeClient: kromeClient.K8sClient,
			Recorder:   recorder,
		},
	}

	return r
}

// syncStatefulset syncs a tuple of (statefulset, []*v1.Pod).
func (r *ReconcileStatefulset) syncStatefulset(set *kromev1.Statefulset, pods []*corev1.Pod) error {
	klog.Infof("Syncing Statefulset %v/%v with %d pods", set.Namespace, set.Name, len(pods))
	if err := r.control.UpdateStatefulSet(set.DeepCopy(), pods); err != nil {
		return err
	}
	klog.Infof("Successfully synced StatefulSet %s/%s successful", set.Namespace, set.Name)
	return nil
}

// Reconcile reads that state of the cluster for a Statefulset object and makes changes based on the state read
// and what is in the Statefulset.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileStatefulset) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Statefulset")

	// Fetch the Statefulset instance
	set := &kromev1.Statefulset{}
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

	return reconcile.Result{}, r.syncStatefulset(set, pods)
}

// adoptOrphanRevisions adopts any orphaned ControllerRevisions matched by set's Selector.
func (r *ReconcileStatefulset) adoptOrphanRevisions(set *kromev1.Statefulset) error {
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
		fresh, err := r.kromeClient.KromeClient.AppsV1().Statefulsets(set.Namespace).Get(context.TODO(), set.Name, metav1.GetOptions{})
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

// getPodsForStatefulSet returns the Pods that a given Statefulset should manage.
// It also reconciles ControllerRef by adopting/orphaning.
//
// NOTE: Returned Pods are pointers to objects from the cache.
//       If you need to modify one, you need to copy it first.
func (r *ReconcileStatefulset) getPodsForStatefulSet(set *kromev1.Statefulset, selector labels.Selector) ([]*corev1.Pod, error) {
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
		fresh, err := r.kromeClient.KromeClient.AppsV1().Statefulsets(set.Namespace).Get(context.TODO(), set.Name, metav1.GetOptions{})
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
