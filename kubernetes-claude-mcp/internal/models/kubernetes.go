package models

import (
	"time"
	
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// K8sResourceMeta contains common metadata for Kubernetes resources
type K8sResourceMeta struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	UID               string            `json:"uid"`
	ResourceVersion   string            `json:"resourceVersion"`
	CreationTimestamp time.Time         `json:"creationTimestamp"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
}

// K8sResourceStatus provides a common interface for resource status
type K8sResourceStatus struct {
	Phase      string `json:"phase,omitempty"`
	Message    string `json:"message,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Conditions []struct {
		Type               string    `json:"type"`
		Status             string    `json:"status"`
		LastTransitionTime time.Time `json:"lastTransitionTime"`
		Reason             string    `json:"reason,omitempty"`
		Message            string    `json:"message,omitempty"`
	} `json:"conditions,omitempty"`
}

// K8sEvent represents a Kubernetes event
type K8sEvent struct {
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	Type      string    `json:"type"`
	Count     int       `json:"count"`
	FirstTime time.Time `json:"firstTime"`
	LastTime  time.Time `json:"lastTime"`
	Object    struct {
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"object"`
}

// K8sPodStatus represents the status of a pod
type K8sPodStatus struct {
	Phase      string `json:"phase"`
	Conditions []struct {
		Type   string `json:"type"`
		Status string `json:"status"`
	} `json:"conditions"`
	ContainerStatuses []struct {
		Name         string `json:"name"`
		Ready        bool   `json:"ready"`
		RestartCount int    `json:"restartCount"`
		State        struct {
			Running    *struct{} `json:"running,omitempty"`
			Waiting    *struct{} `json:"waiting,omitempty"`
			Terminated *struct{} `json:"terminated,omitempty"`
		} `json:"state"`
		LastState struct {
			Running    *struct{} `json:"running,omitempty"`
			Waiting    *struct{} `json:"waiting,omitempty"`
			Terminated *struct{} `json:"terminated,omitempty"`
		} `json:"lastState"`
	} `json:"containerStatuses"`
}

// ExtractResourceMeta extracts common metadata from an unstructured resource
func ExtractResourceMeta(obj unstructured.Unstructured) K8sResourceMeta {
	meta := K8sResourceMeta{
		Name:              obj.GetName(),
		Namespace:         obj.GetNamespace(),
		UID:               string(obj.GetUID()),
		ResourceVersion:   obj.GetResourceVersion(),
		CreationTimestamp: obj.GetCreationTimestamp().Time,
		Labels:            obj.GetLabels(),
		Annotations:       obj.GetAnnotations(),
	}
	return meta
}