//go:build scheduler

package scheduler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LockPath returns the lock file path for a job .md path.
func LockPath(jobMDPath string) string {
	base := strings.TrimSuffix(filepath.Base(jobMDPath), ".md")
	return filepath.Join(filepath.Dir(jobMDPath), base+".lock")
}

// StatePath returns the state file path for a job .md path.
func StatePath(jobMDPath string) string {
	base := strings.TrimSuffix(filepath.Base(jobMDPath), ".md")
	return filepath.Join(filepath.Dir(jobMDPath), base+".state")
}

type jobDiskState struct {
	LastScheduledUTC string `json:"last_scheduled_utc"`
}

// ReadJobState reads last scheduled fire time from a .state file.
func ReadJobState(path string) (time.Time, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	var st jobDiskState
	if err := json.Unmarshal(data, &st); err != nil {
		return time.Time{}, err
	}
	if strings.TrimSpace(st.LastScheduledUTC) == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(st.LastScheduledUTC))
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

// WriteJobState persists the last executed cron slot (UTC).
func WriteJobState(path string, lastScheduled time.Time) error {
	st := jobDiskState{LastScheduledUTC: lastScheduled.UTC().Format(time.RFC3339)}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
