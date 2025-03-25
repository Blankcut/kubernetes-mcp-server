package k8s

import (
	"bytes"
    "io"
	"context"
	"fmt"
	"strings"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/models"
	
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// resourceMappings maps common resource types to their API versions and kinds
var resourceMappings = map[string]schema.GroupVersionResource{
	"pod":         {Group: "", Version: "v1", Resource: "pods"},
	"deployment":  {Group: "apps", Version: "v1", Resource: "deployments"},
	"service":     {Group: "", Version: "v1", Resource: "services"},
	"configmap":   {Group: "", Version: "v1", Resource: "configmaps"},
	"secret":      {Group: "", Version: "v1", Resource: "secrets"},
	"statefulset": {Group: "apps", Version: "v1", Resource: "statefulsets"},
	"daemonset":   {Group: "apps", Version: "v1", Resource: "daemonsets"},
	"job":         {Group: "batch", Version: "v1", Resource: "jobs"},
	"cronjob":     {Group: "batch", Version: "v1", Resource: "cronjobs"},
	"ingress":     {Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
	"namespace":   {Group: "", Version: "v1", Resource: "namespaces"},
	"node":        {Group: "", Version: "v1", Resource: "nodes"},
	"pv":          {Group: "", Version: "v1", Resource: "persistentvolumes"},
	"pvc":         {Group: "", Version: "v1", Resource: "persistentvolumeclaims"},
}

// getGVR returns the GroupVersionResource for a given resource type
func (c *Client) getGVR(resourceType string) (schema.GroupVersionResource, error) {
	// Check if it's in our pre-defined mappings
	resourceType = strings.ToLower(resourceType)
	if gvr, ok := resourceMappings[resourceType]; ok {
		return gvr, nil
	}

	// Try to get it from the API discovery
	c.logger.Debug("Resource not in predefined mappings, discovering from API", "resourceType", resourceType)
	resources, err := c.discoveryClient.ServerPreferredResources()
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("failed to get server resources: %w", err)
	}

	for _, list := range resources {
		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil {
			continue
		}

		for _, r := range list.APIResources {
			if strings.EqualFold(r.Name, resourceType) || strings.EqualFold(r.SingularName, resourceType) {
				c.logger.Debug("Found resource via API discovery", 
					"resourceType", resourceType, 
					"group", gv.Group, 
					"version", gv.Version, 
					"resource", r.Name)
				return schema.GroupVersionResource{
					Group:    gv.Group,
					Version:  gv.Version,
					Resource: r.Name,
				}, nil
			}
		}
	}

	return schema.GroupVersionResource{}, fmt.Errorf("unknown resource type: %s", resourceType)
}

// GetResource retrieves a specific resource by kind, namespace, and name
func (c *Client) GetResource(ctx context.Context, kind, namespace, name string) (*unstructured.Unstructured, error) {
	c.logger.Debug("Getting resource", "kind", kind, "namespace", namespace, "name", name)
	
	gvr, err := c.getGVR(kind)
	if err != nil {
		return nil, err
	}

	var obj *unstructured.Unstructured
	if namespace != "" {
		obj, err = c.dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	} else {
		obj, err = c.dynamicClient.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get %s %s/%s: %w", kind, namespace, name, err)
	}

	return obj, nil
}

// ListResources lists resources of a specific type, optionally filtered by namespace
func (c *Client) ListResources(ctx context.Context, kind, namespace string) ([]unstructured.Unstructured, error) {
	c.logger.Debug("Listing resources", "kind", kind, "namespace", namespace)
	
	gvr, err := c.getGVR(kind)
	if err != nil {
		return nil, err
	}

	var list *unstructured.UnstructuredList
	if namespace != "" {
		list, err = c.dynamicClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	} else {
		list, err = c.dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list resources: %w", err)
	}

	c.logger.Debug("Listed resources", "kind", kind, "count", len(list.Items))
	return list.Items, nil
}

// GetPodStatus returns detailed status information for a pod
func (c *Client) GetPodStatus(ctx context.Context, namespace, name string) (*models.K8sPodStatus, error) {
	c.logger.Debug("Getting pod status", "namespace", namespace, "name", name)
	
	pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod: %w", err)
	}

	status := &models.K8sPodStatus{
		Phase: string(pod.Status.Phase),
	}

	for _, condition := range pod.Status.Conditions {
		status.Conditions = append(status.Conditions, struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		}{
			Type:   string(condition.Type),
			Status: string(condition.Status),
		})
	}

	// Copy container statuses
	for _, containerStatus := range pod.Status.ContainerStatuses {
		cs := struct {
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
		}{
			Name:         containerStatus.Name,
			Ready:        containerStatus.Ready,
			RestartCount: int(containerStatus.RestartCount),
		}

		// Set state
		if containerStatus.State.Running != nil {
			cs.State.Running = &struct{}{}
		}
		if containerStatus.State.Waiting != nil {
			cs.State.Waiting = &struct{}{}
		}
		if containerStatus.State.Terminated != nil {
			cs.State.Terminated = &struct{}{}
		}

		// Set last state
		if containerStatus.LastTerminationState.Running != nil {
			cs.LastState.Running = &struct{}{}
		}
		if containerStatus.LastTerminationState.Waiting != nil {
			cs.LastState.Waiting = &struct{}{}
		}
		if containerStatus.LastTerminationState.Terminated != nil {
			cs.LastState.Terminated = &struct{}{}
		}

		status.ContainerStatuses = append(status.ContainerStatuses, cs)
	}

	return status, nil
}

// GetPodLogs returns logs for a specific container in a pod
func (c *Client) GetPodLogs(ctx context.Context, namespace, name, container string, tailLines int64) (string, error) {
	c.logger.Debug("Getting pod logs", 
		"namespace", namespace, 
		"name", name, 
		"container", container, 
		"tailLines", tailLines)
	
	podLogOptions := corev1.PodLogOptions{
		Container: container,
	}
	
	if tailLines > 0 {
		podLogOptions.TailLines = &tailLines
	}
	
	req := c.clientset.CoreV1().Pods(namespace).GetLogs(name, &podLogOptions)
	podLogs, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get pod logs: %w", err)
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return "", fmt.Errorf("failed to read pod logs: %w", err)
	}

	return buf.String(), nil
}

// FindOwnerReferences finds the owner references for a resource
func (c *Client) FindOwnerReferences(ctx context.Context, obj *unstructured.Unstructured) ([]unstructured.Unstructured, error) {
	c.logger.Debug("Finding owner references", 
		"kind", obj.GetKind(), 
		"name", obj.GetName(), 
		"namespace", obj.GetNamespace())
	
	ownerRefs := obj.GetOwnerReferences()
	if len(ownerRefs) == 0 {
		return nil, nil
	}

	var owners []unstructured.Unstructured
	for _, ref := range ownerRefs {
		c.logger.Debug("Found owner reference", 
			"kind", ref.Kind, 
			"name", ref.Name, 
			"namespace", obj.GetNamespace())
		
		gvr, err := c.getGVR(ref.Kind)
		if err != nil {
			c.logger.Warn("Failed to get GroupVersionResource for owner", 
				"kind", ref.Kind, 
				"error", err)
			continue
		}

		namespace := obj.GetNamespace()
		owner, err := c.dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, ref.Name, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				c.logger.Warn("Owner not found", 
					"kind", ref.Kind, 
					"name", ref.Name, 
					"namespace", namespace)
				continue
			}
			return nil, fmt.Errorf("failed to get owner reference: %w", err)
		}

		owners = append(owners, *owner)
	}

	return owners, nil
}