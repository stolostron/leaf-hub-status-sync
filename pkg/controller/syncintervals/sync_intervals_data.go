package syncintervals

import (
	"time"
)

const defaultSyncIntervalSeconds = 5

// ResolveSyncIntervalFunc is a function for resolving corresponding sync interval from SyncIntervals data structure.
type ResolveSyncIntervalFunc func() time.Duration

// SyncIntervals holds periodic sync intervals.
type SyncIntervals struct {
	managedClusters time.Duration
	policies        time.Duration
}

// NewSyncIntervals returns new HohConfigMapData object initialized with default periodic sync intervals.
func NewSyncIntervals() *SyncIntervals {
	return &SyncIntervals{
		managedClusters: defaultSyncIntervalSeconds * time.Second,
		policies:        defaultSyncIntervalSeconds * time.Second,
	}
}

// GetManagerClusters returns managed clusters sync interval.
func (syncIntervals *SyncIntervals) GetManagerClusters() time.Duration {
	return syncIntervals.managedClusters
}

// GetPolicies returns policies sync interval.
func (syncIntervals *SyncIntervals) GetPolicies() time.Duration {
	return syncIntervals.policies
}
