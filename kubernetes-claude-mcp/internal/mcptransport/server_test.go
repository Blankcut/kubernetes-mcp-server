package mcptransport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/k8s"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/models"
)

type fakeCluster struct {
	objects map[string]*unstructured.Unstructured // "kind/namespace/name"
	listErr error
}

func (f *fakeCluster) GetNamespaces(context.Context) ([]string, error) {
	return []string{"web", "default"}, nil
}

func (f *fakeCluster) ListResources(_ context.Context, kind, namespace string) ([]unstructured.Unstructured, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []unstructured.Unstructured
	for key, obj := range f.objects {
		if strings.HasPrefix(key, kind+"/") && (namespace == "" || obj.GetNamespace() == namespace) {
			out = append(out, *obj)
		}
	}
	return out, nil
}

func (f *fakeCluster) GetResource(_ context.Context, kind, namespace, name string) (*unstructured.Unstructured, error) {
	obj, ok := f.objects[kind+"/"+namespace+"/"+name]
	if !ok {
		return nil, errors.New("not found")
	}
	return obj, nil
}

func (f *fakeCluster) GetResourceEvents(context.Context, string, string, string) ([]models.K8sEvent, error) {
	return []models.K8sEvent{{Reason: "BackOff", Message: "Back-off restarting failed container", Type: "Warning", Count: 7}}, nil
}

func (f *fakeCluster) GetNamespaceTopology(_ context.Context, namespace string) (*k8s.NamespaceTopology, error) {
	return &k8s.NamespaceTopology{Namespace: namespace, Resources: map[string][]string{"Pod": {"api-1"}}}, nil
}

func (f *fakeCluster) TroubleshootResource(_ context.Context, _, kind, name string) (*models.TroubleshootResult, error) {
	return &models.TroubleshootResult{Recommendations: []string{"check the image tag of " + kind + " " + name}}, nil
}

type fakeArgo struct{}

func (fakeArgo) ListApplications(context.Context) ([]models.ArgoApplication, error) {
	return []models.ArgoApplication{{}}, nil
}

func (fakeArgo) GetApplication(context.Context, string) (*models.ArgoApplication, error) {
	return &models.ArgoApplication{}, nil
}

func object(kind, namespace, name string, rest map[string]any) *unstructured.Unstructured {
	o := map[string]any{
		"apiVersion": "v1",
		"kind":       kind,
		"metadata": map[string]any{
			"name":              name,
			"namespace":         namespace,
			"creationTimestamp": "2026-11-01T10:00:00Z",
			"managedFields":     []any{map[string]any{"manager": "kubectl"}},
			"annotations": map[string]any{
				lastApplied: `{"data":{"DATABASE_URL":"cG9zdGdyZXM6Ly9zZWNyZXQ="}}`,
				"team":      "web",
			},
		},
	}
	for k, v := range rest {
		o[k] = v
	}
	return &unstructured.Unstructured{Object: o}
}

func testCluster() *fakeCluster {
	return &fakeCluster{objects: map[string]*unstructured.Unstructured{
		"pod/web/api-1": object("Pod", "web", "api-1", map[string]any{
			"status": map[string]any{
				"phase": "Running",
				"containerStatuses": []any{map[string]any{
					"ready":        false,
					"restartCount": int64(12),
					"state":        map[string]any{"waiting": map[string]any{"reason": "CrashLoopBackOff"}},
				}},
			},
		}),
		"secret/web/api-env": object("Secret", "web", "api-env", map[string]any{
			"data":       map[string]any{"DATABASE_URL": "cG9zdGdyZXM6Ly9zZWNyZXQ=", "API_TOKEN": "dG9rZW4="},
			"stringData": map[string]any{"PLAIN": "hunter2"},
		}),
	}}
}

// connect serves the tools over Streamable HTTP, as the API server mounts
// them, and returns a real MCP client session connected to it.
func connect(t *testing.T, d Deps) *mcp.ClientSession { //nolint:gocritic // a test helper; the copy is the point
	t.Helper()
	srv := httptest.NewServer(NewHTTPHandler(NewServer(&d)))
	t.Cleanup(srv.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: srv.URL}, nil)
	if err != nil {
		t.Fatalf("an MCP client could not connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func deps(c *fakeCluster) Deps {
	return Deps{Kubernetes: c, Topology: c, Troubleshooter: c, ArgoCD: fakeArgo{}, Version: "test"}
}

// call returns the tool's result as the JSON text a model would read.
func call(t *testing.T, s *mcp.ClientSession, name string, args map[string]any) (text string, isError bool) {
	t.Helper()
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: protocol error: %v", name, err)
	}
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text += tc.Text
		}
	}
	return text, res.IsError
}

