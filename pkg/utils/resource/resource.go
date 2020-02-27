package resource

import (
	"fmt"
	funceasyV1 "github.com/funceasy/function-operator/pkg/apis/funceasy/v1"
	"github.com/funceasy/function-operator/pkg/utils"
	funcEasyConfig "github.com/funceasy/function-operator/pkg/utils/config"
	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"path"
)

func NewConfigMapForFunctionCR(functionCR *funceasyV1.Function, runtimeConfig *funcEasyConfig.FunctionRuntimeConfig) (*coreV1.ConfigMap, error) {
	runtimeInfo, _, err := runtimeConfig.GetRuntime(functionCR.Spec.Runtime)
	if err != nil {
		return nil, err
	}
	filename, err := utils.GetFunctionSourceFileName(functionCR, runtimeInfo)
	if err != nil {
		return nil, err
	}
	data := map[string]string{
		"handler": functionCR.Spec.Handler,
		filename:  functionCR.Spec.Function,
	}
	if runtimeInfo.DepFileName != "" {
		data[runtimeInfo.DepFileName] = functionCR.Spec.Deps
	}
	configMap := &coreV1.ConfigMap{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      functionCR.Name,
			Namespace: functionCR.Namespace,
		},
		Data: data,
	}
	return configMap, nil
}

func NewDeploymentForFunctionCR(functionCR *funceasyV1.Function, runtimeConfig *funcEasyConfig.FunctionRuntimeConfig) (*appsV1.Deployment, error) {
	runtimeInfo, runtimeVersion, err := runtimeConfig.GetRuntime(functionCR.Spec.Runtime)
	if err != nil {
		return &appsV1.Deployment{}, err
	}
	labels := LabelsForFunctionCR(functionCR)
	replicas := functionCR.Spec.Size
	podSpec := &coreV1.PodSpec{}
	runtimeVolumeMount := RuntimeVolumeMountForFunctionCR(functionCR.Name)
	SourceVolumeMount := SourceVolumeMountForFunctionCR(functionCR.Name)
	err = PodSpecForFunctionCR(functionCR, runtimeInfo, runtimeVersion, podSpec, runtimeVolumeMount, SourceVolumeMount)
	if err != nil {
		return &appsV1.Deployment{}, err
	}
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
				Spec: *podSpec,
			},
		},
		Status: appsV1.DeploymentStatus{},
	}
	return deployment, nil
}

func NewServiceForFunctionCR(functionCR *funceasyV1.Function) *coreV1.Service {
	podPort := utils.PodPortsWithDefault(functionCR)
	podLabels := LabelsForFunctionCR(functionCR)
	service := &coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "function-" + functionCR.Name,
			Namespace: functionCR.Namespace,
			Labels:    podLabels,
		},
		Spec: coreV1.ServiceSpec{
			Selector: podLabels,
			Type:     coreV1.ServiceTypeClusterIP,
			Ports: []coreV1.ServicePort{
				coreV1.ServicePort{
					Name:       "function-port",
					Protocol:   coreV1.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.FromInt(int(podPort)),
				},
			},
		},
	}
	return service
}

func LabelsForFunctionCR(functionCR *funceasyV1.Function) map[string]string {
	return map[string]string{
		"app":    "funceasy_function",
		"funcId": functionCR.Name,
	}
}

