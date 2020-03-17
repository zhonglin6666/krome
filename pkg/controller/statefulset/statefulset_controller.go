package statefulset

import (
	"context"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	kromev1 "github.com/zhonglin6666/krome/pkg/apis/apps/v1"
	kromeclient "github.com/zhonglin6666/krome/pkg/client"
	// kromelister "github.com/zhonglin6666/krome/pkg/client/listers/apps/v1"
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
	// TODO cache
	//cache := mgr.GetCache()
	//statefulsetInformer, err := cache.GetInformerForKind(controllerKind)
	//if err != nil {
	//	return nil, err
	//}
	//podInformer, err := cache.GetInformerForKind(podKind)
	//if err != nil {
	//	return nil, err
	//}
	//pvcInformer, err := cache.GetInformerForKind(pvcKind)
	//if err != nil {
	//	return nil, err
	//}
	//revInformer, err := cache.GetInformerForKind(controllerRevisionKind)
	//if err != nil {
	//	return nil, err
	//}
	//
	//statefulsetLister, err := kromelister.NewStatefulsetLister(statefulsetInformer.)

	kromeClient, err := kromeclient.NewClient(mgr)
	if err != nil {
		return nil, err
	}

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: kromeClient.K8sClient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "statefulset-controller"})

	return &ReconcileStatefulset{
		client:      mgr.GetClient(),
		scheme:      mgr.GetScheme(),
		kromeClient: kromeClient,
	}, nil
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
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client      client.Client
	scheme      *runtime.Scheme
	kromeClient *kromeclient.Client

	podControl controller.PodControlInterface
	// podLister is able to list/get pods from a shared informer's store
	podLister corelisters.PodLister
	// podListerSynced returns true if the pod shared informer has synced at least once
	podListerSynced cache.InformerSynced
	// setLister is able to list/get stateful sets from a shared informer's store
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
	instance := &kromev1.Statefulset{}
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

	// Define a new Pod object
	pod := newPodForCR(instance)

	// Set Statefulset instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, pod, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Check if this Pod already exists
	found := &corev1.Pod{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating a new Pod", "Pod.Namespace", pod.Namespace, "Pod.Name", pod.Name)
		err = r.client.Create(context.TODO(), pod)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Pod created successfully - don't requeue
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, err
	}

	// Pod already exists - don't requeue
	reqLogger.Info("Skip reconcile: Pod already exists", "Pod.Namespace", found.Namespace, "Pod.Name", found.Name)
	return reconcile.Result{}, nil
}

// newPodForCR returns a busybox pod with the same name/namespace as the cr
func newPodForCR(cr *kromev1.Statefulset) *corev1.Pod {
	labels := map[string]string{
		"app": cr.Name,
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-pod",
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "busybox",
					Image:   "busybox",
					Command: []string{"sleep", "3600"},
				},
			},
		},
	}
}
