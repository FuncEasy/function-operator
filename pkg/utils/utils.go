package utils

import (
	"encoding/json"
	"errors"
	funceasyV1 "github.com/FuncEasy/function-operator/pkg/apis/funceasy/v1"
	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)
type RuntimeConfig struct{
	Suffix string `json:"suffix"`
	BuildImage string `json:"buildImage"`
	RunImage string `json:"runImage"`
}
func NewDeploymentForCR(functionCR *funceasyV1.Function) *appsV1.Deployment {
	labels := LabelsForFunctionCR(functionCR)
	replicas := functionCR.Spec.Size
	deployment := &appsV1.Deployment{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      functionCR.Name,
			Namespace: functionCR.Namespace,
			Labels:    labels,
		},
		Spec: appsV1.DeploymentSpec{
			Replicas: replicas,
			Selector: &metaV1.LabelSelector{
				MatchLabels: labels,
			},
			Template: coreV1.PodTemplateSpec{
				ObjectMeta: metaV1.ObjectMeta{
					Labels: labels,
				},
				Spec: coreV1.PodSpec{
					Volumes: []coreV1.Volume{
						{
							Name: "empty",
							VolumeSource: coreV1.VolumeSource{
								ConfigMap: &coreV1.ConfigMapVolumeSource{
									LocalObjectReference: coreV1.LocalObjectReference{Name: functionCR.Name},
									DefaultMode: int32Ptr(0777),
								},
							},
						},
					},
					InitContainers: []coreV1.Container{
						{
							Name:    "prepare",
							Image:   "busybox",
							Command: []string{"sh", "-c"},
							Args: []string{"sleep 10"},
							VolumeMounts: []coreV1.VolumeMount{
								{
									Name:      "empty",
									MountPath: "/write_test",
								},
							},
						},
					},
					Containers: []coreV1.Container{
						{
							Name:  "function",
							Image: "ziqiancheng/fusion-gateway",
							VolumeMounts: []coreV1.VolumeMount{
								{
									Name:      "empty",
									MountPath: "/write_test",
								},
							},
						},
					},
				},
			},
		},
		Status: appsV1.DeploymentStatus{},
	}
	return deployment
}

func NewConfigMapForFunctionCR(functionCR *funceasyV1.Function, runtimeConfigMap *coreV1.ConfigMap) (*coreV1.ConfigMap, error) {
	var data map[string]string
	var binaryData map[string][]byte
	runtimeConfig, err := getRuntimeConfig(functionCR.Spec.Runtime, runtimeConfigMap)
	if err != nil {
		return nil, err
	}
	if functionCR.Spec.ContentType == "text" {
		data = map[string]string{
			"main."+runtimeConfig.Suffix: functionCR.Spec.Function,
		}
	} else if functionCR.Spec.ContentType == "zip" {
		binaryData = map[string][]byte{
			"bundle.zip": []byte(functionCR.Spec.Function),
		}
	}
	configMap := &coreV1.ConfigMap{
		ObjectMeta: metaV1.ObjectMeta{
			Name: functionCR.Name,
			Namespace: functionCR.Namespace,
		},
		Data:       data,
		BinaryData: binaryData,
	}
	return configMap, nil
}

func LabelsForFunctionCR(functionCR *funceasyV1.Function) map[string]string {
	return map[string]string{
		"app":      "funceasy_function",
		"function": functionCR.Spec.Identifier,
	}
}

func getRuntimeConfig(runtime string, runtimeConfigMap *coreV1.ConfigMap) (*RuntimeConfig, error)  {
	rc := &RuntimeConfig{}
	if _, ok := runtimeConfigMap.Data[runtime]; !ok {
		return nil, errors.New("Runtime Not Found")
	} else {
		err := json.Unmarshal([]byte(runtimeConfigMap.Data[runtime]), rc)
		if err != nil {
			return nil, err
		}
		return rc, nil
	}
}
func int32Ptr(i int32) *int32 { return &i }