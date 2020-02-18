package utils

import (
	"fmt"
	v1 "github.com/FuncEasy/function-operator/pkg/apis/funceasy/v1"
	funcEasyConfig "github.com/FuncEasy/function-operator/pkg/utils/config"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"strings"
)

func SplitHandlerName(handler string) (string, string, error) {
	str := strings.Split(handler, ".")
	if len(str) != 2 {
		return "", "", fmt.Errorf("failed: incorrect handler format ")
	}

	return str[0], str[1], nil
}

func GetFunctionSourceFileName(functionCR *v1.Function, runtimeInfo funcEasyConfig.FunctionRuntimeInfo) (string, error) {
	fileName, _, err := SplitHandlerName(functionCR.Spec.Handler)
	if err != nil {
		return "", err
	}
	if functionCR.Spec.ContentType == "text" {
		return fileName + runtimeInfo.FileSuffix, nil
	} else if functionCR.Spec.ContentType == "zip" {
		return fileName + ".zip", nil
	} else {
		return fileName, nil
	}
}

func ParseEnvToSlice(env map[string]string) []coreV1.EnvVar {
	var res []coreV1.EnvVar
	for key, value := range env {
		res = append(res, coreV1.EnvVar{Name: key, Value: value})
	}
	return res
}

func PodPortsWithDefault(functionCR *v1.Function) int32 {
	if functionCR.Spec.ExposedPort == 0 {
		return int32(8080)
	}
	return functionCR.Spec.ExposedPort
}

func PodLivenessProbe(port int) *coreV1.Probe {
	livenessProbe := &coreV1.Probe{
		Handler: coreV1.Handler{
			HTTPGet: &coreV1.HTTPGetAction{
				Path: "/health",
				Port: intstr.FromInt(port),
			},
		},
		InitialDelaySeconds: int32(5),
		PeriodSeconds:       int32(30),
	}
	return livenessProbe
}

func AppendCommand(originCommand string, cmd ...string) string {
	if len(originCommand) > 0 {
		return fmt.Sprintf("%s && %s", originCommand, strings.Join(cmd, "&&"))
	}
	return strings.Join(cmd, "&&")
}

func Int32Ptr(i int32) *int32 { return &i }
