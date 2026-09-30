package mcp

import (
	"strings"
	"testing"
	"time"

	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/k8s"
	"github.com/Blankcut/kubernetes-mcp-server/kubernetes-claude-mcp/internal/models"
)

func TestCronJobIssueCarriesTheLogTail(t *testing.T) {
	cj := k8s.CronJobHealth{
		Name:         "cinc-sync",
		Schedule:     "20 * * * *",
		LastSchedule: time.Date(2026, 9, 30, 13, 20, 0, 0, time.UTC),
		LastSuccess:  time.Date(2026, 9, 30, 12, 28, 58, 0, time.UTC),
		Failing:      true,
		LogTail:      "starting\n\n[syncCincDocuments] documentIds(Governing) failed for fa3b: [object Object]\n",
	}
	is := cronJobIssue(&cj)
	if is.Category != "CronJobFailing" || !strings.Contains(is.Title, "cinc-sync") {
		t.Fatalf("issue = %+v", is)
	}
	for _, want := range []string{"2026-09-30T13:20:00Z", "last success 2026-09-30T12:28:58Z", "[object Object]"} {
		if !strings.Contains(is.Description, want) {
			t.Errorf("description missing %q:\n%s", want, is.Description)
		}
	}
}

func TestCronJobIssueExplainsAMissingLog(t *testing.T) {
	is := cronJobIssue(&k8s.CronJobHealth{
		Name: "x", LastSchedule: time.Now(), Failing: true,
		LogNote: "the failed run's pod has already been cleaned up",
	})
	if !strings.Contains(is.Description, "never succeeded") || !strings.Contains(is.Description, "cleaned up") {
		t.Errorf("description = %q", is.Description)
	}
}

func TestLastLines(t *testing.T) {
	if got := lastLines("a\n\nb\nc\nd\n", 2); got != "c\nd" {
		t.Errorf("lastLines = %q", got)
	}
}

func TestDeterministicSummary(t *testing.T) {
	if got := deterministicSummary("ns", nil); !strings.Contains(got, "No issues") {
		t.Errorf("empty summary = %q", got)
	}
	got := deterministicSummary("ns", []models.Issue{{Title: "Scheduled job failing: x", Description: "d"}})
	if !strings.Contains(got, "1 issue(s)") || !strings.Contains(got, "Scheduled job failing: x") {
		t.Errorf("summary = %q", got)
	}
}
