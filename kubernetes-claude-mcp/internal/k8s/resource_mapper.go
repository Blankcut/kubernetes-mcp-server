package k8s

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
)

// ResourceMapper maps relationships between Kubernetes resources
type ResourceMapper struct {
	client *Client
	logger *logging.Logger
}

// ResourceRelationship represents a relationship between two resources
type ResourceRelationship struct {
	SourceKind      string `json:"sourceKind"`
	SourceName      string `json:"sourceName"`
	SourceNamespace string `json:"sourceNamespace"`
	TargetKind      string `json:"targetKind"`
	TargetName      string `json:"targetName"`
	TargetNamespace string `json:"targetNamespace"`
	RelationType    string `json:"relationType"`
}

// NamespaceTopology represents the topology of resources in a namespace
type NamespaceTopology struct {
	Namespace     string                       `json:"namespace"`
	Resources     map[string][]string          `json:"resources"`
	Relationships []ResourceRelationship       `json:"relationships"`
	Metrics       map[string]map[string]int    `json:"metrics"`
	Health        map[string]map[string]string `json:"health"`
}

// NewResourceMapper creates a new resource mapper
func NewResourceMapper(client *Client) *ResourceMapper {
	return &ResourceMapper{
		client: client,
		logger: client.logger.Named("resource-mapper"),
	}
}

// GetNamespaceTopology maps all resources and their relationships in a namespace
func (m *ResourceMapper) GetNamespaceTopology(ctx context.Context, namespace string) (*NamespaceTopology, error) {
	m.logger.Info("Mapping namespace topology", "namespace", namespace)

	// Initialize topology
	topology := &NamespaceTopology{
		Namespace:     namespace,
		Resources:     make(map[string][]string),
		Relationships: []ResourceRelationship{},
		Metrics:       make(map[string]map[string]int),
		Health:        make(map[string]map[string]string),
	}

	// Discover all available resource types
	resources, err := m.client.discoveryClient.ServerPreferredResources()
	if err != nil {
		return nil, fmt.Errorf("failed to get server resources: %w", err)
	}

	// Collect all namespaced resources
	for _, resourceList := range resources {
		gv, err := schema.ParseGroupVersion(resourceList.GroupVersion)
		if err != nil {
			m.logger.Warn("Failed to parse group version", "groupVersion", resourceList.GroupVersion)
			continue
		}

		for _, r := range resourceList.APIResources {
			// Skip resources that can't be listed or aren't namespaced
			if !strings.Contains(r.Verbs.String(), "list") || !r.Namespaced {
				continue
			}

			// Build GVR for this resource type
			gvr := schema.GroupVersionResource{
				Group:    gv.Group,
				Version:  gv.Version,
				Resource: r.Name,
			}

			// List resources of this type
			m.logger.Debug("Listing resources", "namespace", namespace, "resource", r.Name)
			list, err := m.client.dynamicClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				m.logger.Warn("Failed to list resources",
					"namespace", namespace,
					"resource", r.Name,
					"error", err)
				continue
			}

			// Add to topology
			if len(list.Items) > 0 {
				topology.Resources[r.Kind] = make([]string, len(list.Items))
				topology.Metrics[r.Kind] = map[string]int{"count": len(list.Items)}
				topology.Health[r.Kind] = make(map[string]string)

				for i, item := range list.Items {
					topology.Resources[r.Kind][i] = item.GetName()

					// Determine health status
					health := m.determineResourceHealth(&item)
					topology.Health[r.Kind][item.GetName()] = health
				}

				// Find relationships for this resource type
				relationships := m.findRelationships(ctx, list.Items, namespace)
				topology.Relationships = append(topology.Relationships, relationships...)
			}
		}
	}

	m.logger.Info("Namespace topology mapped",
		"namespace", namespace,
		"resourceTypes", len(topology.Resources),
		"relationships", len(topology.Relationships))

	return topology, nil
}

