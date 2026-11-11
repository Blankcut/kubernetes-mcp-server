package mcptransport

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const redacted = "[redacted]"

// lastApplied is kubectl's copy of the manifest it last applied. For a Secret
// created with `kubectl apply` it holds the values in the clear.
const lastApplied = "kubectl.kubernetes.io/last-applied-configuration"

// sanitize returns the object as a tool result: a copy, never the caller's.
//
//   - A Secret's data and stringData keep their key names and lose their
//     values. Which keys exist is what troubleshooting needs ("the Pod wants
//     DATABASE_URL and the Secret has no such key"); the values are not.
//   - The last-applied annotation goes, on every kind: it repeats the spec, and
//     on a Secret it repeats the values.
//   - managedFields goes: field-ownership bookkeeping, often larger than the
//     object, and no help in reading it.
func sanitize(obj *unstructured.Unstructured) map[string]any {
	if obj == nil {
		return map[string]any{}
	}
	out := obj.DeepCopy()

	unstructured.RemoveNestedField(out.Object, "metadata", "managedFields")
	if ann := out.GetAnnotations(); ann != nil {
		if _, ok := ann[lastApplied]; ok {
			delete(ann, lastApplied)
			out.SetAnnotations(ann)
		}
	}

	if strings.EqualFold(out.GetKind(), "Secret") {
		for _, field := range []string{"data", "stringData"} {
			values, found, _ := unstructured.NestedMap(out.Object, field)
			if !found {
				continue
			}
			for k := range values {
				values[k] = redacted
			}
			_ = unstructured.SetNestedMap(out.Object, values, field)
		}
	}
	return out.Object
}

// summarize is one row of list_resources: enough to pick the resource to look
// at, without the whole object. A list of full Pods is hundreds of kilobytes.
func summarize(obj *unstructured.Unstructured) map[string]any {
	row := map[string]any{"name": obj.GetName()}
	if ns := obj.GetNamespace(); ns != "" {
		row["namespace"] = ns
	}
	if ts := obj.GetCreationTimestamp(); !ts.IsZero() {
		row["created"] = ts.UTC().Format("2006-01-02T15:04:05Z")
	}
	if status := statusLine(obj); status != "" {
		row["status"] = status
	}
	return row
}

// statusLine is the one-line state of the kinds whose state is worth a line.
func statusLine(obj *unstructured.Unstructured) string {
	o := obj.Object
	switch strings.ToLower(obj.GetKind()) {
	case "pod":
		phase, _, _ := unstructured.NestedString(o, "status", "phase")
		statuses, _, _ := unstructured.NestedSlice(o, "status", "containerStatuses")
		ready, restarts := 0, int64(0)
		reason := ""
		for _, raw := range statuses {
			cs, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if r, _, _ := unstructured.NestedBool(cs, "ready"); r {
				ready++
			}
			n, _, _ := unstructured.NestedInt64(cs, "restartCount")
			restarts += n
			// The waiting reason is what "kubectl get pods" shows in place of
			// the phase: CrashLoopBackOff, ImagePullBackOff, and so on.
			if w, _, _ := unstructured.NestedString(cs, "state", "waiting", "reason"); w != "" && reason == "" {
				reason = w
			}
		}
		if reason == "" {
			reason = phase
		}
		return fmt.Sprintf("%s, %d/%d ready, %d restarts", reason, ready, len(statuses), restarts)
	case "deployment", "statefulset":
		want, found, _ := unstructured.NestedInt64(o, "spec", "replicas")
		if !found {
			want = 1
		}
		ready, _, _ := unstructured.NestedInt64(o, "status", "readyReplicas")
		return fmt.Sprintf("%d/%d ready", ready, want)
	case "daemonset":
		want, _, _ := unstructured.NestedInt64(o, "status", "desiredNumberScheduled")
		ready, _, _ := unstructured.NestedInt64(o, "status", "numberReady")
		return fmt.Sprintf("%d/%d ready", ready, want)
	case "job":
		succeeded, _, _ := unstructured.NestedInt64(o, "status", "succeeded")
		failed, _, _ := unstructured.NestedInt64(o, "status", "failed")
		active, _, _ := unstructured.NestedInt64(o, "status", "active")
		return fmt.Sprintf("%d succeeded, %d failed, %d active", succeeded, failed, active)
	case "cronjob":
		schedule, _, _ := unstructured.NestedString(o, "spec", "schedule")
		suspended, _, _ := unstructured.NestedBool(o, "spec", "suspend")
		last, _, _ := unstructured.NestedString(o, "status", "lastScheduleTime")
		line := "schedule " + schedule
		if suspended {
			line += ", suspended"
		}
		if last != "" {
			line += ", last run " + last
		}
		return line
	case "namespace", "pv", "persistentvolume", "pvc", "persistentvolumeclaim":
		phase, _, _ := unstructured.NestedString(o, "status", "phase")
		return phase
	case "node":
		conditions, _, _ := unstructured.NestedSlice(o, "status", "conditions")
		for _, raw := range conditions {
			c, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if t, _, _ := unstructured.NestedString(c, "type"); t == "Ready" {
				if s, _, _ := unstructured.NestedString(c, "status"); s == "True" {
					return "Ready"
				}
				return "NotReady"
			}
		}
	}
	return ""
}
