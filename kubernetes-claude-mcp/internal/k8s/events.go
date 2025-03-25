package k8s

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/models"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

// GetResourceEvents returns events related to a specific resource
func (c *Client) GetResourceEvents(ctx context.Context, namespace, kind, name string) ([]models.K8sEvent, error) {
	c.logger.Debug("Getting events for resource", "namespace", namespace, "kind", kind, "name", name)

	// Build field selector
	var fieldSelector fields.Selector
	if namespace != "" {
		// For namespaced resources
		fieldSelector = fields.AndSelectors(
			fields.OneTermEqualSelector("involvedObject.name", name),
			fields.OneTermEqualSelector("involvedObject.kind", kind),
			fields.OneTermEqualSelector("involvedObject.namespace", namespace),
		)
	} else {
		// For cluster-scoped resources (no namespace)
		fieldSelector = fields.AndSelectors(
			fields.OneTermEqualSelector("involvedObject.name", name),
			fields.OneTermEqualSelector("involvedObject.kind", kind),
		)
	}

	// Get events
	eventList, err := c.clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fieldSelector.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	// Convert to our model
	var events []models.K8sEvent
	for _, event := range eventList.Items {
		e := models.K8sEvent{
			Reason:    event.Reason,
			Message:   event.Message,
			Type:      event.Type,
			Count:     int(event.Count),
			FirstTime: event.FirstTimestamp.Time,
			LastTime:  event.LastTimestamp.Time,
			Object: struct {
				Kind      string `json:"kind"`
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			}{
				Kind:      event.InvolvedObject.Kind,
				Name:      event.InvolvedObject.Name,
				Namespace: event.InvolvedObject.Namespace,
			},
		}
		events = append(events, e)
	}

	// Sort events by last time, most recent first
	sort.Slice(events, func(i, j int) bool {
		return events[i].LastTime.After(events[j].LastTime)
	})

	c.logger.Debug("Got events for resource",
		"namespace", namespace,
		"kind", kind,
		"name", name,
		"count", len(events))
	return events, nil
}

// GetNamespaceEvents returns all events in a namespace
func (c *Client) GetNamespaceEvents(ctx context.Context, namespace string) ([]models.K8sEvent, error) {
	c.logger.Debug("Getting events for namespace", "namespace", namespace)

	// Get events
	eventList, err := c.clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	// Convert to our model
	var events []models.K8sEvent
	for _, event := range eventList.Items {
		e := models.K8sEvent{
			Reason:    event.Reason,
			Message:   event.Message,
			Type:      event.Type,
			Count:     int(event.Count),
			FirstTime: event.FirstTimestamp.Time,
			LastTime:  event.LastTimestamp.Time,
			Object: struct {
				Kind      string `json:"kind"`
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			}{
				Kind:      event.InvolvedObject.Kind,
				Name:      event.InvolvedObject.Name,
				Namespace: event.InvolvedObject.Namespace,
			},
		}
		events = append(events, e)
	}

	// Sort events by last time, most recent first
	sort.Slice(events, func(i, j int) bool {
		return events[i].LastTime.After(events[j].LastTime)
	})

	c.logger.Debug("Got events for namespace", "namespace", namespace, "count", len(events))
	return events, nil
}

// GetRecentWarningEvents returns recent warning events across all namespaces
func (c *Client) GetRecentWarningEvents(ctx context.Context, timeWindow time.Duration) ([]models.K8sEvent, error) {
	c.logger.Debug("Getting recent warning events", "timeWindow", timeWindow)

	// Calculate the cutoff time
	cutoffTime := time.Now().Add(-timeWindow)

	// Get events from all namespaces
	eventList, err := c.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("type", "Warning").String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list warning events: %w", err)
	}

	// Filter and convert to our model
	var events []models.K8sEvent
	for _, event := range eventList.Items {
		// Skip events older than the cutoff time
		if event.LastTimestamp.Time.Before(cutoffTime) {
			continue
		}

		e := models.K8sEvent{
			Reason:    event.Reason,
			Message:   event.Message,
			Type:      event.Type,
			Count:     int(event.Count),
			FirstTime: event.FirstTimestamp.Time,
			LastTime:  event.LastTimestamp.Time,
			Object: struct {
				Kind      string `json:"kind"`
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			}{
				Kind:      event.InvolvedObject.Kind,
				Name:      event.InvolvedObject.Name,
				Namespace: event.InvolvedObject.Namespace,
			},
		}
		events = append(events, e)
	}

	// Sort events by last time, most recent first
	sort.Slice(events, func(i, j int) bool {
		return events[i].LastTime.After(events[j].LastTime)
	})

	c.logger.Debug("Got recent warning events", "count", len(events), "timeWindow", timeWindow)
	return events, nil
}

// GetClusterHealthEvents returns events that might indicate cluster health issues
func (c *Client) GetClusterHealthEvents(ctx context.Context) ([]models.K8sEvent, error) {
	c.logger.Debug("Getting cluster health events")

	// Define keywords that might indicate cluster health issues
	healthIssueKeywords := []string{
		"Failed", "Error", "CrashLoopBackOff", "OOMKilled", "Evicted",
		"NodeNotReady", "Unhealthy", "OutOfDisk", "MemoryPressure", "DiskPressure",
		"NetworkUnavailable", "Unschedulable",
	}

	// Build field selector for warning events
	fieldSelector := fields.OneTermEqualSelector("type", "Warning")

	// Get events from all namespaces
	eventList, err := c.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{
		FieldSelector: fieldSelector.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list warning events: %w", err)
	}

	// Filter and convert to our model
	var events []models.K8sEvent
	for _, event := range eventList.Items {
		// Check if the event matches any health issue keywords
		matchesKeyword := false
		for _, keyword := range healthIssueKeywords {
			if strings.Contains(event.Reason, keyword) || strings.Contains(event.Message, keyword) {
				matchesKeyword = true
				break
			}
		}

		if !matchesKeyword {
			continue
		}

		e := models.K8sEvent{
			Reason:    event.Reason,
			Message:   event.Message,
			Type:      event.Type,
			Count:     int(event.Count),
			FirstTime: event.FirstTimestamp.Time,
			LastTime:  event.LastTimestamp.Time,
			Object: struct {
				Kind      string `json:"kind"`
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			}{
				Kind:      event.InvolvedObject.Kind,
				Name:      event.InvolvedObject.Name,
				Namespace: event.InvolvedObject.Namespace,
			},
		}
		events = append(events, e)
	}

	// Sort events by last time, most recent first
	sort.Slice(events, func(i, j int) bool {
		return events[i].LastTime.After(events[j].LastTime)
	})

	c.logger.Debug("Got cluster health events", "count", len(events))
	return events, nil
}