func TestAnMCPClientCanListAndCallTheTools(t *testing.T) {
	s := connect(t, deps(testCluster()))

	tools, err := s.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	got := map[string]bool{}
	for _, tool := range tools.Tools {
		got[tool.Name] = true
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("%s is not marked read-only; every tool here only reads", tool.Name)
		}
		if tool.Description == "" {
			t.Errorf("%s has no description", tool.Name)
		}
	}
	for _, want := range []string{
		"list_namespaces", "list_resources", "get_resource", "get_events",
		"namespace_topology", "troubleshoot_resource",
		"list_argocd_applications", "get_argocd_application",
	} {
		if !got[want] {
			t.Errorf("tool %s is not offered (offered: %v)", want, got)
		}
	}

	text, isErr := call(t, s, "list_namespaces", nil)
	if isErr || !strings.Contains(text, `"default"`) || !strings.Contains(text, `"web"`) {
		t.Errorf("list_namespaces = %q (error=%v)", text, isErr)
	}

	text, isErr = call(t, s, "list_resources", map[string]any{"kind": "pod", "namespace": "web"})
	if isErr || !strings.Contains(text, "CrashLoopBackOff, 0/1 ready, 12 restarts") {
		t.Errorf("list_resources should give each pod's one-line status, got %q (error=%v)", text, isErr)
	}
	if strings.Contains(text, "containerStatuses") {
		t.Errorf("list_resources returned whole objects, not summaries: %q", text)
	}

	text, isErr = call(t, s, "get_events", map[string]any{"namespace": "web"})
	if isErr || !strings.Contains(text, "BackOff") {
		t.Errorf("get_events = %q (error=%v)", text, isErr)
	}

	text, isErr = call(t, s, "troubleshoot_resource", map[string]any{"kind": "pod", "name": "api-1", "namespace": "web"})
	if isErr || !strings.Contains(text, "check the image tag of pod api-1") {
		t.Errorf("troubleshoot_resource = %q (error=%v)", text, isErr)
	}

	text, isErr = call(t, s, "namespace_topology", map[string]any{"namespace": "web"})
	if isErr || !strings.Contains(text, "api-1") {
		t.Errorf("namespace_topology = %q (error=%v)", text, isErr)
	}
}

// A tool result goes straight into a model's context. The chart's role cannot
// read Secrets, but a server run against a wider kubeconfig can.
func TestSecretValuesNeverLeave(t *testing.T) {
	s := connect(t, deps(testCluster()))

	text, isErr := call(t, s, "get_resource", map[string]any{"kind": "secret", "name": "api-env", "namespace": "web"})
	if isErr {
		t.Fatalf("get_resource failed: %s", text)
	}
	for _, leaked := range []string{"cG9zdGdyZXM6Ly9zZWNyZXQ=", "dG9rZW4=", "hunter2"} {
		if strings.Contains(text, leaked) {
			t.Errorf("a Secret value reached the client: %q in %s", leaked, text)
		}
	}
	for _, key := range []string{"DATABASE_URL", "API_TOKEN", "PLAIN"} {
		if !strings.Contains(text, key) {
			t.Errorf("key name %s was dropped; which keys exist is what troubleshooting needs", key)
		}
	}
	if strings.Contains(text, "managedFields") {
		t.Errorf("managedFields was returned")
	}
	if !strings.Contains(text, `"team"`) {
		t.Errorf("ordinary annotations should be kept: %s", text)
	}
}

func TestSanitizeLeavesTheCallersObjectAlone(t *testing.T) {
	secret := testCluster().objects["secret/web/api-env"]
	sanitize(secret)
	data, _, _ := unstructured.NestedMap(secret.Object, "data")
	if data["API_TOKEN"] != "dG9rZW4=" {
		t.Errorf("sanitize changed the object it was given: %v", data)
	}
}

