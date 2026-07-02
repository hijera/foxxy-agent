//go:build scheduler

package schedservice

import (
	"fmt"
	"os"
	"strings"

	"github.com/hijera/foxxy-agent/external/scheduler/storage"
)

// CreateJob writes a new *.md job file.
func (o *Service) CreateJob(in SchedulerJobCreate) error {
	if err := o.requireEnabled(); err != nil {
		return err
	}
	if err := ValidateJobID(in.JobID); err != nil {
		return err
	}
	abs, err := o.jobAbsPath(in.JobID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err == nil {
		return ErrJobExists
	} else if !os.IsNotExist(err) {
		return err
	}
	fm := &storage.JobFrontmatter{
		Description: strings.TrimSpace(in.Description),
		Schedule:    strings.TrimSpace(in.Schedule),
		Paused:      in.Paused,
		CWD:         strings.TrimSpace(in.CWD),
		Model:       strings.TrimSpace(in.Model),
		Mode:        strings.TrimSpace(in.Mode),
	}
	if _, err := storage.ParseCronUTC(fm.Schedule); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidJobID, err)
	}
	data, err := storage.FormatJobMarkdown(fm, in.Body)
	if err != nil {
		return err
	}
	if _, err := storage.ParseJobFromBytes(data); err != nil {
		return err
	}
	return os.WriteFile(abs, data, 0o644)
}

// ReplaceJob overwrites an existing job file.
func (o *Service) ReplaceJob(jobID string, in SchedulerJobCreate) error {
	if err := o.requireEnabled(); err != nil {
		return err
	}
	if strings.TrimSpace(in.JobID) != "" && strings.TrimSpace(in.JobID) != jobID {
		return ErrInvalidJobID
	}
	abs, err := o.jobAbsPath(jobID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err != nil {
		if os.IsNotExist(err) {
			return ErrJobNotFound
		}
		return err
	}
	fm := &storage.JobFrontmatter{
		Description: strings.TrimSpace(in.Description),
		Schedule:    strings.TrimSpace(in.Schedule),
		Paused:      in.Paused,
		CWD:         strings.TrimSpace(in.CWD),
		Model:       strings.TrimSpace(in.Model),
		Mode:        strings.TrimSpace(in.Mode),
	}
	if _, err := storage.ParseCronUTC(fm.Schedule); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidJobID, err)
	}
	data, err := storage.FormatJobMarkdown(fm, in.Body)
	if err != nil {
		return err
	}
	if _, err := storage.ParseJobFromBytes(data); err != nil {
		return err
	}
	return os.WriteFile(abs, data, 0o644)
}

// renameJobFiles moves basename.md plus .state and .lock sidecars when idle.
func (o *Service) renameJobFiles(oldID, newID string) error {
	oldID = strings.TrimSpace(oldID)
	newID = strings.TrimSpace(newID)
	if oldID == "" || newID == "" || oldID == newID {
		return nil
	}
	if err := ValidateJobID(newID); err != nil {
		return err
	}
	oldAbs, err := o.jobAbsPath(oldID)
	if err != nil {
		return err
	}
	newAbs, err := o.jobAbsPath(newID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(oldAbs); err != nil {
		if os.IsNotExist(err) {
			return ErrJobNotFound
		}
		return err
	}
	if _, err := os.Stat(newAbs); err == nil {
		return ErrJobExists
	} else if !os.IsNotExist(err) {
		return err
	}
	if lockOrTracked(oldAbs) {
		return ErrJobBusy
	}
	if err := os.Rename(oldAbs, newAbs); err != nil {
		return err
	}
	for _, pathFn := range []func(string) string{storage.StatePath, storage.LockPath} {
		oldSide := pathFn(oldAbs)
		if _, err := os.Stat(oldSide); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := os.Rename(oldSide, pathFn(newAbs)); err != nil {
			return err
		}
	}
	return nil
}

// PatchJob merges fields into an existing job file.
func (o *Service) PatchJob(jobID string, p SchedulerJobPatch) error {
	if err := o.requireEnabled(); err != nil {
		return err
	}
	targetID := strings.TrimSpace(jobID)
	if p.JobID != nil {
		newID := strings.TrimSpace(*p.JobID)
		if newID != "" && newID != targetID {
			if err := o.renameJobFiles(targetID, newID); err != nil {
				return err
			}
			targetID = newID
		}
	}
	abs, err := o.jobAbsPath(targetID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err != nil {
		if os.IsNotExist(err) {
			return ErrJobNotFound
		}
		return err
	}
	fm, body, err := storage.ParseJobFile(abs)
	if err != nil {
		return err
	}
	if p.Description != nil {
		fm.Description = strings.TrimSpace(*p.Description)
	}
	if p.Schedule != nil {
		fm.Schedule = strings.TrimSpace(*p.Schedule)
	}
	if p.Paused != nil {
		fm.Paused = *p.Paused
	}
	if p.CWD != nil {
		fm.CWD = strings.TrimSpace(*p.CWD)
	}
	if p.Model != nil {
		fm.Model = strings.TrimSpace(*p.Model)
	}
	if p.Mode != nil {
		fm.Mode = strings.TrimSpace(*p.Mode)
	}
	if p.Body != nil {
		body = strings.TrimRight(*p.Body, "\n")
	}
	if _, err := storage.ParseCronUTC(fm.Schedule); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidJobID, err)
	}
	data, err := storage.FormatJobMarkdown(fm, body)
	if err != nil {
		return err
	}
	if _, err := storage.ParseJobFromBytes(data); err != nil {
		return err
	}
	return os.WriteFile(abs, data, 0o644)
}

// DeleteJob removes job markdown and sidecars when idle.
func (o *Service) DeleteJob(jobID string) error {
	if err := o.requireEnabled(); err != nil {
		return err
	}
	abs, err := o.jobAbsPath(jobID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err != nil {
		if os.IsNotExist(err) {
			return ErrJobNotFound
		}
		return err
	}
	if lockOrTracked(abs) {
		return ErrJobBusy
	}
	_ = os.Remove(storage.LockPath(abs))
	_ = os.Remove(storage.StatePath(abs))
	if err := os.Remove(abs); err != nil {
		return err
	}
	return nil
}

// PauseJob sets paused:true in frontmatter without starting a run.
func (o *Service) PauseJob(jobID string) error {
	v := true
	return o.PatchJob(jobID, SchedulerJobPatch{Paused: &v})
}

// ResumeJob sets paused:false in frontmatter.
func (o *Service) ResumeJob(jobID string) error {
	v := false
	return o.PatchJob(jobID, SchedulerJobPatch{Paused: &v})
}
