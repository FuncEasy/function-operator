package config

import (
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"strings"
)

type FunctionRuntimeConfig struct {
	globalConfig *v1.ConfigMap
	RuntimeList  []FunctionRuntimeInfo
}

type FunctionRuntimeVersion struct {
	Version string  `json:"version"`
	Images  []Image `json:"images"`
}

type Image struct {
	Stage string            `json:"stage"`
	Image string            `json:"image"`
	CMD   string            `json:"cmd,omitempty"`
	Env   map[string]string `json:"env,omitempty"`
}

const (
	INSTALL_STAGE string = "install"
	COMPILE_STAGE string = "compile"
	RUN_STAGE     string = "run"
)

type FunctionRuntimeInfo struct {
	Name        string                   `json:"name"`
	Version     []FunctionRuntimeVersion `json:"version"`
	DepFileName string                   `json:"depFileName,omitempty"`
	FileSuffix  string                   `json:"fileSuffix"`
}

func NewFunctionRuntimeConfig(globalConfig *v1.ConfigMap) *FunctionRuntimeConfig {
	var runtimeList []FunctionRuntimeInfo
	return &FunctionRuntimeConfig{
		globalConfig: globalConfig,
		RuntimeList:  runtimeList,
	}
}
func (frc *FunctionRuntimeConfig) ReadRuntimeConfig() {
	if runtimeList, ok := frc.globalConfig.Data["runtime_list"]; ok {
		err := json.Unmarshal([]byte(runtimeList), &frc.RuntimeList)
		if err != nil {
			logrus.Fatal("Read Runtime Config Failed", err)
		}
	}
}

func (frc *FunctionRuntimeConfig) GetRuntime(runtime string) (FunctionRuntimeInfo, FunctionRuntimeVersion, error) {

	str := strings.Split(runtime, ":")
	if len(str) != 2 {
		return FunctionRuntimeInfo{}, FunctionRuntimeVersion{}, fmt.Errorf("failed: incorrect runtime format ")
	}
	runtimeName := str[0]
	runtimeVersionName := str[1]
	for _, _runtimeInfo := range frc.RuntimeList {
		if _runtimeInfo.Name == runtimeName {
			for _, _runtimeVersion := range _runtimeInfo.Version {
				if _runtimeVersion.Version == runtimeVersionName {
					return _runtimeInfo, _runtimeVersion, nil
				}
			}
		}
	}
	return FunctionRuntimeInfo{}, FunctionRuntimeVersion{}, fmt.Errorf("Falied: runtime: %s not found ", runtime)
}

func (frv *FunctionRuntimeVersion) GetImage(stage string) (*Image, error) {
	for _, image := range frv.Images {
		if image.Stage == stage {
			return &image, nil
		}
	}
	return nil, fmt.Errorf("Failed: %s Image Not Found ", stage)
}
