package worker

import (
	"context"
	"time"
)

// Job is a function that should be run by the worker group. The context provided
// allows the Job to cancel if the worker group is closed. All other life-cycle
// management should be wrapped within the Job.
type Job interface {
	String() string
	Run(context.Context) error
}

type retryableJob struct {
	name  string
	count uint8
	when  time.Time
	job   Job
}

func (j retryableJob) String() string {
	return j.job.String()
}

func (j retryableJob) Run(ctx context.Context) error {
	return j.job.Run(ctx)
}
