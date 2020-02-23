package function

import (
	"context"
	funceasyv1 "github.com/funceasy/function-operator/pkg/apis/funceasy/v1"
	"github.com/funceasy/function-operator/pkg/utils"
	funcEasyConfig "github.com/funceasy/function-operator/pkg/utils/config"
	FunctionResource "github.com/funceasy/function-operator/pkg/utils/resource"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"regexp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"
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
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &appsv1.ReplicaSet{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileFunction implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileFunction{}
var configUpdatedTime time.Time = time.Now()
var requeueForceUpdate = false

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
	configUpdatedTime = time.Now()
}

// Reconcile reads that state of the cluster for a Function object and makes changes based on the state read
// and what is in the Function.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileFunction) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	if r.config == nil || r.runtimeConfig == nil || time.Since(configUpdatedTime) > 30*time.Second {
		logrus.Infof("Read GlobalConfig...")
		r.configInit()
		logrus.Infof("Read GlobalConfig Success")
	} else {
		logrus.Warn("Read GlobalConfig From Cache")
	}
	reqLogger := logrus.WithFields(logrus.Fields{
		"Request.Namespace": request.Namespace,
		"Request.Name":      request.Name,
	})
	r.logger = reqLogger
	reqLogger.Info("Reconciling Function")

	instance := &funceasyv1.Function{}
	theRegexp := regexp.MustCompile(`^(\w+)-(\w+)$`)
	params := theRegexp.FindStringSubmatch(request.Name)
	if params != nil {
		functionCRName := params[1]
		err := r.client.Get(context.TODO(), types.NamespacedName{
			Namespace: request.Namespace,
			Name:      functionCRName,
		}, instance)
		if err != nil {
			if errors.IsNotFound(err) {
				return reconcile.Result{}, nil
			}
			return reconcile.Result{}, err
		}
	} else {
		err := r.client.Get(context.TODO(), request.NamespacedName, instance)
		if err != nil {
			if errors.IsNotFound(err) {
				return reconcile.Result{}, nil
			}
			return reconcile.Result{}, err
		}
	}

	configMap, requeue, err := r.ensureConfigMap(instance)
	if requeue {
		return reconcile.Result{Requeue: true}, err
	}

	_, requeue, err = r.ensureDeployment(instance)
	if requeue {
		return reconcile.Result{Requeue: true}, err
	}

	requeue, err = r.ensureService(instance)
	if requeue {
		return reconcile.Result{Requeue: true}, err
	}

	err = r.checkUpdate(instance, configMap)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.updateStatus(instance)
	if err != nil {
		r.logger.Warn("Failed Update Status -> Requeue ")
		return reconcile.Result{Requeue: true}, nil
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
				r.logger.Error("[Deployment] Failed to Set Deployment ")
			}
			if err := controllerutil.SetControllerReference(instance, deployment, r.scheme); err != nil {
				r.logger.Error("[Deployment] Failed to Set Deployment Reference")
				return deployFound, true, err
			}
			r.logger.Info("[Deployment] Creating New Deployment")
			err = r.client.Create(context.TODO(), deployment)
			if err != nil {
				r.logger.Error("[Deployment] Failed to Create Deployment")
				return deployFound, true, err
			}
			r.logger.Info("[Deployment] Create Deployment Success")
			return deployment, true, nil
		} else {
			r.logger.Error("[Deployment] Failed to Get Deployment")
			return deployFound, true, err
		}
	}
	r.logger.Info("[Deployment] Deployment already exists")
	return deployFound, false, nil
}

func (r *ReconcileFunction) updateStatus(instance *funceasyv1.Function) error {
	r.logger.Info("[UpdateStatus] UpdateStatus...")
	labels := FunctionResource.LabelsForFunctionCR(instance)
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(instance.Namespace),
		client.MatchingLabels(labels),
	}
	if err := r.client.List(context.TODO(), podList, listOpts...); err != nil {
		r.logger.Error(err, "[UpdateStatus] Failed to list pods.")
		return err
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
			r.logger.Error("[UpdateStatus] Failed to update Function status.")
			return err
		}
	}
	r.logger.Info("[UpdateStatus] UpdateStatus success")
	return nil
}

