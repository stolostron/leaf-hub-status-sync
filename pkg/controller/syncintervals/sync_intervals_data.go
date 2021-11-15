package syncintervals

import (
	"time"
)

const defaultSyncInterval = 5 * time.Second

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
		managedClusters: defaultSyncInterval,
		policies:        defaultSyncInterval,
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
