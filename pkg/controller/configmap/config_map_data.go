package configmap

import (
	"time"
)

const defaultPeriodicSyncInterval = "5s"

// PeriodicSyncIntervals holds periodic sync intervals.
type PeriodicSyncIntervals struct {
	ManagedClusters time.Duration
	Policies        time.Duration
}

// HohConfigMapData holds config map data.
type HohConfigMapData struct {
	Intervals PeriodicSyncIntervals
}

// NewHohConfigMapData returns new HohConfigMapData object initialized with default periodic sync intervals.
func NewHohConfigMapData() *HohConfigMapData {
	hohConfigMapData := &HohConfigMapData{}

	hohConfigMapData.Intervals.ManagedClusters, _ = time.ParseDuration(defaultPeriodicSyncInterval)
	hohConfigMapData.Intervals.Policies, _ = time.ParseDuration(defaultPeriodicSyncInterval)

	return hohConfigMapData
}

// PeriodicSyncIntervalResolver is a function for resolving corresponding periodic sync interval from HohConfigMapData.
type PeriodicSyncIntervalResolver func(*HohConfigMapData) time.Duration