func PodSpecForFunctionCR(functionCR *funceasyV1.Function, runtimeInfo funcEasyConfig.FunctionRuntimeInfo, runtimeVersion funcEasyConfig.FunctionRuntimeVersion, podSpec *coreV1.PodSpec, runtimeVolumeMount coreV1.VolumeMount, sourceVolumeMount coreV1.VolumeMount) error {
	runtimeVolume := coreV1.Volume{
		Name: runtimeVolumeMount.Name,
		VolumeSource: coreV1.VolumeSource{
			EmptyDir: &coreV1.EmptyDirVolumeSource{},
		},
	}

	sourceVolume := coreV1.Volume{
		Name: sourceVolumeMount.Name,
		VolumeSource: coreV1.VolumeSource{
			ConfigMap: &coreV1.ConfigMapVolumeSource{
				LocalObjectReference: coreV1.LocalObjectReference{
					Name: functionCR.Name,
				},
			},
		},
	}

	podSpec.Volumes = []coreV1.Volume{runtimeVolume, sourceVolume}

	filename, err := utils.GetFunctionSourceFileName(functionCR, runtimeInfo)
	if err != nil {
		return err
	}
	initContainers, err := InitContainersForPod(functionCR, filename, runtimeInfo, runtimeVersion, runtimeVolumeMount, sourceVolumeMount)
	if err != nil {
		return err
	}
	podSpec.InitContainers = initContainers

	imageInfo, err := runtimeVersion.GetImage(funcEasyConfig.RUN_STAGE)
	if err != nil {
		return err
	}

	moduleName, handlerName, err := utils.SplitHandlerName(functionCR.Spec.Handler)
	if err != nil {
		return err
	}
	env := []coreV1.EnvVar{
		{
			Name:  "FUNCTION_HANDLER",
			Value: handlerName,
		},
		{
			Name:  "FUNCTION_MODULE_NAME",
			Value: moduleName,
		},
		{
			Name:  "FUNCTION_RUNTIME",
			Value: functionCR.Spec.Runtime,
		},
		{
			Name:  "FUNCTION_TIMEOUT",
			Value: functionCR.Spec.Timeout,
		},
		{
			Name:  "DATA_SOURCE_ID",
			Value: functionCR.Spec.DataSource,
		},
		{
			Name:  "DATA_SOURCE_TOKEN",
			Value: functionCR.Spec.DataServiceToken,
		},
		{
			Name: "DATA_SOURCE_SERVICE",
			Value: "data-source-service",
		},
	}
	mainPort := utils.PodPortsWithDefault(functionCR)
	ports := []coreV1.ContainerPort{
		coreV1.ContainerPort{
			Name:          "main",
			ContainerPort: mainPort,
		},
	}

	podSpec.Containers = []coreV1.Container{
		coreV1.Container{
			Name:            "run-" + functionCR.Name,
			Image:           imageInfo.Image,
			Ports:           ports,
			Env:             env,
			VolumeMounts:    []coreV1.VolumeMount{runtimeVolumeMount, sourceVolumeMount},
			LivenessProbe:   utils.PodLivenessProbe(int(mainPort)),
			ImagePullPolicy: coreV1.PullAlways,
		},
	}

	return nil
}

func InitContainersForPod(functionCR *funceasyV1.Function, sourceFilename string, runtimeInfo funcEasyConfig.FunctionRuntimeInfo, runtimeVersion funcEasyConfig.FunctionRuntimeVersion, runtimeVolumeMount coreV1.VolumeMount, sourceVolumeMount coreV1.VolumeMount) ([]coreV1.Container, error) {
	var initContainers []coreV1.Container
	prepareInitContainer := GetPrepareInitContainer(functionCR, sourceFilename, runtimeInfo, runtimeVolumeMount, sourceVolumeMount)
	if prepareInitContainer != nil {
		initContainers = append(initContainers, *prepareInitContainer)
	}
	if functionCR.Spec.Deps != "none" {
		installInitContainer := GetInstallInitContainer(runtimeInfo, runtimeVersion, runtimeVolumeMount, sourceVolumeMount)
		if installInitContainer != nil {
			initContainers = append(initContainers, *installInitContainer)
		}
	}
	compileContainer, err := GetCompileInitContainer(functionCR, runtimeVersion, runtimeVolumeMount)
	if err != nil {
		return initContainers, err
	}
	if compileContainer != nil {
		initContainers = append(initContainers, *compileContainer)
	}
	return initContainers, nil
}

