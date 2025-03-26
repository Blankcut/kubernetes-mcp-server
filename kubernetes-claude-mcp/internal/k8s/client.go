package k8s

import (
	"context"
	"fmt"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/config"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
)

// Client wraps the Kubernetes clientset and provides additional functionality
type Client struct {
	clientset       *kubernetes.Clientset
	dynamicClient   dynamic.Interface
	discoveryClient *discovery.DiscoveryClient
	restConfig      *rest.Config
	defaultNS       string
	logger          *logging.Logger
	ResourceMapper  *ResourceMapper
}

// NewClient creates a new Kubernetes client based on the provided configuration
func NewClient(cfg config.KubernetesConfig, logger *logging.Logger) (*Client, error) {
	if logger == nil {
		logger = logging.NewLogger().Named("k8s")
	}

	var restConfig *rest.Config
	var err error

	logger.Debug("Initializing Kubernetes client",
		"inCluster", cfg.InCluster,
		"kubeconfig", cfg.KubeConfig,
		"defaultNamespace", cfg.DefaultNamespace)

	if cfg.InCluster {
		// Use in-cluster config when deployed inside Kubernetes
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to create in-cluster config: %w", err)
		}
		logger.Debug("Using in-cluster configuration")
	} else {
		// Use kubeconfig file
		kubeconfigPath := cfg.KubeConfig
		if kubeconfigPath == "" {
			// Try to use default location if not specified
			if home := homedir.HomeDir(); home != "" {
				kubeconfigPath = filepath.Join(home, ".kube", "config")
				logger.Debug("Using default kubeconfig path", "path", kubeconfigPath)
			} else {
				return nil, fmt.Errorf("kubeconfig not specified and home directory not found")
			}
		}

		// Build config from kubeconfig file
		configLoadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
		configOverrides := &clientcmd.ConfigOverrides{}

		if cfg.DefaultContext != "" {
			configOverrides.CurrentContext = cfg.DefaultContext
			logger.Debug("Using specified context", "context", cfg.DefaultContext)
		}

		kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			configLoadingRules,
			configOverrides,
		)

		restConfig, err = kubeConfig.ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
		}
	}

	// Increase QPS and Burst for better performance in busy environments
	restConfig.QPS = 100
	restConfig.Burst = 100

	// Create clientset
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}

	// Create dynamic client
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	// Create discovery client
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %w", err)
	}

	defaultNamespace := cfg.DefaultNamespace
	if defaultNamespace == "" {
		defaultNamespace = "default"
	}

	logger.Info("Kubernetes client initialized",
		"defaultNamespace", defaultNamespace)

	// Create the client instance
	client := &Client{
		clientset:       clientset,
		dynamicClient:   dynamicClient,
		discoveryClient: discoveryClient,
		restConfig:      restConfig,
		defaultNS:       defaultNamespace,
		logger:          logger,
	}

	// Initialize the ResourceMapper (ensure NewResourceMapper is defined in your package)
	client.ResourceMapper = NewResourceMapper(client)

	return client, nil
}

// CheckConnectivity verifies connectivity to the Kubernetes API
func (c *Client) CheckConnectivity(ctx context.Context) error {
	c.logger.Debug("Checking Kubernetes connectivity")

	// Try to get server version as a basic connectivity test
	_, err := c.clientset.Discovery().ServerVersion()
	if err != nil {
		c.logger.Warn("Kubernetes connectivity check failed", "error", err)
		return fmt.Errorf("failed to connect to Kubernetes API: %w", err)
	}

	c.logger.Debug("Kubernetes connectivity check successful")
	return nil
}

// GetNamespaces returns a list of all namespaces in the cluster
func (c *Client) GetNamespaces(ctx context.Context) ([]string, error) {
	c.logger.Debug("Getting namespaces")

	namespaceList, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	var namespaces []string
	for _, ns := range namespaceList.Items {
		namespaces = append(namespaces, ns.Name)
	}

	c.logger.Debug("Got namespaces", "count", len(namespaces))
	return namespaces, nil
}

// GetDefaultNamespace returns the default namespace for operations
func (c *Client) GetDefaultNamespace() string {
	return c.defaultNS
}

// GetRestConfig returns the Kubernetes REST configuration
func (c *Client) GetRestConfig() *rest.Config {
	return c.restConfig
}

// GetClientset returns the Kubernetes clientset
func (c *Client) GetClientset() *kubernetes.Clientset {
	return c.clientset
}

// GetDynamicClient returns the dynamic client
func (c *Client) GetDynamicClient() dynamic.Interface {
	return c.dynamicClient
}

// GetDiscoveryClient returns the discovery client
func (c *Client) GetDiscoveryClient() *discovery.DiscoveryClient {
	return c.discoveryClient
}

// GetNamespaceTopology returns the topology for a specific namespace
func (c *Client) GetNamespaceTopology(ctx context.Context, namespace string) (*NamespaceTopology, error) {
	return c.ResourceMapper.GetNamespaceTopology(ctx, namespace)
}
