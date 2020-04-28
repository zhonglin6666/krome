package deployment

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/record"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	kromev1 "krome.io/krome/pkg/apis/apps/v1"
	kromeclient "krome.io/krome/pkg/client/clientset/versioned"
)

var (
	// controllerKind contains the schema.GroupVersionKind for StatefulSet type.
	controllerKind = kromev1.SchemeGroupVersion.WithKind("Deployment")
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Deployment Controller and adds it to the Manager. The Manager will set fields on the Controller
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
	//k8sClient, err := k8sclient.NewForConfig(mgr.GetConfig())
	//if err != nil {
	//	return nil, err
	//}

	kromeClient, err := kromeclient.NewForConfig(mgr.GetConfig())
	if err != nil {
		return nil, err
	}

	return &ReconcileDeployment{
		client:      mgr.GetClient(),
		scheme:      mgr.GetScheme(),
		kromeClient: kromeClient,
		rsControl: RealRSControl{
			KubeClient: kromeClient,
		},
		recorder: mgr.GetEventRecorderFor("deployment"),
	}, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("deployment-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Deployment
	err = c.Watch(&source.Kind{Type: &kromev1.Deployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Pods and requeue the owner Deployment
	err = c.Watch(&source.Kind{Type: &kromev1.ReplicaSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &kromev1.Deployment{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileDeployment implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileDeployment{}

// ReconcileDeployment reconciles a Deployment object
type ReconcileDeployment struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client      client.Client
	scheme      *runtime.Scheme
	kromeClient *kromeclient.Clientset

	// rsControl is used for adopting/releasing replica sets.
	rsControl kubecontroller.RSControlInterface
	recorder  record.EventRecorder
}

// Reconcile reads that state of the cluster for a Deployment object and makes changes based on the state read
// and what is in the Deployment.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileDeployment) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Deployment instance
	instance := &kromev1.Deployment{}
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

	// List ReplicaSets owned by this Deployment, while reconciling ControllerRef
	// through adotpion/orphaning
	rsList, err := r.getReplicaSetsForDeployment(instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	if instance.DeletionTimestamp != nil {
		return reconcile.Result{}, r.syncStatusOnly(instance, rsList)
	}

	// Update deployment conditions with an Unknown condition when pausing/resuming
	// a deployment. In this way, we can be sure that we won't timeout when a user
	// resumes a Deployment with a set progressDeadlineSeconds.
	if err = r.checkPausedConditions(instance); err != nil {
		return reconcile.Result{}, err
	}

	if instance.Spec.Paused {
		return reconcile.Result{}, r.sync(instance, rsList)
	}

	// rollback is not re-entrant in case the underlying replica sets are updated with a new
	// revision so we should ensure that we won't proceed to update replica sets until we
	// make sure that the deployment has cleaned up its rollback spec in subsequent enqueues.
	if getRollbackTo(instance) != nil {
		return reconcile.Result{}, r.rollback(instance, rsList)
	}

	scalingEvent, err := r.isScalingEvent(instance, rsList)
	if err != nil {
		return reconcile.Result{}, err
	}
	if scalingEvent {
		return reconcile.Result{}, r.sync(instance, rsList)
	}

	switch instance.Spec.Strategy.Type {
	case kromev1.RecreateDeploymentStrategyType:
		return reconcile.Result{}, r.rolloutRecreate(instance, rsList, podMap)
	case kromev1.RollingUpdateDeploymentStrategyType:
		return reconcile.Result{}, r.rolloutRolling(instance, rsList)
	}

	return reconcile.Result{}, fmt.Errorf("unexpected deployment strategy type: %s", instance.Spec.Strategy.Type)
}

// getReplicaSetsForDeployment uses ControllerRefManager to reconcile
// ControllerRef by adopting and orphaning.
// It returns the list of ReplicaSets that this Deployment should manage.
func (r *ReconcileDeployment) getReplicaSetsForDeployment(d *kromev1.Deployment) ([]*kromev1.ReplicaSet, error) {
	selector, err := metav1.LabelSelectorAsSelector(d.Spec.Selector)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("error converting Deployment %v selector: %v", d.Name, err))
		// This is a non-transient error, so don't retry.
		return nil, nil
	}

	// List all pods to include the pods that don't match the selector anymore but
	// has a ControllerRef pointing to this Deployment.
	var rsList = kromev1.ReplicaSetList{}
	if err := r.client.List(context.TODO(), &rsList, &client.ListOptions{LabelSelector: selector}); err != nil {
		return nil, err
	}

	items := make([]*kromev1.ReplicaSet, len(rsList.Items))
	for i, rs := range rsList.Items {
		items[i] = rs.DeepCopy()
	}

	// If any adoptions are attempted, we should first recheck for deletion with
	// an uncached quorum read sometime after listing ReplicaSets (see #42639).
	canAdoptFunc := kubecontroller.RecheckDeletionTimestamp(func() (metav1.Object, error) {
		fresh, err := r.kromeClient.AppsV1().Deployments(d.Namespace).Get(context.TODO(), d.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if fresh.UID != d.UID {
			return nil, fmt.Errorf("original Deployment %v/%v is gone: got uid %v, wanted %v", d.Namespace, d.Name, fresh.UID, d.UID)
		}
		return fresh, nil
	})

	cm := NewReplicaSetControllerRefManager(r.rsControl, d, selector, controllerKind, canAdoptFunc)
	return cm.ClaimReplicaSets(items)
}
