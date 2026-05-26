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
	window            time.Duration
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
