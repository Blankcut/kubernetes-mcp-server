package k8s

import (
	"context"
	"fmt"
	"strings"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// NamespaceResourcesCollection contains all resources in a namespace
type NamespaceResourcesCollection struct {
	Namespace string                                 `json:"namespace"`
	Resources map[string][]unstructured.Unstructured `json:"resources"`
	Stats     map[string]int                         `json:"stats"`
}

// ResourceDetails contains detailed information about a resource
type ResourceDetails struct {
	Resource      *unstructured.Unstructured `json:"resource"`
	Events        []interface{}              `json:"events"`
	Relationships []ResourceRelationship     `json:"relationships"`
	Metrics       map[string]interface{}     `json:"metrics"`
}

// GetAllNamespaceResources retrieves all resources in a namespace
func (c *Client) GetAllNamespaceResources(ctx context.Context, namespace string) (*NamespaceResourcesCollection, error) {
	c.logger.Info("Getting all resources in namespace", "namespace", namespace)

	collection := &NamespaceResourcesCollection{
		Namespace: namespace,
		Resources: make(map[string][]unstructured.Unstructured),
		Stats:     make(map[string]int),
	}

	// Discover all available resource types
	resources, err := c.discoveryClient.ServerPreferredResources()
	if err != nil {
		return nil, fmt.Errorf("failed to get server resources: %w", err)
	}

	// Use a wait group to parallelize resource collection
	var wg sync.WaitGroup
	var mu sync.Mutex // Mutex for safely updating the collection

	// Collect resources for each API group concurrently
	for _, resourceList := range resources {
		wg.Add(1)

		go func(resourceList *metav1.APIResourceList) {
			defer wg.Done()

			gv, err := schema.ParseGroupVersion(resourceList.GroupVersion)
			if err != nil {
				c.logger.Warn("Failed to parse group version", "groupVersion", resourceList.GroupVersion)
				return
			}

			for _, r := range resourceList.APIResources {
				// Skip resources that can't be listed or aren't namespaced
				if !strings.Contains(r.Verbs.String(), "list") || !r.Namespaced {
					continue
				}

				// Skip subresources (contains slash)
				if strings.Contains(r.Name, "/") {
					continue
				}

				// Build GVR for this resource type
				gvr := schema.GroupVersionResource{
					Group:    gv.Group,
					Version:  gv.Version,
					Resource: r.Name,
				}

				// List resources of this type
				list, err := c.dynamicClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
				if err != nil {
					c.logger.Warn("Failed to list resources",
						"namespace", namespace,
						"resource", r.Name,
						"error", err)
					continue
				}

				// Skip if no resources found
				if len(list.Items) == 0 {
					continue
				}

				// Add to collection with thread safety
				mu.Lock()
				collection.Resources[r.Kind] = list.Items
				collection.Stats[r.Kind] = len(list.Items)
				mu.Unlock()
			}
		}(resourceList)
	}

	// Wait for all resource collections to complete
	wg.Wait()

	c.logger.Info("Collected all namespace resources",
		"namespace", namespace,
		"resourceTypes", len(collection.Resources),
		"totalResources", c.countTotalResources(collection.Stats))

	return collection, nil
}

// countTotalResources counts the total number of resources across all types
func (c *Client) countTotalResources(stats map[string]int) int {
	total := 0
	for _, count := range stats {
		total += count
	}
	return total
}

// GetResourceDetails gets detailed information about a specific resource
func (c *Client) GetResourceDetails(ctx context.Context, kind, namespace, name string) (*ResourceDetails, error) {
	c.logger.Info("Getting resource details", "kind", kind, "namespace", namespace, "name", name)

	// Get the resource
	resource, err := c.GetResource(ctx, kind, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource: %w", err)
	}

	// Initialize resource details
	details := &ResourceDetails{
		Resource: resource,
		Metrics:  make(map[string]interface{}),
	}

	// Get resource events
	events, err := c.GetResourceEvents(ctx, namespace, kind, name)
	if err != nil {
		c.logger.Warn("Failed to get resource events", "error", err)
	} else {
		// Convert events to interface for JSON serialization
		eventsInterface := make([]interface{}, len(events))
		for i, event := range events {
			eventMap := map[string]interface{}{
				"reason":    event.Reason,
				"message":   event.Message,
				"type":      event.Type,
				"count":     event.Count,
				"firstTime": event.FirstTime,
				"lastTime":  event.LastTime,
				"object": map[string]string{
					"kind":      event.Object.Kind,
					"name":      event.Object.Name,
					"namespace": event.Object.Namespace,
				},
			}
			eventsInterface[i] = eventMap
		}
		details.Events = eventsInterface
	}

	// Add resource-specific metrics
	c.addResourceMetrics(ctx, resource, details)

	return details, nil
}

// addResourceMetrics adds resource-specific metrics based on resource type
func (c *Client) addResourceMetrics(ctx context.Context, resource *unstructured.Unstructured, details *ResourceDetails) {
	kind := resource.GetKind()

	switch kind {
	case "Pod":
		// Add container statuses
		containers, found, _ := unstructured.NestedSlice(resource.Object, "spec", "containers")
		if found {
			details.Metrics["containerCount"] = len(containers)
		}

		// Add status phase
		phase, found, _ := unstructured.NestedString(resource.Object, "status", "phase")
		if found {
			details.Metrics["phase"] = phase
		}

		// Add restart counts
		containerStatuses, found, _ := unstructured.NestedSlice(resource.Object, "status", "containerStatuses")
		if found {
			totalRestarts := 0
			for _, cs := range containerStatuses {
				containerStatus, ok := cs.(map[string]interface{})
				if !ok {
					continue
				}

				restarts, found, _ := unstructured.NestedInt64(containerStatus, "restartCount")
				if found {
					totalRestarts += int(restarts)
				}
			}
			details.Metrics["totalRestarts"] = totalRestarts
		}

	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet":
		// Add replica counts
		replicas, found, _ := unstructured.NestedInt64(resource.Object, "spec", "replicas")
		if found {
			details.Metrics["desiredReplicas"] = replicas
		}

		availableReplicas, found, _ := unstructured.NestedInt64(resource.Object, "status", "availableReplicas")
		if found {
			details.Metrics["availableReplicas"] = availableReplicas
		}

		readyReplicas, found, _ := unstructured.NestedInt64(resource.Object, "status", "readyReplicas")
		if found {
			details.Metrics["readyReplicas"] = readyReplicas
		}

		if kind == "Deployment" {
			// Add deployment strategy
			strategy, found, _ := unstructured.NestedString(resource.Object, "spec", "strategy", "type")
			if found {
				details.Metrics["strategy"] = strategy
			}
		}

	case "Service":
		// Add service type
		serviceType, found, _ := unstructured.NestedString(resource.Object, "spec", "type")
		if found {
			details.Metrics["type"] = serviceType
		}

		// Add port count
		ports, found, _ := unstructured.NestedSlice(resource.Object, "spec", "ports")
		if found {
			details.Metrics["portCount"] = len(ports)
		}

	case "PersistentVolumeClaim":
		// Add storage capacity
		capacity, found, _ := unstructured.NestedString(resource.Object, "spec", "resources", "requests", "storage")
		if found {
			details.Metrics["requestedStorage"] = capacity
		}

		// Add access modes
		accessModes, found, _ := unstructured.NestedStringSlice(resource.Object, "spec", "accessModes")
		if found {
			details.Metrics["accessModes"] = accessModes
		}

		// Add phase
		phase, found, _ := unstructured.NestedString(resource.Object, "status", "phase")
		if found {
			details.Metrics["phase"] = phase
		}
	}
}
