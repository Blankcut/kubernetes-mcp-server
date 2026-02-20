package models

import (
	"time"
)

// ArgoApplication represents an ArgoCD application
type ArgoApplication struct {
	// These fields might need adjustment based on the actual API response
	Metadata struct {
		Name      string            `json:"name"`
		Namespace string            `json:"namespace"`
		Labels    map[string]string `json:"labels,omitempty"`
	} `json:"metadata"`
	Spec struct {
		Source struct {
			RepoURL        string `json:"repoURL"`
			Path           string `json:"path,omitempty"`
			TargetRevision string `json:"targetRevision,omitempty"`
			Chart          string `json:"chart,omitempty"`
		} `json:"source"`
		Destination struct {
			Server    string `json:"server"`
			Namespace string `json:"namespace"`
		} `json:"destination"`
	} `json:"spec"`
	Status struct {
		Sync struct {
			Status   string `json:"status"`
			Revision string `json:"revision,omitempty"`
		} `json:"sync"`
		Health struct {
			Status string `json:"status"`
		} `json:"health"`
		Resources []ArgoResourceStatus `json:"resources,omitempty"`
	} `json:"status"`
	Name string `json:"name"`
}

// ArgoResourceStatus represents the status of a resource managed by ArgoCD
type ArgoResourceStatus struct {
	Group     string `json:"group"`
	Version   string `json:"version"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Health    struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	} `json:"health"`
}

// ArgoApplicationHistory represents a sync entry in an application's history
type ArgoApplicationHistory struct {
	ID         int64     `json:"id"`
	Revision   string    `json:"revision"`
	DeployedAt time.Time `json:"deployedAt"`
	Status     string    `json:"status"`
}

// ArgoResourceNode represents a node in the application resource tree
type ArgoResourceNode struct {
	Group     string `json:"group"`
	Version   string `json:"version"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Health    struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	} `json:"health"`
	ParentRefs []struct {
		Group     string `json:"group"`
		Version   string `json:"version"`
		Kind      string `json:"kind"`
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	} `json:"parentRefs"`
}

// ArgoResourceTree represents the resource tree of an application
type ArgoResourceTree struct {
	Nodes []ArgoResourceNode `json:"nodes"`
	Edges []struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"edges"`
}
