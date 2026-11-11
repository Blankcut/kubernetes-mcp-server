// Package mcptransport serves this server's cluster reads over the Model
// Context Protocol, so an MCP client (Claude Code, Cursor, and others) can
// connect to it and call them as tools.
//
// For its first four minor versions the project took its name from the
// protocol without speaking it: everything was a REST API. The REST API is
// unchanged and stays the surface other services integrate with. This package
// adds the protocol beside it.
//
// The tools are the REST API's reads. Two things are deliberately different:
//
//   - Nothing here calls Claude. The REST analysis endpoints send what they
//     collect to a model and return its answer; an MCP client is a model, so it
//     gets the collected facts and does its own reasoning. No tool call costs
//     the server's operator anything.
//   - Secret values never leave. The Helm chart's default role cannot read
//     Secrets at all, but a server run against a wider kubeconfig can, and a
//     tool result goes straight into a model's context.
package mcptransport

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/k8s"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/models"
)

// Kubernetes is the cluster read surface the tools use; *k8s.Client satisfies it.
type Kubernetes interface {
	GetNamespaces(ctx context.Context) ([]string, error)
	ListResources(ctx context.Context, kind, namespace string) ([]unstructured.Unstructured, error)
	GetResource(ctx context.Context, kind, namespace, name string) (*unstructured.Unstructured, error)
	GetResourceEvents(ctx context.Context, namespace, kind, name string) ([]models.K8sEvent, error)
}

// Topology maps a namespace's resources; *k8s.ResourceMapper satisfies it.
type Topology interface {
	GetNamespaceTopology(ctx context.Context, namespace string) (*k8s.NamespaceTopology, error)
}

// Troubleshooter collects what is wrong with one resource;
// *correlator.TroubleshootCorrelator satisfies it.
type Troubleshooter interface {
	TroubleshootResource(ctx context.Context, namespace, kind, name string) (*models.TroubleshootResult, error)
}

// ArgoCD reads Applications; *argocd.Client satisfies it.
type ArgoCD interface {
	ListApplications(ctx context.Context) ([]models.ArgoApplication, error)
	GetApplication(ctx context.Context, name string) (*models.ArgoApplication, error)
}

// Deps is what the tools read from. ArgoCD may be nil, in which case the
// ArgoCD tools are not offered.
type Deps struct {
	Kubernetes     Kubernetes
	Topology       Topology
	Troubleshooter Troubleshooter
	ArgoCD         ArgoCD
	Version        string
}

const kindHelp = "Resource kind, singular: pod, deployment, statefulset, daemonset, job, cronjob, service, ingress, configmap, pvc, pv, node, namespace. Other kinds the cluster serves also work by their plural resource name."

const instructions = "Read-only view of a Kubernetes cluster and, when configured, its Argo CD applications. " +
	"Start with troubleshoot_resource when something is failing: it gathers the resource, its events and its related objects in one call. " +
	"Nothing here changes the cluster, and Secret values are never returned."

func readOnly(title string) *mcp.ToolAnnotations {
	closed := false
	return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closed}
}

// NewServer builds the MCP server with every tool registered.
func NewServer(d *Deps) *mcp.Server {
	version := d.Version
	if version == "" {
		version = "dev"
	}
	s := mcp.NewServer(
		&mcp.Implementation{Name: "kubernetes-mcp-server", Title: "Kubernetes MCP Server", Version: version},
		// No logging capability: the server has nothing to push to a client.
		&mcp.ServerOptions{Instructions: instructions, Capabilities: &mcp.ServerCapabilities{}},
	)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_namespaces",
		Description: "List the namespaces in the cluster.",
		Annotations: readOnly("List namespaces"),
	}, d.listNamespaces)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_resources",
		Description: "List resources of one kind, with each one's name, namespace, age and a one-line status. Use get_resource for the full object.",
		Annotations: readOnly("List resources"),
	}, d.listResources)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_resource",
		Description: "Get one resource's full object (spec and status). Secret values are replaced with a placeholder; the key names are kept.",
		Annotations: readOnly("Get resource"),
	}, d.getResource)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_events",
		Description: "Get recent Kubernetes events in a namespace, optionally for one resource. Events explain scheduling, image pull, probe and crash-loop failures.",
		Annotations: readOnly("Get events"),
	}, d.getEvents)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "namespace_topology",
		Description: "Map a namespace: which resources exist, how they relate (a Deployment's Pods, a Service's endpoints, the ConfigMaps and Secrets a Pod mounts) and each one's health.",
		Annotations: readOnly("Namespace topology"),
	}, d.namespaceTopology)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "troubleshoot_resource",
		Description: "Collect what is wrong with one resource: its state, its events, related objects, the deployment history behind it, and the issues and recommendations the server's own checks found. The first call to make when something is failing.",
		Annotations: readOnly("Troubleshoot resource"),
	}, d.troubleshoot)

	if d.ArgoCD != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "list_argocd_applications",
			Description: "List Argo CD applications with their sync and health status.",
			Annotations: readOnly("List Argo CD applications"),
		}, d.listArgoApplications)

		mcp.AddTool(s, &mcp.Tool{
			Name:        "get_argocd_application",
			Description: "Get one Argo CD application: its source, destination, sync status, health and the resources it manages.",
			Annotations: readOnly("Get Argo CD application"),
		}, d.getArgoApplication)
	}

	return s
}

