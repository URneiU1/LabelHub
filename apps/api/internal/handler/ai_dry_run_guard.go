package handler

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"labelhub-api/internal/model"
)

const dryRunQuotaMaxRunsEnv = "LLM_DRY_RUN_QUOTA_MAX_RUNS"
const dryRunCircuitMaxFailuresEnv = "LLM_DRY_RUN_CIRCUIT_MAX_FAILURES"
const dryRunGuardWindowMinutesEnv = "LLM_DRY_RUN_GUARD_WINDOW_MINUTES"

var errDryRunQuotaExceeded = errors.New("dry-run quota exceeded")
var errDryRunCircuitOpen = errors.New("dry-run circuit breaker is open")

type dryRunGuardConfig struct {
	quotaMaxRuns      int
	circuitMaxFailure int
	windowMinutes     int
	window            time.Duration
}

type dryRunGuardStatus struct {
	WindowMinutes      int    `json:"windowMinutes"`
	QuotaMaxRuns       int    `json:"quotaMaxRuns"`
	CircuitMaxFailures int    `json:"circuitMaxFailures"`
	RecentRuns         int64  `json:"recentRuns"`
	RecentFailures     int64  `json:"recentFailures"`
	QuotaRemaining     *int   `json:"quotaRemaining"`
	CircuitOpen        bool   `json:"circuitOpen"`
	State              string `json:"state"`
}

func enforceDryRunGuard(db *gorm.DB, taskID uint64, requestedRuns int) error {
	config, err := dryRunGuardConfigFromEnv()
	if err != nil {
		return err
	}
	if config.quotaMaxRuns == 0 && config.circuitMaxFailure == 0 {
		return nil
	}
	if requestedRuns <= 0 {
		requestedRuns = 1
	}
	since := time.Now().UTC().Add(-config.window)
	if config.quotaMaxRuns > 0 {
		var count int64
		if err := db.Model(&model.AIDryRun{}).Where("task_id = ? AND created_at >= ?", taskID, since).Count(&count).Error; err != nil {
			return err
		}
		if count+int64(requestedRuns) > int64(config.quotaMaxRuns) {
			return errDryRunQuotaExceeded
		}
	}
	if config.circuitMaxFailure > 0 {
		var failures int64
		if err := db.Model(&model.AIDryRun{}).Where("task_id = ? AND status = ? AND created_at >= ?", taskID, "failed", since).Count(&failures).Error; err != nil {
			return err
		}
		if failures >= int64(config.circuitMaxFailure) {
			return errDryRunCircuitOpen
		}
	}
	return nil
}

func dryRunGuardStatusForTask(db *gorm.DB, taskID uint64) (dryRunGuardStatus, error) {
	config, err := dryRunGuardConfigFromEnv()
	if err != nil {
		return dryRunGuardStatus{}, err
	}
	status := dryRunGuardStatus{
		WindowMinutes:      config.windowMinutes,
		QuotaMaxRuns:       config.quotaMaxRuns,
		CircuitMaxFailures: config.circuitMaxFailure,
		State:              "disabled",
	}
	if config.quotaMaxRuns == 0 && config.circuitMaxFailure == 0 {
		return status, nil
	}
	status.State = "ok"
	since := time.Now().UTC().Add(-config.window)
	if config.quotaMaxRuns > 0 {
		var count int64
		if err := db.Model(&model.AIDryRun{}).Where("task_id = ? AND created_at >= ?", taskID, since).Count(&count).Error; err != nil {
			return dryRunGuardStatus{}, err
		}
		status.RecentRuns = count
		remaining := config.quotaMaxRuns - int(count)
		if remaining < 0 {
			remaining = 0
		}
		status.QuotaRemaining = &remaining
		if remaining == 0 {
			status.State = "quota_exhausted"
		}
	}
	if config.circuitMaxFailure > 0 {
		var failures int64
		if err := db.Model(&model.AIDryRun{}).Where("task_id = ? AND status = ? AND created_at >= ?", taskID, "failed", since).Count(&failures).Error; err != nil {
			return dryRunGuardStatus{}, err
		}
		status.RecentFailures = failures
		status.CircuitOpen = failures >= int64(config.circuitMaxFailure)
		if status.CircuitOpen {
			status.State = "circuit_open"
		}
	}
	return status, nil
}

func dryRunGuardConfigFromEnv() (dryRunGuardConfig, error) {
	quotaMaxRuns, err := optionalNonNegativeIntEnv(dryRunQuotaMaxRunsEnv)
	if err != nil {
		return dryRunGuardConfig{}, err
	}
	circuitMaxFailure, err := optionalNonNegativeIntEnv(dryRunCircuitMaxFailuresEnv)
	if err != nil {
		return dryRunGuardConfig{}, err
	}
	windowMinutes, err := optionalNonNegativeIntEnv(dryRunGuardWindowMinutesEnv)
	if err != nil {
		return dryRunGuardConfig{}, err
	}
	if windowMinutes == 0 {
		windowMinutes = 60
	}
	return dryRunGuardConfig{
		quotaMaxRuns:      quotaMaxRuns,
		circuitMaxFailure: circuitMaxFailure,
		windowMinutes:     windowMinutes,
		window:            time.Duration(windowMinutes) * time.Minute,
	}, nil
}

func optionalNonNegativeIntEnv(name string) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, errors.New(name + " must be a non-negative integer")
	}
	return value, nil
}
