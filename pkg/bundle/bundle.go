package bundle

import (
	"errors"

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
	// UpdateObject function to update a single object inside a bundle.
	UpdateObject(object Object)
	// DeleteObject function to delete a single object inside a bundle.
	DeleteObject(object Object)
	// GetBundleGeneration function to get bundle generation.
	GetBundleGeneration() uint64
}
