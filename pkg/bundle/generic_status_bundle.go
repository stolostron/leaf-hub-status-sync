package bundle

import (
	"sync"

	statusbundle "github.com/open-cluster-management/hub-of-hubs-data-types/bundle/status"
	"k8s.io/apimachinery/pkg/types"
)

// NewGenericStatusBundle creates a new instance of GenericStatusBundle.
func NewGenericStatusBundle(leafHubName string, incarnation uint64, generation uint64) Bundle {
	return &GenericStatusBundle{
		Objects:       make([]Object, 0),
		LeafHubName:   leafHubName,
		BundleVersion: *statusbundle.NewBundleVersion(incarnation, generation),
		lock:          sync.Mutex{},
	}
}

// GenericStatusBundle is a bundle that is used to send to the hub of hubs the leaf CR as is
// except for fields that are not relevant in the hub of hubs like finalizers, etc.
// for bundles that require more specific behavior, it's required to implement your own status bundle struct.
type GenericStatusBundle struct {
	Objects     []Object `json:"objects"`
	LeafHubName string   `json:"leafHubName"`
	statusbundle.BundleVersion
	lock sync.Mutex
}

// UpdateObject function to update a single object inside a bundle.
func (bundle *GenericStatusBundle) UpdateObject(object Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	index, err := bundle.getObjectIndexByUID(object.GetUID())
	if err != nil { // object not found, need to add it to the bundle
		bundle.Objects = append(bundle.Objects, object)
		bundle.Generation++

		return
	}

	// if we reached here, object already exists in the bundle.. check if we need to update the object
	if object.GetResourceVersion() == bundle.Objects[index].GetResourceVersion() {
		return // update in bundle only if object changed. check for changes using resourceVersion field
	}

	bundle.Objects[index] = object
	bundle.Generation++
}

// DeleteObject function to delete a single object inside a bundle.
func (bundle *GenericStatusBundle) DeleteObject(object Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	index, err := bundle.getObjectIndexByUID(object.GetUID())
	if err != nil { // trying to delete object which doesn't exist - return with no error
		return
	}

	bundle.Objects = append(bundle.Objects[:index], bundle.Objects[index+1:]...) // remove from objects

	bundle.Generation++
}

// GetBundleGeneration function to get bundle's generation.
func (bundle *GenericStatusBundle) GetBundleGeneration() uint64 {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.Generation
}

// GetObjectsCount function to get bundle objects count.
func (bundle *GenericStatusBundle) GetObjectsCount() int {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return len(bundle.Objects)
}

func (bundle *GenericStatusBundle) getObjectIndexByUID(uid types.UID) (int, error) {
	for i, object := range bundle.Objects {
		if object.GetUID() == uid {
			return i, nil
		}
	}

	return -1, errObjectNotFound
}
