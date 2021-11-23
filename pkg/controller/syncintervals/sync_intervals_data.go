package syncintervals

import (
	"time"
)

const (
	defaultStatusSyncInterval      = 5 * time.Second
	defaultControlInfoSyncInterval = 60 * time.Second
)

// ResolveSyncIntervalFunc is a function for resolving corresponding sync interval from SyncIntervals data structure.
type ResolveSyncIntervalFunc func() time.Duration

// SyncIntervals holds periodic sync intervals.
type SyncIntervals struct {
	managedClusters time.Duration
	policies        time.Duration
	controlInfo     time.Duration
}

// NewSyncIntervals returns new HohConfigMapData object initialized with default periodic sync intervals.
func NewSyncIntervals() *SyncIntervals {
	return &SyncIntervals{
		managedClusters: defaultStatusSyncInterval,
		policies:        defaultStatusSyncInterval,
		controlInfo:     defaultControlInfoSyncInterval,
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

// GetControlInfo returns control info sync interval.
func (syncIntervals *SyncIntervals) GetControlInfo() time.Duration {
	return syncIntervals.controlInfo
}
