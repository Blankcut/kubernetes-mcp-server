package k8s

import (
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func cronJob(schedule, success time.Time, active int, suspend bool) *batchv1.CronJob {
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "cinc-sync"},
		Spec:       batchv1.CronJobSpec{Schedule: "20 * * * *", Suspend: &suspend},
	}
	if !schedule.IsZero() {
		cj.Status.LastScheduleTime = &metav1.Time{Time: schedule}
	}
	if !success.IsZero() {
		cj.Status.LastSuccessfulTime = &metav1.Time{Time: success}
	}
	for i := 0; i < active; i++ {
		cj.Status.Active = append(cj.Status.Active, corev1.ObjectReference{Name: "run"})
	}
	return cj
}

func TestEvaluateCronJob(t *testing.T) {
	base := time.Date(2026, 9, 30, 13, 20, 0, 0, time.UTC)
	cases := []struct {
		name string
		cj   *batchv1.CronJob
		want bool
	}{
		// vine-notices on 2026-09-30: last success 12:28, scheduled again 13:20, failed.
		{"latest run failed", cronJob(base, base.Add(-52*time.Minute), 0, false), true},
		{"latest run succeeded", cronJob(base, base.Add(8*time.Minute), 0, false), false},
		{"never succeeded", cronJob(base, time.Time{}, 0, false), true},
		// Mid-run the success time trails the schedule, but nothing has failed yet.
		{"run in progress", cronJob(base, base.Add(-52*time.Minute), 1, false), false},
		{"suspended", cronJob(base, base.Add(-52*time.Minute), 0, true), false},
		{"never scheduled", cronJob(time.Time{}, time.Time{}, 0, false), false},
	}
	for _, c := range cases {
		if got := EvaluateCronJob(c.cj).Failing; got != c.want {
			t.Errorf("%s: Failing = %v, want %v", c.name, got, c.want)
		}
	}
}

func job(name, owner string, created time.Time, failed bool) batchv1.Job {
	j := batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name:              name,
		CreationTimestamp: metav1.Time{Time: created},
		OwnerReferences:   []metav1.OwnerReference{{Kind: "CronJob", Name: owner}},
	}}
	cond := batchv1.JobComplete
	if failed {
		cond = batchv1.JobFailed
	}
	j.Status.Conditions = []batchv1.JobCondition{{Type: cond, Status: corev1.ConditionTrue}}
	return j
}

func TestLatestFailedJob(t *testing.T) {
	t0 := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	jobs := []batchv1.Job{
		job("cinc-sync-1", "cinc-sync", t0, true),
		job("cinc-sync-3", "cinc-sync", t0.Add(2*time.Hour), true),
		job("cinc-sync-2", "cinc-sync", t0.Add(time.Hour), false),
		job("other-9", "other", t0.Add(5*time.Hour), true),
	}
	got := LatestFailedJob(jobs, "cinc-sync")
	if got == nil || got.Name != "cinc-sync-3" {
		t.Fatalf("LatestFailedJob = %v, want cinc-sync-3 (newest failed, own CronJob only)", got)
	}
	if LatestFailedJob(jobs[2:3], "cinc-sync") != nil {
		t.Error("a completed run is not a failed one")
	}
}
