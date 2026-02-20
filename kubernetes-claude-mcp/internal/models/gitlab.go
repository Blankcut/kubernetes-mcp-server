package models

// GitLabProject represents a GitLab project
type GitLabProject struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	Path              string `json:"path"`
	PathWithNamespace string `json:"path_with_namespace"`
	WebURL            string `json:"web_url"`
	DefaultBranch     string `json:"default_branch"`
	Visibility        string `json:"visibility"`
}

// GitLabPipeline represents a GitLab CI/CD pipeline
type GitLabPipeline struct {
	ID        int         `json:"id"`
	Status    string      `json:"status"`
	Ref       string      `json:"ref"`
	SHA       string      `json:"sha"`
	WebURL    string      `json:"web_url"`
	CreatedAt interface{} `json:"created_at"`
	UpdatedAt interface{} `json:"updated_at"`
}

// GitLabJob represents a job in a GitLab CI/CD pipeline
type GitLabJob struct {
	ID         int    `json:"id"`
	Status     string `json:"status"`
	Stage      string `json:"stage"`
	Name       string `json:"name"`
	Ref        string `json:"ref"`
	CreatedAt  int64  `json:"created_at"`
	StartedAt  int64  `json:"started_at"`
	FinishedAt int64  `json:"finished_at"`
	Pipeline   struct {
		ID int `json:"id"`
	} `json:"pipeline"`
}

// GitLabCommit represents a Git commit in GitLab
type GitLabCommit struct {
	ID             string      `json:"id"`
	ShortID        string      `json:"short_id"`
	Title          string      `json:"title"`
	Message        string      `json:"message"`
	AuthorName     string      `json:"author_name"`
	AuthorEmail    string      `json:"author_email"`
	CommitterName  string      `json:"committer_name"`
	CommitterEmail string      `json:"committer_email"`
	CreatedAt      interface{} `json:"created_at"`
	ParentIDs      []string    `json:"parent_ids"`
	WebURL         string      `json:"web_url"`
}

// GitLabDiff represents a file diff in a commit
type GitLabDiff struct {
	OldPath     string `json:"old_path"`
	NewPath     string `json:"new_path"`
	Diff        string `json:"diff"`
	NewFile     bool   `json:"new_file"`
	RenamedFile bool   `json:"renamed_file"`
	DeletedFile bool   `json:"deleted_file"`
}

// GitLabDeployment represents a deployment in GitLab
type GitLabDeployment struct {
	ID          int         `json:"id"`
	Status      string      `json:"status"`
	CreatedAt   interface{} `json:"created_at"`
	UpdatedAt   interface{} `json:"updated_at"`
	Environment struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Slug  string `json:"slug"`
		State string `json:"state"`
	} `json:"environment"`
	Deployable struct {
		ID       int    `json:"id"`
		Status   string `json:"status"`
		Stage    string `json:"stage"`
		Name     string `json:"name"`
		Ref      string `json:"ref"`
		Tag      bool   `json:"tag"`
		Pipeline struct {
			ID     int    `json:"id"`
			Status string `json:"status"`
		} `json:"pipeline"`
	} `json:"deployable"`
	Commit GitLabCommit `json:"commit"`
}

// GitLabRelease represents a release in GitLab
type GitLabRelease struct {
	TagName     string `json:"tag_name"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"created_at"`
	Assets      struct {
		Links []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"links"`
	} `json:"assets"`
}

// GitLabMergeRequest represents a merge request in GitLab
type GitLabMergeRequest struct {
	ID          int    `json:"id"`
	IID         int    `json:"iid"`
	ProjectID   int    `json:"project_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	MergedBy    *struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
	} `json:"merged_by,omitempty"`
	MergedAt     interface{} `json:"merged_at"`
	CreatedAt    interface{} `json:"created_at"`
	UpdatedAt    interface{} `json:"updated_at"`
	TargetBranch string      `json:"target_branch"`
	SourceBranch string      `json:"source_branch"`
	Author       struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
	} `json:"author"`
	Assignees []struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
	} `json:"assignees"`
	SourceProjectID int    `json:"source_project_id"`
	TargetProjectID int    `json:"target_project_id"`
	WebURL          string `json:"web_url"`
	MergeStatus     string `json:"merge_status"`
	Changes         []struct {
		OldPath     string `json:"old_path"`
		NewPath     string `json:"new_path"`
		Diff        string `json:"diff"`
		NewFile     bool   `json:"new_file"`
		RenamedFile bool   `json:"renamed_file"`
		DeletedFile bool   `json:"deleted_file"`
	} `json:"changes,omitempty"`
	DiffRefs struct {
		BaseSHA  string `json:"base_sha"`
		HeadSHA  string `json:"head_sha"`
		StartSHA string `json:"start_sha"`
	} `json:"diff_refs"`
	UserNotesCount      int              `json:"user_notes_count"`
	HasConflicts        bool             `json:"has_conflicts"`
	Pipelines           []GitLabPipeline `json:"pipelines,omitempty"`
	MergeRequestContext struct {
		CommitMessages     []string `json:"commit_messages,omitempty"`
		AffectedFiles      []string `json:"affected_files,omitempty"`
		HelmChartAffected  bool     `json:"helm_chart_affected,omitempty"`
		KubernetesManifest bool     `json:"kubernetes_manifests_affected,omitempty"`
	} `json:"merge_request_context,omitempty"`
}

// GitLabMergeRequestComment represents a comment on a GitLab merge request
type GitLabMergeRequestComment struct {
	ID           int    `json:"id"`
	Body         string `json:"body"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	System       bool   `json:"system"`
	NoteableID   int    `json:"noteable_id"`   //nolint:misspell // GitLab API uses "noteable"
	NoteableType string `json:"noteable_type"` //nolint:misspell // GitLab API uses "noteable"
	Author       struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
	} `json:"author"`
}

// GitLabMergeRequestApproval represents approval information for a merge request
type GitLabMergeRequestApproval struct {
	ID               int  `json:"id"`
	ProjectID        int  `json:"project_id"`
	ApprovalRequired bool `json:"approval_required"`
	ApprovedBy       []struct {
		User struct {
			ID       int    `json:"id"`
			Username string `json:"username"`
			Name     string `json:"name"`
		} `json:"user"`
	} `json:"approved_by"`
	ApprovalsRequired int `json:"approvals_required"`
	ApprovalsLeft     int `json:"approvals_left"`
}
