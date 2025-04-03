// internal/helm/parser.go

package helm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/pkg/logging"
)

// Parser handles Helm chart parsing and analysis
type Parser struct {
	workDir string
	logger  *logging.Logger
}

// NewParser creates a new Helm chart parser
func NewParser(logger *logging.Logger) *Parser {
	if logger == nil {
		logger = logging.NewLogger().Named("helm")
	}

	// Create a temporary working directory
	workDir, err := os.MkdirTemp("", "helm-parser-*")
	if err != nil {
		logger.Error("Failed to create working directory", "error", err)
		return nil
	}

	return &Parser{
		workDir: workDir,
		logger:  logger,
	}
}

// ParseChart renders a Helm chart and returns the resulting Kubernetes manifests
func (p *Parser) ParseChart(ctx context.Context, chartPath string, valuesFiles []string, values map[string]interface{}) ([]string, error) {
	p.logger.Debug("Parsing Helm chart", "chartPath", chartPath, "valuesFiles", valuesFiles)

	// Check if helm command is available
	if _, err := exec.LookPath("helm"); err != nil {
		return nil, fmt.Errorf("helm command not found in PATH: %w", err)
	}

	// Prepare helm template command
	args := []string{"template", "release", chartPath}

	// Add values files
	for _, valuesFile := range valuesFiles {
		args = append(args, "-f", valuesFile)
	}

	// Add --set arguments for values
	for k, v := range values {
		args = append(args, "--set", fmt.Sprintf("%s=%v", k, v))
	}

	// Execute helm template command
	cmd := exec.CommandContext(ctx, "helm", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	p.logger.Debug("Executing helm template command", "args", args)
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to execute helm template: %s, error: %w", stderr.String(), err)
	}

	// Parse the rendered templates
	manifests := p.splitYAMLDocuments(stdout.String())
	p.logger.Debug("Parsed Helm chart", "manifestCount", len(manifests))

	return manifests, nil
}

// WriteChartFiles writes chart files to the working directory for processing
func (p *Parser) WriteChartFiles(files map[string]string) (string, error) {
	chartDir := filepath.Join(p.workDir, "chart")

	// Create chart directory if not exists
	if err := os.MkdirAll(chartDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create chart directory: %w", err)
	}

	// Write files
	for path, content := range files {
		fullPath := filepath.Join(chartDir, path)
		dirPath := filepath.Dir(fullPath)

		// Create directories
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return "", fmt.Errorf("failed to create directory %s: %w", dirPath, err)
		}

		// Write file
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("failed to write file %s: %w", fullPath, err)
		}
	}

	return chartDir, nil
}

// WriteValuesFile writes a values file to the working directory
func (p *Parser) WriteValuesFile(content string) (string, error) {
	valuesFile := filepath.Join(p.workDir, "values.yaml")

	if err := os.WriteFile(valuesFile, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write values file: %w", err)
	}

	return valuesFile, nil
}

// ParseYAML parses a YAML file to extract Kubernetes resources
func (p *Parser) ParseYAML(content string) ([]map[string]interface{}, error) {
	// Split YAML documents
	documents := p.splitYAMLDocuments(content)

	var resources []map[string]interface{}

	for _, doc := range documents {
		// Parse each document as YAML
		var resource map[string]interface{}

		// *** Add this line (or similar depending on your library) ***
		err := yaml.Unmarshal([]byte(doc), &resource) // Use your chosen library's unmarshal function
		if err != nil {
			// Handle the error appropriately, maybe log it and continue
			p.logger.Warn("Failed to unmarshal YAML document", "error", err)
			continue
		}

		// Add to resources if it's a valid Kubernetes resource (and not empty after parsing)
		if resource != nil {
			resources = append(resources, resource)
		}
	}

	return resources, nil
}

// splitYAMLDocuments splits multi-document YAML into individual documents
func (p *Parser) splitYAMLDocuments(content string) []string {
	// Simple implementation - in a real system, use a proper YAML parser
	var documents []string

	// Split on document separator
	parts := strings.Split(content, "---")

	for _, part := range parts {
		// Trim whitespace
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			documents = append(documents, trimmed)
		}
	}

	return documents
}

// Cleanup removes temporary files
func (p *Parser) Cleanup() {
	if p.workDir != "" {
		p.logger.Debug("Cleaning up working directory", "path", p.workDir)
		os.RemoveAll(p.workDir)
	}
}

// DiffChartVersions compares two versions of a chart and returns resources that would be affected
func (p *Parser) DiffChartVersions(ctx context.Context, chartPath1, chartPath2 string, valuesFiles []string) ([]string, error) {
	// Render both chart versions
	manifests1, err := p.ParseChart(ctx, chartPath1, valuesFiles, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse first chart version: %w", err)
	}

	manifests2, err := p.ParseChart(ctx, chartPath2, valuesFiles, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse second chart version: %w", err)
	}

	// Compare manifests to find differences
	diff := p.compareManifests(manifests1, manifests2)

	return diff, nil
}

// compareManifests compares two sets of manifests and returns the names of resources that differ
func (p *Parser) compareManifests(manifests1, manifests2 []string) []string {
	// This is a simplified implementation
	// In a real system, you would parse the YAML and compare by resource identifiers

	var changedResources []string

	// For now, we just assume all manifests might be affected
	// In a real implementation, you'd compare name/kind/namespace

	for _, manifest := range manifests2 {
		// Extract resource name and kind
		if strings.Contains(manifest, "kind:") && strings.Contains(manifest, "name:") {
			// Very simplistic parsing - would need proper YAML parsing in real code
			lines := strings.Split(manifest, "\n")
			var kind, name string

			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "kind:") {
					kind = strings.TrimSpace(strings.TrimPrefix(line, "kind:"))
				} else if strings.HasPrefix(line, "name:") {
					name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
				}

				if kind != "" && name != "" {
					changedResources = append(changedResources, fmt.Sprintf("%s/%s", kind, name))
					break
				}
			}
		}
	}

	return changedResources
}