// GetResourceGraph returns a resource graph for visualization
func (m *ResourceMapper) GetResourceGraph(ctx context.Context, namespace string) (map[string]interface{}, error) {
	topology, err := m.GetNamespaceTopology(ctx, namespace)
	if err != nil {
		return nil, err
	}

	// Convert topology to graph format
	graph := map[string]interface{}{
		"nodes": []map[string]interface{}{},
		"edges": []map[string]interface{}{},
	}

	// Add nodes
	nodeIndex := make(map[string]int)
	nodeCount := 0

	for kind, resources := range topology.Resources {
		for _, name := range resources {
			health := "unknown"
			if h, ok := topology.Health[kind][name]; ok {
				health = h
			}

			node := map[string]interface{}{
				"id":     nodeCount,
				"kind":   kind,
				"name":   name,
				"health": health,
				"group":  kind,
			}

			// Add to nodes array
			graph["nodes"] = append(graph["nodes"].([]map[string]interface{}), node)

			// Save index for edge creation
			nodeIndex[fmt.Sprintf("%s/%s", kind, name)] = nodeCount
			nodeCount++
		}
	}

	// Add edges
	for _, rel := range topology.Relationships {
		sourceKey := fmt.Sprintf("%s/%s", rel.SourceKind, rel.SourceName)
		targetKey := fmt.Sprintf("%s/%s", rel.TargetKind, rel.TargetName)

		sourceIdx, sourceOk := nodeIndex[sourceKey]
		targetIdx, targetOk := nodeIndex[targetKey]

		if sourceOk && targetOk {
			edge := map[string]interface{}{
				"source":       sourceIdx,
				"target":       targetIdx,
				"relationship": rel.RelationType,
			}

			graph["edges"] = append(graph["edges"].([]map[string]interface{}), edge)
		}
	}

	return graph, nil
}

