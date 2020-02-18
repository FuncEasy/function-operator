package function

import (
	"context"
	"fmt"
	funceasyv1 "github.com/FuncEasy/function-operator/pkg/apis/funceasy/v1"
	funcEasyConfig "github.com/FuncEasy/function-operator/pkg/utils/config"
	FunctionResource "github.com/FuncEasy/function-operator/pkg/utils/resource"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

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
	client        client.Client
	scheme        *runtime.Scheme
	config        *corev1.ConfigMap
	runtimeConfig *funcEasyConfig.FunctionRuntimeConfig
	logger        *logrus.Entry
}

func (r *ReconcileFunction) configInit() {
	logrus.Info("Read Global Config... ")
	config, err := getGlobalConfig(r.client)
	if err != nil {
		logrus.Fatal("Read Global Config Failed", err)
	}
	runtimeConfig := funcEasyConfig.NewFunctionRuntimeConfig(config)
	runtimeConfig.ReadRuntimeConfig()
	r.config = config
	r.runtimeConfig = runtimeConfig
}

// Reconcile reads that state of the cluster for a Function object and makes changes based on the state read
// and what is in the Function.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileFunction) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	if r.config == nil || r.runtimeConfig == nil {
		r.configInit()
	}
	logrus.Infof("Read GlobalConfig Success")
	reqLogger := logrus.WithFields(logrus.Fields{
		"Request.Namespace": request.Namespace,
		"Request.Name":      request.Name,
	})
	r.logger = reqLogger
	reqLogger.Info("Reconciling Function")

	instance := &funceasyv1.Function{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	err = r.ensureConfigMap(instance)

	if err != nil {
		return reconcile.Result{}, err
	}

	deployment, requeue ,err := r.ensureDeployment(instance)
	if requeue {
		return reconcile.Result{Requeue:requeue}, err
	}

	err = r.ensureService(instance)

	if err != nil {
		return reconcile.Result{}, err
	}

	requeue, err = r.updateStatus(instance, deployment)
	if requeue {
		return reconcile.Result{Requeue:requeue}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileFunction) ensureDeployment(instance *funceasyv1.Function) (deployment *appsv1.Deployment, requeue bool, error error) {
	deployFound := &appsv1.Deployment{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, deployFound)
	if err != nil {
		if errors.IsNotFound(err) {
			deployment, err := FunctionResource.NewDeploymentForFunctionCR(instance, r.runtimeConfig)
			if err != nil {
				r.logger.Error("Failed to Set Deployment ")
			}
			if err := controllerutil.SetControllerReference(instance, deployment, r.scheme); err != nil {
				r.logger.Error("Failed to Set Deployment Reference")
				return deployFound, true, err
			}
			r.logger.Info("Creating New Deployment")
			err = r.client.Create(context.TODO(), deployment)
			if err != nil {
				r.logger.Error("Failed to Create Deployment")
				return deployFound, true, err
			}
			r.logger.Info("Create Deployment Success")
			return deployFound, true, nil
		} else {
			r.logger.Error("Failed to Get Deployment", "Deployment.Namespace")
			return deployFound, true, err
		}
	}
	r.logger.Info("Deployment already exists")
	r.logger.Info("Check Deployment Update")
	size := instance.Spec.Size
	if *deployFound.Spec.Replicas != *size {
		deployFound.Spec.Replicas = size
		if err = r.client.Update(context.TODO(), deployFound); err != nil {
			r.logger.Error(err, "Failed to update Deployment.")
			return deployFound, true, err
		}
		r.logger.Info("Update Deployment Success")
		return deployFound, true, nil
	}
	r.logger.Info("Deployment Already Updated")
	return deployFound, false, nil
}

func (r *ReconcileFunction) updateStatus(instance *funceasyv1.Function, deployment *appsv1.Deployment) (requeue bool, error error) {
	r.logger.Info("UpdateStatus...")
	labels := FunctionResource.LabelsForFunctionCR(instance)
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(instance.Namespace),
		client.MatchingLabels(labels),
	}
	if err := r.client.List(context.TODO(), podList, listOpts...); err != nil {
		r.logger.Error(err, "UpdateStatus: Failed to list pods.")
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
			r.logger.Error("UpdateStatus: Failed to update Function status.")
			fmt.Println(err)
			return true, err
		}
	}
	r.logger.Info("UpdateStatus success")
	return false, nil
}

func getGlobalConfig(c client.Client) (*corev1.ConfigMap, error) {
	config := &corev1.ConfigMap{}
	err := c.Get(context.TODO(), types.NamespacedName{
		Namespace: "funceasy",
		Name:      "funceasy-config",
	}, config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func (r *ReconcileFunction) ensureConfigMap(instance *funceasyv1.Function) error {
	configMapFound := &corev1.ConfigMap{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, configMapFound)
	if err != nil {
		if errors.IsNotFound(err) {
			runtimeConfigMap := r.runtimeConfig
			configMap, err := FunctionResource.NewConfigMapForFunctionCR(instance, runtimeConfigMap)
			if err != nil {
				r.logger.Error("Failed to Set ConfigMap: ", err)
				return err
			}
			if err := controllerutil.SetControllerReference(instance, configMap, r.scheme); err != nil {
				r.logger.Error("Failed to Set ConfigMap Reference ", err)
				return err
			}
			r.logger.Info("Creating New ConfigMap")
			err = r.client.Create(context.TODO(), configMap)
			if err != nil {
				r.logger.Error("Failed to Create ConfigMap: ", err)
				return err
			}
			r.logger.Info("Create ConfigMap Success: ")
			return nil
		} else {
			r.logger.Error("Failed to Get ConfigMap: ", err)
			return err
		}
	}
	return nil
}

func (r *ReconcileFunction) ensureService(instance *funceasyv1.Function) error {
	serviceFound := &corev1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{
		Namespace: instance.Namespace,
		Name:      "function-" + instance.Name,
	}, serviceFound)
	if err != nil {
		if errors.IsNotFound(err) {
			service := FunctionResource.NewServiceForFunctionCR(instance)
			if err := controllerutil.SetControllerReference(instance, service, r.scheme); err != nil {
				r.logger.Error("Failed to Set Service Reference ", err)
				return err
			}
			r.logger.Info("Creating New Service")
			err = r.client.Create(context.TODO(), service)
			if err != nil {
				r.logger.Error("Failed to Create Service: ", err)
				return err
			}
			r.logger.Info("Create Service Success: ")
			return nil
		} else {
			r.logger.Error("Failed to Get Service: ", err)
			return err
		}
	}
	return nil
}