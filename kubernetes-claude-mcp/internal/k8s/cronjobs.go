package k8s

import (
	"context"
	"fmt"
	"sort"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CronJobHealth is what a CronJob's own status says about whether its schedule
// is being met, plus the evidence from its last failed run.
//
// A failing CronJob is invisible to every other health signal: its Deployment
// and Pods read healthy because they are, and the BackoffLimitExceeded event
// that marks the failure expires with the rest of the event stream (an hour by
// default). The CronJob's own lastScheduleTime/lastSuccessfulTime pair is the
// one durable record.
type CronJobHealth struct {
	Name         string    `json:"name"`
	Schedule     string    `json:"schedule"`
	Suspended    bool      `json:"suspended"`
	LastSchedule time.Time `json:"lastSchedule,omitempty"`
	LastSuccess  time.Time `json:"lastSuccess,omitempty"`
	// Failing: the most recent scheduled run finished without succeeding.
	Failing bool `json:"failing"`
	// LastFailedJob is the newest failed Job this CronJob owns, when one is
	// still around (failedJobsHistoryLimit keeps a few).
	LastFailedJob string `json:"lastFailedJob,omitempty"`
	// LogTail is the end of that run's pod log. The cause is never in the
	// CronJob object itself; it is almost always here.
	LogTail string `json:"logTail,omitempty"`
	// LogNote explains a missing LogTail -- usually that the pod was cleaned up
	// by ttlSecondsAfterFinished before anyone looked.
	LogNote string `json:"logNote,omitempty"`
}

// EvaluateCronJob reads a CronJob's status. A run still in progress is not
// failing yet, and a suspended CronJob is not expected to run at all.
func EvaluateCronJob(cj *batchv1.CronJob) CronJobHealth {
	h := CronJobHealth{
		Name:      cj.Name,
		Schedule:  cj.Spec.Schedule,
		Suspended: cj.Spec.Suspend != nil && *cj.Spec.Suspend,
	}
	if t := cj.Status.LastScheduleTime; t != nil {
		h.LastSchedule = t.Time
	}
	if t := cj.Status.LastSuccessfulTime; t != nil {
		h.LastSuccess = t.Time
	}
	running := len(cj.Status.Active) > 0
	h.Failing = !h.Suspended && !running && !h.LastSchedule.IsZero() &&
		(h.LastSuccess.IsZero() || h.LastSuccess.Before(h.LastSchedule))
	return h
}

// LatestFailedJob returns the newest Job owned by the named CronJob that ended
// in failure, or nil.
func LatestFailedJob(jobs []batchv1.Job, cronJobName string) *batchv1.Job {
	var failed []*batchv1.Job
	for i := range jobs {
		j := &jobs[i]
		if !ownedByCronJob(j, cronJobName) || !jobFailed(j) {
			continue
		}
		failed = append(failed, j)
	}
	if len(failed) == 0 {
		return nil
	}
	sort.Slice(failed, func(a, b int) bool {
		return failed[a].CreationTimestamp.After(failed[b].CreationTimestamp.Time)
	})
	return failed[0]
}

func ownedByCronJob(j *batchv1.Job, name string) bool {
	for _, ref := range j.OwnerReferences {
		if ref.Kind == "CronJob" && ref.Name == name {
			return true
		}
	}
	return false
}

func jobFailed(j *batchv1.Job) bool {
	for _, c := range j.Status.Conditions {
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// FailingCronJobs returns the CronJobs in a namespace whose latest scheduled
// run did not succeed, each with the tail of its last failed run's log when
// that pod still exists. Healthy CronJobs are omitted.
func (c *Client) FailingCronJobs(ctx context.Context, namespace string, tailLines int64) ([]CronJobHealth, error) {
	cronJobs, err := c.clientset.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list cronjobs: %w", err)
	}

	var failing []CronJobHealth
	for i := range cronJobs.Items {
		if h := EvaluateCronJob(&cronJobs.Items[i]); h.Failing {
			failing = append(failing, h)
		}
	}
	if len(failing) == 0 {
		return nil, nil
	}

	jobs, err := c.clientset.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		// The CronJob status alone is the diagnosis; logs are a bonus.
		c.logger.Warn("Failed to list jobs for failing cronjobs", "namespace", namespace, "error", err)
		return failing, nil
	}

	for i := range failing {
		job := LatestFailedJob(jobs.Items, failing[i].Name)
		if job == nil {
			failing[i].LogNote = "no failed run is still recorded (failedJobsHistoryLimit may be 0)"
			continue
		}
		failing[i].LastFailedJob = job.Name
		failing[i].LogTail, failing[i].LogNote = c.lastJobPodLog(ctx, namespace, job.Name, tailLines)
	}
	return failing, nil
}

// lastJobPodLog returns the log tail of the newest pod a Job ran, or a note
// saying why there is none.
func (c *Client) lastJobPodLog(ctx context.Context, namespace, jobName string, tailLines int64) (logTail, note string) {
	pods, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "job-name=" + jobName,
	})
	if err != nil {
		return "", "could not list the run's pods: " + err.Error()
	}
	if len(pods.Items) == 0 {
		return "", "the failed run's pod has already been cleaned up (ttlSecondsAfterFinished), so its log is gone"
	}
	sort.Slice(pods.Items, func(a, b int) bool {
		return pods.Items[a].CreationTimestamp.After(pods.Items[b].CreationTimestamp.Time)
	})
	logs, err := c.GetPodLogs(ctx, namespace, pods.Items[0].Name, "", tailLines)
	if err != nil {
		return "", "could not read the run's log: " + err.Error()
	}
	return logs, ""
}
