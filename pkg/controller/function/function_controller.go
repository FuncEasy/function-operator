package function

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"reflect"

	funceasyv1 "github.com/FuncEasy/function-operator/pkg/apis/funceasy/v1"
	"github.com/FuncEasy/function-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_function")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Function Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileFunction{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("function-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Function
	err = c.Watch(&source.Kind{Type: &funceasyv1.Function{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner Function
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &funceasyv1.Function{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileFunction implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileFunction{}

// ReconcileFunction reconciles a Function object
type ReconcileFunction struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Function object and makes changes based on the state read
// and what is in the Function.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileFunction) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Function")

	// Fetch the Function instance
	instance := &funceasyv1.Function{}
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
	//pod := newPodForCR(instance)
	//
	//
	//// Set Function instance as the owner and controller
	//if err := controllerutil.SetControllerReference(instance, pod, r.scheme); err != nil {
	//	return reconcile.Result{}, err
	//}
	//
	//// Check if this Pod already exists
	//found := &corev1.Pod{}
	//err = r.client.Get(context.TODO(), types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, found)
	//if err != nil && errors.IsNotFound(err) {
	//	reqLogger.Info("Creating a new Pod", "Pod.Namespace", pod.Namespace, "Pod.Name", pod.Name)
	//	err = r.client.Create(context.TODO(), pod)
	//	if err != nil {
	//		return reconcile.Result{}, err
	//	}
	//
	//	// Pod created successfully - don't requeue
	//	return reconcile.Result{}, nil
	//} else if err != nil {
	//	return reconcile.Result{}, err
	//}
	//
	//// Pod already exists - don't requeue
	//reqLogger.Info("Skip reconcile: Pod already exists", "Pod.Namespace", found.Namespace, "Pod.Name", found.Name)

	err = r.ensureConfigMap(instance, reqLogger)

	if err != nil {
		return reconcile.Result{}, err
	}

	deployment, requeue ,err := r.ensureDeployment(instance, reqLogger)
	if requeue {
		return reconcile.Result{Requeue:requeue}, err
	}

	requeue, err = r.updateStatus(instance, deployment, reqLogger)
	if requeue {
		return reconcile.Result{Requeue:requeue}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileFunction) ensureDeployment(instance *funceasyv1.Function, reqLogger logr.Logger) (deployment * appsv1.Deployment, requeue bool, error error)  {
	deployFound := &appsv1.Deployment{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, deployFound)
	if err != nil {
		if errors.IsNotFound(err) {
			deployment := utils.NewDeploymentForCR(instance)
			if err := controllerutil.SetControllerReference(instance, deployment, r.scheme); err != nil {
				reqLogger.Error(err, "Failed to Set Deployment Reference", "Deployment.Namespace", instance.Namespace, "Deployment.Name", instance.Name)
				return deployFound, true, err
			}
			reqLogger.Info("Creating New Deployment", "Deployment.Namespace", instance.Namespace, "Deployment.Name", instance.Name)
			err = r.client.Create(context.TODO(), deployment)
			if err != nil {
				reqLogger.Error(err, "Failed to Create Deployment", "Deployment.Namespace", instance.Namespace, "Deployment.Name", instance.Name)
				return deployFound, true, err
			}
			reqLogger.Info("Create Deployment Success", "Deployment.Namespace", instance.Namespace, "Deployment.Name", instance.Name)
			return deployFound, true, nil
		} else {
			reqLogger.Error(err, "Failed to Get Deployment", "Deployment.Namespace", instance.Namespace, "Deployment.Name", instance.Name)
			return deployFound, true, err
		}
	}
	reqLogger.Info("Deployment already exists", "Deployment.Namespace", deployFound.Namespace, "Deployment.Name", deployFound.Name)
	reqLogger.Info("Check Deployment Update", "Deployment.Namespace", deployFound.Namespace, "Deployment.Name", deployFound.Name)
	size := instance.Spec.Size
	if *deployFound.Spec.Replicas != *size {
		deployFound.Spec.Replicas = size
		if err = r.client.Update(context.TODO(), deployFound); err != nil {
			reqLogger.Error(err, "Failed to update Deployment.", "Deployment.Namespace", deployFound.Namespace, "Deployment.Name", deployFound.Name)
			return deployFound, true, err
		}
		reqLogger.Info("Update Deployment Success", "Deployment.Namespace", deployFound.Namespace, "Deployment.Name", deployFound.Name)
		return deployFound,true, nil
	}
	reqLogger.Info("Deployment Already Updated", "Deployment.Namespace", deployFound.Namespace, "Deployment.Name", deployFound.Name)
	return deployFound, false, nil
}

func (r *ReconcileFunction) updateStatus(instance *funceasyv1.Function, deployment *appsv1.Deployment, reqLogger logr.Logger) (requeue bool, error error) {
	reqLogger.Info("UpdateStatus...", "Function.Namespace", instance.Namespace, "Function.Name", instance.Name)
	labels := utils.LabelsForFunctionCR(instance)
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(instance.Namespace),
		client.MatchingLabels(labels),
	}
	if err := r.client.List(context.TODO(), podList, listOpts...); err != nil{
		reqLogger.Error(err, "UpdateStatus: Failed to list pods.", "Pod.Namespace", instance.Namespace, "Pod.Label", labels)
		return true, err
	}

	var podListStatus []funceasyv1.PodsStatus
	for _, pod := range podList.Items {
		item := funceasyv1.PodsStatus{
			PodName:               pod.Name,
			PodPhase:              pod.Status.Phase,
			InitContainerStatuses: pod.Status.InitContainerStatuses,
			ContainerStatuses:     pod.Status.ContainerStatuses,
		}
		podListStatus = append(podListStatus, item)
	}

	if !reflect.DeepEqual(podListStatus, instance.Status.PodsStatus) {
		instance.Status.PodsStatus = podListStatus
		err := r.client.Status().Update(context.TODO(), instance)
		if err != nil {
			reqLogger.Error(err, "UpdateStatus: Failed to update Function status.", "Function.Namespace", instance.Namespace, "Function.Name", instance.Name)
			fmt.Println(err)
			return true, err
		}
	}
	reqLogger.Info("UpdateStatus success", "Function.Namespace", instance.Namespace, "Function.Name", instance.Name)
	return false, nil
}

func (r *ReconcileFunction) getRuntimeConfig(reqLogger logr.Logger) (*corev1.ConfigMap, error)  {
	runtimeConfig := &corev1.ConfigMap{}
	err := r.client.Get(context.TODO(), types.NamespacedName{
		Namespace: "funceasy",
		Name:      "funceasy-runtime",
	}, runtimeConfig)
	if err != nil {
		reqLogger.Error(err, "Failed to get Runtime Config.", "ConfigMap.Namespace", "funceasy", "ConfigMap.Name", "funceasy-runtime")
		return nil, err
	}
	return runtimeConfig, nil
}

func (r *ReconcileFunction) ensureConfigMap(instance *funceasyv1.Function, reqLogger logr.Logger) (error error) {
	configMapFound := &corev1.ConfigMap{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, configMapFound)
	if err != nil {
		if errors.IsNotFound(err) {
			runtimeConfigMap, err := r.getRuntimeConfig(reqLogger)
			if err != nil {
				reqLogger.Error(err, "Failed to Get RuntimeConfigMap", "ConfigMap.Namespace", instance.Namespace, "ConfigMap.Name", instance.Name)
				return err
			}
			configMap, err := utils.NewConfigMapForFunctionCR(instance, runtimeConfigMap)
			if err != nil {
				reqLogger.Error(err, "Failed to Set ConfigMap", "ConfigMap.Namespace", instance.Namespace, "ConfigMap.Name", instance.Name)
				return err
			}
			if err := controllerutil.SetControllerReference(instance, configMap, r.scheme); err != nil {
				reqLogger.Error(err, "Failed to Set ConfigMap Reference", "ConfigMap.Namespace", configMap.Namespace, "ConfigMap.Name", configMap.Name)
				return err
			}
			reqLogger.Info("Creating New ConfigMap", "ConfigMap.Namespace", configMap.Namespace, "ConfigMap.Name", configMap.Name)
			err = r.client.Create(context.TODO(), configMap)
			if err != nil {
				reqLogger.Error(err, "Failed to Create ConfigMap", "ConfigMap.Namespace", configMap.Namespace, "ConfigMap.Name", configMap.Name)
				return err
			}
			reqLogger.Info("Create ConfigMap Success", "ConfigMap.Namespace", configMap.Namespace, "ConfigMap.Name", configMap.Name)
			return nil
		} else {
			reqLogger.Error(err, "Failed to Get ConfigMap", "ConfigMap.Namespace", instance.Namespace, "ConfigMap.Name", instance.Name)
			return err
		}
	}
	return nil
}