func (r *ReconcileFunction) ensureConfigMap(instance *funceasyv1.Function) (configMap *corev1.ConfigMap, requeue bool, error error) {
	configMapFound := &corev1.ConfigMap{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, configMapFound)
	if err != nil {
		if errors.IsNotFound(err) {
			runtimeConfigMap := r.runtimeConfig
			configMap, err := FunctionResource.NewConfigMapForFunctionCR(instance, runtimeConfigMap)
			if err != nil {
				r.logger.Error("[ConfigMap] Failed to Set ConfigMap: ", err)
				return nil, true, err
			}
			if err := controllerutil.SetControllerReference(instance, configMap, r.scheme); err != nil {
				r.logger.Error("[ConfigMap] Failed to Set ConfigMap Reference ", err)
				return nil, true, err
			}
			r.logger.Info("[ConfigMap] Creating New ConfigMap")
			err = r.client.Create(context.TODO(), configMap)
			if err != nil {
				r.logger.Error("[ConfigMap] Failed to Create ConfigMap: ", err)
				return nil, true, err
			}
			r.logger.Info("[ConfigMap] Create ConfigMap Success: ")
			return nil, true, nil
		} else {
			r.logger.Error("[ConfigMap] Failed to Get ConfigMap: ", err)
			return nil, true, err
		}
	}
	return configMapFound, false, nil
}

func (r *ReconcileFunction) ensureService(instance *funceasyv1.Function) (requeue bool, error error) {
	serviceFound := &corev1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{
		Namespace: instance.Namespace,
		Name:      "function-" + instance.Name,
	}, serviceFound)
	if err != nil {
		if errors.IsNotFound(err) {
			service := FunctionResource.NewServiceForFunctionCR(instance)
			if err := controllerutil.SetControllerReference(instance, service, r.scheme); err != nil {
				r.logger.Error("[Service] Failed to Set Service Reference ", err)
				return true, err
			}
			r.logger.Info("[Service] Creating New Service")
			err = r.client.Create(context.TODO(), service)
			if err != nil {
				r.logger.Error("[Service] Failed to Create Service: ", err)
				return true, err
			}
			r.logger.Info("[Service] Create Service Success: ")
			return true, nil
		} else {
			r.logger.Error("[Service] Failed to Get Service: ", err)
			return true, err
		}
	}
	return false, nil
}

func (r *ReconcileFunction) checkUpdate(instance *funceasyv1.Function, configMapFound *corev1.ConfigMap) error {
	r.logger.Info("[Update] Check Update...")
	runtimeConfigMap := r.runtimeConfig
	newConfigMap, err := FunctionResource.NewConfigMapForFunctionCR(instance, runtimeConfigMap)
	if err != nil {
		r.logger.Error(err, "[Update] Get ConfigMap Failed.")
	}

	deployment := &appsv1.Deployment{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, deployment)
	if err != nil {
		r.logger.Error("[Update] Get Deployment Failed")
		return err
	}

	if !reflect.DeepEqual(newConfigMap.Data, configMapFound.Data) || requeueForceUpdate {
		if requeueForceUpdate {
			r.logger.Info("[Update] Requeue Forced ")
		}
		configMapFound.Data = newConfigMap.Data
		r.logger.Info("[Update ConfigMap] Update ConfigMap...")
		err = r.client.Update(context.TODO(), configMapFound)
		if err != nil {
			r.logger.Error("[Update ConfigMap] Failed to Update ConfigMap")
			return err
		}
		r.logger.Info("[Update ConfigMap] Update ConfigMap Complete")
		r.logger.Infof("[Update Deployment Force] Update Deployment Force... ")
		r.logger.Infof("[Update Deployment Force] Update Deployment Force: Replicas %d->%d", *deployment.Spec.Replicas, 0)
		newDeployment, err := FunctionResource.NewDeploymentForFunctionCR(instance, r.runtimeConfig)
		if err != nil {
			r.logger.Error("[Update Deployment Force] Failed to Set New Deployment ")
			return err
		}
		r.logger.Info("[Update Deployment Force] Updating Deployment...")
		deployment.Spec.Replicas = utils.Int32Ptr(0)
		deployment.Spec.Template = newDeployment.Spec.Template
		err = r.client.Update(context.TODO(), deployment)
		if err != nil {
			requeueForceUpdate = true
			r.logger.Error("[Update Deployment Force] Failed to Force Update Deployment")
			return err
		}
		requeueForceUpdate = false
	} else {
		r.logger.Info("[Update Deployment] Check Deployment Update")
		needUpdate, err := utils.DeploymentUpdate(instance, deployment)
		if err != nil {
			r.logger.Error(err, "[Update Deployment] Check Deployment Failed.")
		}
		if needUpdate {
			r.logger.Info("[Update Deployment] Deployment Updating...")
			if err = r.client.Update(context.TODO(), deployment); err != nil {
				r.logger.Error(err, "Failed to update Deployment.")
				return err
			}
			r.logger.Info("[Update Deployment] Update Deployment Success")
		}
		r.logger.Info("[Update Deployment] Deployment Already Updated")
	}
	r.logger.Info("[Update] Check Update: Complete")
	return nil
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