// The last-applied annotation repeats the manifest, values included, on any kind.
func TestLastAppliedAnnotationIsDroppedOnEveryKind(t *testing.T) {
	s := connect(t, deps(testCluster()))
	text, _ := call(t, s, "get_resource", map[string]any{"kind": "pod", "name": "api-1", "namespace": "web"})
	if strings.Contains(text, "last-applied-configuration") {
		t.Errorf("last-applied-configuration was returned: %s", text)
	}
}

func TestAFailedReadIsAToolErrorTheModelCanRead(t *testing.T) {
	c := testCluster()
	c.listErr = errors.New(`pods is forbidden: User "system:serviceaccount:x:y" cannot list resource "pods"`)
	s := connect(t, deps(c))

	text, isErr := call(t, s, "list_resources", map[string]any{"kind": "pod"})
	if !isErr || !strings.Contains(text, "forbidden") {
		t.Errorf("want a tool error carrying the cause, got %q (error=%v)", text, isErr)
	}

	text, isErr = call(t, s, "get_resource", map[string]any{"kind": "pod", "name": "nope", "namespace": "web"})
	if !isErr || !strings.Contains(text, "not found") {
		t.Errorf("want a tool error for a missing resource, got %q (error=%v)", text, isErr)
	}
}

func TestArgoCDToolsAreNotOfferedWithoutArgoCD(t *testing.T) {
	c := testCluster()
	s := connect(t, Deps{Kubernetes: c, Topology: c, Troubleshooter: c})
	tools, err := s.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	for _, tool := range tools.Tools {
		if strings.Contains(tool.Name, "argocd") {
			t.Errorf("%s offered with no ArgoCD client behind it", tool.Name)
		}
	}
}

func TestStatusLines(t *testing.T) {
	for _, tt := range []struct {
		name string
		obj  map[string]any
		want string
	}{
		{"deployment", map[string]any{"kind": "Deployment", "spec": map[string]any{"replicas": int64(3)}, "status": map[string]any{"readyReplicas": int64(1)}}, "1/3 ready"},
		{"deployment with replicas unset", map[string]any{"kind": "Deployment", "status": map[string]any{"readyReplicas": int64(1)}}, "1/1 ready"},
		{"job", map[string]any{"kind": "Job", "status": map[string]any{"failed": int64(2)}}, "0 succeeded, 2 failed, 0 active"},
		{"suspended cronjob", map[string]any{"kind": "CronJob", "spec": map[string]any{"schedule": "0 9 * * *", "suspend": true}}, "schedule 0 9 * * *, suspended"},
		{"node", map[string]any{"kind": "Node", "status": map[string]any{"conditions": []any{map[string]any{"type": "Ready", "status": "False"}}}}, "NotReady"},
		{"a kind with no status line", map[string]any{"kind": "ConfigMap"}, ""},
	} {
		if got := statusLine(&unstructured.Unstructured{Object: tt.obj}); got != tt.want {
			t.Errorf("%s: statusLine = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// Results must be JSON objects: structured content that is an array or a bare
// value is not valid under the protocol.
func TestEveryResultIsAJSONObject(t *testing.T) {
	s := connect(t, deps(testCluster()))
	for name, args := range map[string]map[string]any{
		"list_namespaces":          nil,
		"list_resources":           {"kind": "pod"},
		"get_events":               {"namespace": "web"},
		"list_argocd_applications": nil,
	} {
		text, _ := call(t, s, name, args)
		var obj map[string]any
		if err := json.Unmarshal([]byte(text), &obj); err != nil {
			t.Errorf("%s did not return a JSON object: %v (%q)", name, err, text)
		}
	}
}

// The release workflow refuses a tag the chart's appVersion does not match;
// this keeps what the server tells MCP clients in step with both.
func TestVersionMatchesChart(t *testing.T) {
	chart, err := os.ReadFile("../../../helm/kubernetes-mcp-server/Chart.yaml")
	if err != nil {
		t.Skipf("chart not found from here: %v", err)
	}
	m := regexp.MustCompile(`(?m)^appVersion:\s*"?([^"\s]+)"?`).FindSubmatch(chart)
	if m == nil {
		t.Fatal("no appVersion in Chart.yaml")
	}
	if string(m[1]) != Version {
		t.Errorf("mcptransport.Version is %q but the chart's appVersion is %q; change them together", Version, m[1])
	}
}
