package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// FunctionSpec defines the desired state of Function
type FunctionSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	Function         string            `json:"function"`
	Identifier       string            `json:"identifier"`
	Version          string            `json:"version"`
	Runtime          string            `json:"runtime"`
	Deps             string            `json:"deps,omitempty"`
	Handler          string            `json:"handler"`
	ContentType      string            `json:"contentType"`
	Timeout          string            `json:"timeout"`
	Size             *int32            `json:"size"`
	ExposedPort      int32             `json:"exposedPorts,omitempty"`
	ExternalService  map[string]string `json:"externalService,omitempty"`
	DataSource       string            `json:"dataSource,omitempty"`
	DataServiceToken string            `json:"dataServiceToken,omitempty"`
	CPU string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
	HPA bool `json:"hpa,omitempty"`
	HPAPrediction bool `json:"HPAPrediction,omitempty"`
	CPUTargetAverageUtilization *int32 `json:"cpuTargetAverageUtilization,omitempty"`
}

// FunctionStatus defines the observed state of Function
type FunctionStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	PodsStatus []PodsStatus `json:"podStatus"`
	Size int32 `json:"size"`
	Selector string `json:"selector"`
}

type PodsStatus struct {
	PodName               string                   `json:"podName"`
	PodPhase              corev1.PodPhase          `json:"podPhase"`
	InitContainerStatuses []corev1.ContainerStatus `json:"initContainerStatuses"`
	ContainerStatuses     []corev1.ContainerStatus `json:"containerStatuses"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Function is the Schema for the functions API
// +kubebuilder:subresource:status
//+kubebuilder:subresource:scale:specpath=.spec.size,statuspath=.status.size,selectorpath=.status.selector
// +kubebuilder:resource:path=functions,scope=Namespaced
type Function struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FunctionSpec   `json:"spec,omitempty"`
	Status FunctionStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FunctionList contains a list of Function
type FunctionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Function `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Function{}, &FunctionList{})
}