func GetPrepareInitContainer(functionCR *funceasyV1.Function, sourceFilename string, runtimeInfo funcEasyConfig.FunctionRuntimeInfo, runtimeVolumeMount coreV1.VolumeMount, sourceVolumeMount coreV1.VolumeMount) *coreV1.Container {
	prepareContainerCMD := ""
	sourceFile := path.Join(sourceVolumeMount.MountPath, sourceFilename)
	if functionCR.Spec.ContentType == "zip" {
		decodedFile := path.Join("/tmp", sourceFilename+".decoded")
		prepareContainerCMD = utils.AppendCommand(prepareContainerCMD, fmt.Sprintf("base64 -d < %s > %s", sourceFile, decodedFile))
		prepareContainerCMD = utils.AppendCommand(prepareContainerCMD, fmt.Sprintf("unzip -o %s -d %s", decodedFile, runtimeVolumeMount.MountPath))
	} else {
		targetFile := path.Join(runtimeVolumeMount.MountPath, sourceFilename)
		prepareContainerCMD = utils.AppendCommand(prepareContainerCMD, fmt.Sprintf("cp %s %s", sourceFile, targetFile))
	}

	if runtimeInfo.DepFileName != "" {
		depsFile := path.Join(sourceVolumeMount.MountPath, runtimeInfo.DepFileName)
		prepareContainerCMD = utils.AppendCommand(prepareContainerCMD, fmt.Sprintf("cp %s %s", depsFile, runtimeVolumeMount.MountPath))
	}
	return &coreV1.Container{
		Name:         "prepare",
		Image:        "ziqiancheng/unzip",
		Command:      []string{"sh", "-c"},
		Args:         []string{prepareContainerCMD},
		VolumeMounts: []coreV1.VolumeMount{runtimeVolumeMount, sourceVolumeMount},
	}
}

func GetInstallInitContainer(runtimeInfo funcEasyConfig.FunctionRuntimeInfo, runtimeVersion funcEasyConfig.FunctionRuntimeVersion, runtimeVolumeMount coreV1.VolumeMount, sourceVolumeMount coreV1.VolumeMount) *coreV1.Container {
	depsFile := path.Join(sourceVolumeMount.MountPath, runtimeInfo.DepFileName)
	ImageInfo, _ := runtimeVersion.GetImage(funcEasyConfig.INSTALL_STAGE)
	if ImageInfo == nil {
		return nil
	}
	installCommand := ImageInfo.CMD

	env := []coreV1.EnvVar{
		{
			Name:  "FUNCEASY_INSTALL_VOLUME",
			Value: runtimeVolumeMount.MountPath,
		},
		{
			Name:  "FUNCEASY_DEPS_FILE",
			Value: depsFile,
		},
	}
	env = append(env, utils.ParseEnvToSlice(ImageInfo.Env)...)

	return &coreV1.Container{
		Name:         "install",
		Image:        ImageInfo.Image,
		Command:      []string{"sh", "-c"},
		Args:         []string{installCommand},
		WorkingDir:   runtimeVolumeMount.MountPath,
		Env:          env,
		VolumeMounts: []coreV1.VolumeMount{runtimeVolumeMount},
	}
}

func GetCompileInitContainer(functionCR *funceasyV1.Function, runtimeVersion funcEasyConfig.FunctionRuntimeVersion, runtimeVolumeMount coreV1.VolumeMount) (*coreV1.Container, error) {
	_, handlerName, err := utils.SplitHandlerName(functionCR.Spec.Handler)
	if err != nil {
		return nil, err
	}
	ImageInfo, _ := runtimeVersion.GetImage(funcEasyConfig.COMPILE_STAGE)
	if ImageInfo == nil {
		return nil, nil
	}
	installCommand := ImageInfo.CMD

	env := []coreV1.EnvVar{
		{
			Name:  "FUNCEASY_INSTALL_VOLUME",
			Value: runtimeVolumeMount.MountPath,
		},
		{
			Name:  "FUNCEASY_FUNCTION_NAME",
			Value: handlerName,
		},
	}
	env = append(env, utils.ParseEnvToSlice(ImageInfo.Env)...)

	return &coreV1.Container{
		Name:         "compile",
		Image:        ImageInfo.Image,
		Command:      []string{"sh", "-c"},
		Args:         []string{installCommand},
		WorkingDir:   runtimeVolumeMount.MountPath,
		Env:          env,
		VolumeMounts: []coreV1.VolumeMount{runtimeVolumeMount},
	}, nil
}

func RuntimeVolumeMountForFunctionCR(name string) coreV1.VolumeMount {
	return coreV1.VolumeMount{
		Name:      name,
		ReadOnly:  false,
		MountPath: "/funceasy",
	}
}

func SourceVolumeMountForFunctionCR(name string) coreV1.VolumeMount {
	return coreV1.VolumeMount{
		Name:      name + "-src",
		ReadOnly:  false,
		MountPath: "/src",
	}
}