// findRelationships discovers relationships between resources
func (m *ResourceMapper) findRelationships(ctx context.Context, resources []unstructured.Unstructured, namespace string) []ResourceRelationship {
	var relationships []ResourceRelationship

	for _, resource := range resources {
		// Check owner references
		for _, ownerRef := range resource.GetOwnerReferences() {
			rel := ResourceRelationship{
				SourceKind:      ownerRef.Kind,
				SourceName:      ownerRef.Name,
				SourceNamespace: namespace,
				TargetKind:      resource.GetKind(),
				TargetName:      resource.GetName(),
				TargetNamespace: namespace,
				RelationType:    "owns",
			}
			relationships = append(relationships, rel)
		}

		// Check for Pod -> Service relationships (via labels/selectors)
		if resource.GetKind() == "Service" {
			selector, found, _ := unstructured.NestedMap(resource.Object, "spec", "selector")
			if found && len(selector) > 0 {
				// Find pods matching this selector
				pods, err := m.client.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
					LabelSelector: m.labelsToSelector(selector),
				})

				if err == nil {
					for _, pod := range pods.Items {
						rel := ResourceRelationship{
							SourceKind:      "Service",
							SourceName:      resource.GetName(),
							SourceNamespace: namespace,
							TargetKind:      "Pod",
							TargetName:      pod.Name,
							TargetNamespace: namespace,
							RelationType:    "selects",
						}
						relationships = append(relationships, rel)
					}
				}
			}
		}

		// Check for ConfigMap/Secret references in Pods
		if resource.GetKind() == "Pod" {
			// Check volumes for ConfigMap references
			volumes, found, _ := unstructured.NestedSlice(resource.Object, "spec", "volumes")
			if found {
				for _, v := range volumes {
					volume, ok := v.(map[string]interface{})
					if !ok {
						continue
					}

					// Check for ConfigMap references
					if configMap, hasConfigMap, _ := unstructured.NestedMap(volume, "configMap"); hasConfigMap {
						if cmName, hasName, _ := unstructured.NestedString(configMap, "name"); hasName {
							rel := ResourceRelationship{
								SourceKind:      "Pod",
								SourceName:      resource.GetName(),
								SourceNamespace: namespace,
								TargetKind:      "ConfigMap",
								TargetName:      cmName,
								TargetNamespace: namespace,
								RelationType:    "mounts",
							}
							relationships = append(relationships, rel)
						}
					}

					// Check for Secret references
					if secret, hasSecret, _ := unstructured.NestedMap(volume, "secret"); hasSecret {
						if secretName, hasName, _ := unstructured.NestedString(secret, "secretName"); hasName {
							rel := ResourceRelationship{
								SourceKind:      "Pod",
								SourceName:      resource.GetName(),
								SourceNamespace: namespace,
								TargetKind:      "Secret",
								TargetName:      secretName,
								TargetNamespace: namespace,
								RelationType:    "mounts",
							}
							relationships = append(relationships, rel)
						}
					}
				}
			}

			// Check environment variables for ConfigMap/Secret references
			containers, found, _ := unstructured.NestedSlice(resource.Object, "spec", "containers")
			if found {
				for _, c := range containers {
					container, ok := c.(map[string]interface{})
					if !ok {
						continue
					}

					// Check for EnvFrom references
					envFrom, hasEnvFrom, _ := unstructured.NestedSlice(container, "envFrom")
					if hasEnvFrom {
						for _, ef := range envFrom {
							envFromObj, ok := ef.(map[string]interface{})
							if !ok {
								continue
							}

							// Check for ConfigMap references
							if configMap, hasConfigMap, _ := unstructured.NestedMap(envFromObj, "configMapRef"); hasConfigMap {
								if cmName, hasName, _ := unstructured.NestedString(configMap, "name"); hasName {
									rel := ResourceRelationship{
										SourceKind:      "Pod",
										SourceName:      resource.GetName(),
										SourceNamespace: namespace,
										TargetKind:      "ConfigMap",
										TargetName:      cmName,
										TargetNamespace: namespace,
										RelationType:    "configures",
									}
									relationships = append(relationships, rel)
								}
							}

							// Check for Secret references
							if secret, hasSecret, _ := unstructured.NestedMap(envFromObj, "secretRef"); hasSecret {
								if secretName, hasName, _ := unstructured.NestedString(secret, "name"); hasName {
									rel := ResourceRelationship{
										SourceKind:      "Pod",
										SourceName:      resource.GetName(),
										SourceNamespace: namespace,
										TargetKind:      "Secret",
										TargetName:      secretName,
										TargetNamespace: namespace,
										RelationType:    "configures",
									}
									relationships = append(relationships, rel)
								}
							}
						}
					}

					// Check individual env vars for ConfigMap/Secret references
					env, hasEnv, _ := unstructured.NestedSlice(container, "env")
					if hasEnv {
						for _, e := range env {
							envVar, ok := e.(map[string]interface{})
							if !ok {
								continue
							}

							// Check for ConfigMap references
							if valueFrom, hasValueFrom, _ := unstructured.NestedMap(envVar, "valueFrom"); hasValueFrom {
								if configMap, hasConfigMap, _ := unstructured.NestedMap(valueFrom, "configMapKeyRef"); hasConfigMap {
									if cmName, hasName, _ := unstructured.NestedString(configMap, "name"); hasName {
										rel := ResourceRelationship{
											SourceKind:      "Pod",
											SourceName:      resource.GetName(),
											SourceNamespace: namespace,
											TargetKind:      "ConfigMap",
											TargetName:      cmName,
											TargetNamespace: namespace,
											RelationType:    "configures",
										}
										relationships = append(relationships, rel)
									}
								}

								// Check for Secret references
								if secret, hasSecret, _ := unstructured.NestedMap(valueFrom, "secretKeyRef"); hasSecret {
									if secretName, hasName, _ := unstructured.NestedString(secret, "name"); hasName {
										rel := ResourceRelationship{
											SourceKind:      "Pod",
											SourceName:      resource.GetName(),
											SourceNamespace: namespace,
											TargetKind:      "Secret",
											TargetName:      secretName,
											TargetNamespace: namespace,
											RelationType:    "configures",
										}
										relationships = append(relationships, rel)
									}
								}
							}
						}
					}
				}
			}
		}

		// Check for PVC -> PV relationships
		if resource.GetKind() == "PersistentVolumeClaim" {
			volumeName, found, _ := unstructured.NestedString(resource.Object, "spec", "volumeName")
			if found && volumeName != "" {
				rel := ResourceRelationship{
					SourceKind:      "PersistentVolumeClaim",
					SourceName:      resource.GetName(),
					SourceNamespace: namespace,
					TargetKind:      "PersistentVolume",
					TargetName:      volumeName,
					TargetNamespace: "",
					RelationType:    "binds",
				}
				relationships = append(relationships, rel)
			}
		}

		// Check for Ingress -> Service relationships
		if resource.GetKind() == "Ingress" {
			rules, found, _ := unstructured.NestedSlice(resource.Object, "spec", "rules")
			if found {
				for _, r := range rules {
					rule, ok := r.(map[string]interface{})
					if !ok {
						continue
					}

					http, found, _ := unstructured.NestedMap(rule, "http")
					if !found {
						continue
					}

					paths, found, _ := unstructured.NestedSlice(http, "paths")
					if !found {
						continue
					}

					for _, p := range paths {
						path, ok := p.(map[string]interface{})
						if !ok {
							continue
						}

						backend, found, _ := unstructured.NestedMap(path, "backend")
						if !found {
							// Check for newer API version format
							backend, found, _ = unstructured.NestedMap(path, "backend", "service")
							if !found {
								continue
							}
						}

						serviceName, found, _ := unstructured.NestedString(backend, "name")
						if found {
							rel := ResourceRelationship{
								SourceKind:      "Ingress",
								SourceName:      resource.GetName(),
								SourceNamespace: namespace,
								TargetKind:      "Service",
								TargetName:      serviceName,
								TargetNamespace: namespace,
								RelationType:    "routes",
							}
							relationships = append(relationships, rel)
						}
					}
				}
			}
		}
	}

	// Deduplicate relationships
	deduplicatedRelationships := make([]ResourceRelationship, 0)
	relMap := make(map[string]bool)

	for _, rel := range relationships {
		key := fmt.Sprintf("%s/%s/%s/%s/%s/%s/%s",
			rel.SourceKind, rel.SourceName, rel.SourceNamespace,
			rel.TargetKind, rel.TargetName, rel.TargetNamespace,
			rel.RelationType)

		if _, exists := relMap[key]; !exists {
			relMap[key] = true
			deduplicatedRelationships = append(deduplicatedRelationships, rel)
		}
	}

	return deduplicatedRelationships
}

