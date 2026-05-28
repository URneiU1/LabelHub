package main

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"labelhub.local/llmreview"
)

var errAIWorkerCircuitOpen = errors.New("ai worker provider circuit open")

type aiWorkerCircuit struct {
	maxProvider5XX int
	openDuration   time.Duration
	now            func() time.Time

	mu                     sync.Mutex
	consecutiveProvider5XX int
	openedUntil            time.Time
}

func newAIWorkerCircuitFromEnv() *aiWorkerCircuit {
	return newAIWorkerCircuit(
		positiveIntEnv("AI_WORKER_CIRCUIT_MAX_5XX", 20),
		positiveDurationEnv("AI_WORKER_CIRCUIT_OPEN_MS", 300000),
	)
}

func newAIWorkerCircuit(maxProvider5XX int, openDuration time.Duration) *aiWorkerCircuit {
	if maxProvider5XX <= 0 || openDuration <= 0 {
		return nil
	}
	return &aiWorkerCircuit{
		maxProvider5XX: maxProvider5XX,
		openDuration:   openDuration,
		now:            time.Now,
	}
}

func (c *aiWorkerCircuit) check() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	if now.Before(c.openedUntil) {
		return fmt.Errorf("%w until %s", errAIWorkerCircuitOpen, c.openedUntil.Format(time.RFC3339))
	}
	if !c.openedUntil.IsZero() {
		c.openedUntil = time.Time{}
		c.consecutiveProvider5XX = 0
	}
	return nil
}

func (c *aiWorkerCircuit) recordProviderFailure(cause error) {
	if c == nil || !llmreview.IsProviderHTTP5xx(cause) {
		return
	}
	c.recordProvider5XX()
}

func (c *aiWorkerCircuit) recordProvider5XX() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.consecutiveProvider5XX++
	if c.consecutiveProvider5XX >= c.maxProvider5XX {
		c.openedUntil = c.now().Add(c.openDuration)
	}
}

func (c *aiWorkerCircuit) recordProviderSuccess() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.consecutiveProvider5XX = 0
	c.openedUntil = time.Time{}
}

func positiveIntEnv(key string, fallback int) int {
	value := envOrDefault(key, "")
	if value == "" {
		return fallback
	}
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func positiveDurationEnv(key string, fallbackMS int) time.Duration {
	value := envOrDefault(key, "")
	if value == "" {
		return time.Duration(fallbackMS) * time.Millisecond
	}
	var ms int
	if _, err := fmt.Sscanf(value, "%d", &ms); err != nil || ms <= 0 {
		return time.Duration(fallbackMS) * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}
