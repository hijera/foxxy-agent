//go:build scheduler

package schedservice

// SchedulerInfo is the envelope object returned with GET /foxxycode/scheduler/jobs.
type SchedulerInfo struct {
	Enabled        bool   `json:"enabled"`
	Dir            string `json:"dir"`
	Timeout        string `json:"timeout"`
	MaxQueue       int    `json:"max_queue"`
	RunsActive     int    `json:"runs_active"`
	RetainSessions int    `json:"retain_sessions"`
}

// SchedulerJob is the wire shape for one task.
type SchedulerJob struct {
	JobID                string `json:"job_id"`
	Description          string `json:"description,omitempty"`
	Schedule             string `json:"schedule"`
	Paused               bool   `json:"paused"`
	CWD                  string `json:"cwd,omitempty"`
	Model                string `json:"model,omitempty"`
	Mode                 string `json:"mode,omitempty"`
	Body                 string `json:"body,omitempty"`
	LastScheduledSlotUTC string `json:"last_scheduled_slot_utc,omitempty"`
	NextRunUTC           string `json:"next_run_utc,omitempty"`
	Running              bool   `json:"running"`
}

// JobsListResponse is GET /foxxycode/scheduler/jobs.
type JobsListResponse struct {
	Scheduler SchedulerInfo  `json:"scheduler"`
	Jobs      []SchedulerJob `json:"jobs"`
}

// SchedulerJobCreate is POST /foxxycode/scheduler/jobs.
type SchedulerJobCreate struct {
	JobID       string `json:"job_id"`
	Description string `json:"description"`
	Schedule    string `json:"schedule"`
	Paused      bool   `json:"paused"`
	CWD         string `json:"cwd,omitempty"`
	Model       string `json:"model,omitempty"`
	Mode        string `json:"mode,omitempty"`
	Body        string `json:"body"`
}

// SchedulerJobPatch is PATCH /foxxycode/scheduler/jobs/{job_id}.
// JobID, when set to a value different from the path job_id, renames the job file and sidecars.
type SchedulerJobPatch struct {
	JobID       *string `json:"job_id"`
	Description *string `json:"description"`
	Schedule    *string `json:"schedule"`
	Paused      *bool   `json:"paused"`
	CWD         *string `json:"cwd"`
	Model       *string `json:"model"`
	Mode        *string `json:"mode"`
	Body        *string `json:"body"`
}

// SchedulerRunEntry is one row of GET /foxxycode/scheduler/jobs/{job_id}/runs.
type SchedulerRunEntry struct {
	SessionID string `json:"session_id"`
	StartedAt string `json:"started_at,omitempty"`
	EndedAt   string `json:"ended_at,omitempty"`
	Status    string `json:"status,omitempty"`
}