// labelsToSelector converts a map of labels to a selector string
func (m *ResourceMapper) labelsToSelector(labels map[string]interface{}) string {
	var selectors []string

	for key, value := range labels {
		if strValue, ok := value.(string); ok {
			selectors = append(selectors, fmt.Sprintf("%s=%s", key, strValue))
		}
	}

	return strings.Join(selectors, ",")
}

// determineResourceHealth determines the health status of a resource
func (m *ResourceMapper) determineResourceHealth(obj *unstructured.Unstructured) string {
	kind := obj.GetKind()

	// Check common status fields
	status, found, _ := unstructured.NestedMap(obj.Object, "status")
	if !found {
		return "unknown"
	}

	// Check different resource types
	switch kind {
	case "Pod":
		phase, found, _ := unstructured.NestedString(status, "phase")
		if found {
			switch phase {
			case "Running", "Succeeded":
				return "healthy"
			case "Pending":
				return "progressing"
			case "Failed":
				return "unhealthy"
			default:
				return "unknown"
			}
		}

	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet":
		// Check if all replicas are available
		replicas, foundReplicas, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas")
		if !foundReplicas {
			replicas = 1 // Default to 1 if not specified
		}

		availableReplicas, foundAvailable, _ := unstructured.NestedInt64(status, "availableReplicas")
		if foundAvailable && availableReplicas == replicas {
			return "healthy"
		} else if foundAvailable && availableReplicas > 0 {
			return "progressing"
		} else {
			return "unhealthy"
		}

	case "Service":
		// Services are typically healthy unless they have no endpoints
		// We'd need to check endpoints separately
		return "healthy"

	case "Ingress":
		// Check if LoadBalancer has assigned addresses
		ingress, found, _ := unstructured.NestedSlice(status, "loadBalancer", "ingress")
		if found && len(ingress) > 0 {
			return "healthy"
		}
		return "progressing"

	case "PersistentVolumeClaim":
		phase, found, _ := unstructured.NestedString(status, "phase")
		if found && phase == "Bound" {
			return "healthy"
		} else if found && phase == "Pending" {
			return "progressing"
		} else {
			return "unhealthy"
		}

	case "Job":
		conditions, found, _ := unstructured.NestedSlice(status, "conditions")
		if found {
			for _, c := range conditions {
				condition, ok := c.(map[string]interface{})
				if !ok {
					continue
				}

				condType, typeFound, _ := unstructured.NestedString(condition, "type")
				condStatus, statusFound, _ := unstructured.NestedString(condition, "status")

				if typeFound && statusFound && condType == "Complete" && condStatus == "True" {
					return "healthy"
				} else if typeFound && statusFound && condType == "Failed" && condStatus == "True" {
					return "unhealthy"
				}
			}
			return "progressing"
		}

	default:
		// For other resources, try to check common status conditions
		conditions, found, _ := unstructured.NestedSlice(status, "conditions")
		if found {
			for _, c := range conditions {
				condition, ok := c.(map[string]interface{})
				if !ok {
					continue
				}

				condType, typeFound, _ := unstructured.NestedString(condition, "type")
				condStatus, statusFound, _ := unstructured.NestedString(condition, "status")

				if typeFound && statusFound {
					// Check for common condition types indicating health
					if (condType == "Ready" || condType == "Available") && condStatus == "True" {
						return "healthy"
					} else if condType == "Progressing" && condStatus == "True" {
						return "progressing"
					} else if (condType == "Failed" || condType == "Error") && condStatus == "True" {
						return "unhealthy"
					}
				}
			}
		}
	}

	return "unknown"
}
