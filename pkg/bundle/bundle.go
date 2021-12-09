package bundle

import (
	"errors"

	"github.com/open-cluster-management/hub-of-hubs-data-types/bundle/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var errObjectNotFound = errors.New("object not found")

// ExtractObjIDFunc a function type used to get the id of an object.
type ExtractObjIDFunc func(obj Object) (string, bool)

// Object is an interface for a single object inside a bundle.
type Object interface {
	metav1.Object
	runtime.Object
}

// Bundle is an abstraction for managing different bundle types.
type Bundle interface {
	// GetID function to get bundle ID (data-type), used to identify the bundle.
	GetID() string
	// UpdateObject function to update a single object inside a bundle.
	UpdateObject(object Object)
	// DeleteObject function to delete a single object inside a bundle.
	DeleteObject(object Object)
	// GetBundleVersion function to get bundle generation.
	GetBundleVersion() *status.BundleVersion
}

// DeltaStateBundle abstracts the logic needed from the delta-state bundle.
type DeltaStateBundle interface {
	Bundle
	// SyncState syncs the state of the delta-bundle with the full-state.
	SyncState()
	// Reset flushes the delta-state bundle's objects.
	Reset()
}