// NewHTTPHandler serves the server over Streamable HTTP.
//
// Stateless, with plain JSON responses: every tool is a single read, so there
// is no session to keep and nothing to stream, and any replica can answer any
// request. The caller puts authentication in front of it.
func NewHTTPHandler(s *mcp.Server) http.Handler {
	return mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return s },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
}

type noInput struct{}

type listResourcesInput struct {
	Kind      string `json:"kind" jsonschema:"Resource kind, singular: pod, deployment, statefulset, daemonset, job, cronjob, service, ingress, configmap, pvc, pv, node, namespace"`
	Namespace string `json:"namespace,omitempty" jsonschema:"Namespace to list in. Omit for every namespace (and for cluster-scoped kinds such as node)."`
}

type getResourceInput struct {
	Kind      string `json:"kind" jsonschema:"Resource kind, singular, e.g. pod or deployment"`
	Name      string `json:"name" jsonschema:"The resource's name"`
	Namespace string `json:"namespace,omitempty" jsonschema:"The resource's namespace. Omit for cluster-scoped kinds such as node."`
}

type getEventsInput struct {
	Namespace string `json:"namespace" jsonschema:"Namespace to read events from"`
	Kind      string `json:"kind,omitempty" jsonschema:"With name: only events about this resource. Kind as Kubernetes writes it, e.g. Pod or Deployment."`
	Name      string `json:"name,omitempty" jsonschema:"With kind: only events about this resource"`
}

type namespaceInput struct {
	Namespace string `json:"namespace" jsonschema:"The namespace"`
}

type nameInput struct {
	Name string `json:"name" jsonschema:"The Argo CD application's name"`
}

func (d *Deps) listNamespaces(ctx context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, any, error) {
	namespaces, err := d.Kubernetes.GetNamespaces(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("could not list namespaces: %w", err)
	}
	sort.Strings(namespaces)
	return nil, map[string]any{"namespaces": namespaces}, nil
}

func (d *Deps) listResources(ctx context.Context, _ *mcp.CallToolRequest, in listResourcesInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Kind) == "" {
		return nil, nil, fmt.Errorf("kind is required. %s", kindHelp)
	}
	items, err := d.Kubernetes.ListResources(ctx, in.Kind, in.Namespace)
	if err != nil {
		return nil, nil, fmt.Errorf("could not list %s: %w", in.Kind, err)
	}
	out := make([]map[string]any, 0, len(items))
	for i := range items {
		out = append(out, summarize(&items[i]))
	}
	return nil, map[string]any{"kind": in.Kind, "count": len(out), "resources": out}, nil
}

func (d *Deps) getResource(ctx context.Context, _ *mcp.CallToolRequest, in getResourceInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Kind) == "" || strings.TrimSpace(in.Name) == "" {
		return nil, nil, fmt.Errorf("kind and name are required. %s", kindHelp)
	}
	obj, err := d.Kubernetes.GetResource(ctx, in.Kind, in.Namespace, in.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("could not get %s %s: %w", in.Kind, in.Name, err)
	}
	return nil, sanitize(obj), nil
}

func (d *Deps) getEvents(ctx context.Context, _ *mcp.CallToolRequest, in getEventsInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Namespace) == "" {
		return nil, nil, fmt.Errorf("namespace is required")
	}
	events, err := d.Kubernetes.GetResourceEvents(ctx, in.Namespace, in.Kind, in.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("could not get events in %s: %w", in.Namespace, err)
	}
	if events == nil {
		events = []models.K8sEvent{}
	}
	return nil, map[string]any{"namespace": in.Namespace, "count": len(events), "events": events}, nil
}

func (d *Deps) namespaceTopology(ctx context.Context, _ *mcp.CallToolRequest, in namespaceInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Namespace) == "" {
		return nil, nil, fmt.Errorf("namespace is required")
	}
	topology, err := d.Topology.GetNamespaceTopology(ctx, in.Namespace)
	if err != nil {
		return nil, nil, fmt.Errorf("could not map namespace %s: %w", in.Namespace, err)
	}
	return nil, topology, nil
}

func (d *Deps) troubleshoot(ctx context.Context, _ *mcp.CallToolRequest, in getResourceInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Kind) == "" || strings.TrimSpace(in.Name) == "" {
		return nil, nil, fmt.Errorf("kind and name are required. %s", kindHelp)
	}
	result, err := d.Troubleshooter.TroubleshootResource(ctx, in.Namespace, in.Kind, in.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("could not troubleshoot %s %s: %w", in.Kind, in.Name, err)
	}
	return nil, result, nil
}

func (d *Deps) listArgoApplications(ctx context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, any, error) {
	apps, err := d.ArgoCD.ListApplications(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("could not list Argo CD applications: %w", err)
	}
	if apps == nil {
		apps = []models.ArgoApplication{}
	}
	return nil, map[string]any{"count": len(apps), "applications": apps}, nil
}

func (d *Deps) getArgoApplication(ctx context.Context, _ *mcp.CallToolRequest, in nameInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, nil, fmt.Errorf("name is required")
	}
	app, err := d.ArgoCD.GetApplication(ctx, in.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("could not get Argo CD application %s: %w", in.Name, err)
	}
	return nil, app, nil
}
