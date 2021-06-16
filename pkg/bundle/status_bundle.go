package bundle

import (
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sync"
	"time"
)

type Object interface {
	metav1.Object
	runtime.Object
}

type TimestampedObject struct {
	object              Object
	lastUpdateTimestamp *time.Time
}

func NewStatusBundle() *StatusBundle {
	return &StatusBundle{
		Objects:             make([]*TimestampedObject, 0),
		DeletedObjectsUIDs:  make([]types.UID, 0),
		lastUpdateTimestamp: &time.Time{},
		lock:                sync.Mutex{},
	}
}

type StatusBundle struct {
	Objects             []*TimestampedObject `json:"objects"`
	DeletedObjectsUIDs  []types.UID          `json:"deletedObjectsUids"`
	lastUpdateTimestamp *time.Time
	lock                sync.Mutex
}

func (bundle *StatusBundle) UpdateObject(object Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()
	lastObjUpdateTimestamp := getLastUpdateTimestamp(object)
	index, err := bundle.getObjectIndexByUID(object.GetUID())
	if err != nil { // object not found, need to add it to the bundle
		bundle.Objects = append(bundle.Objects, &TimestampedObject{object, lastObjUpdateTimestamp})
		bundle.updateBundleTimestamp(lastObjUpdateTimestamp)
		return
	}

	// if we reached here, object uid already exists in the bundle
	if !lastObjUpdateTimestamp.After(*(bundle.Objects[index].lastUpdateTimestamp)) {
		return // update object only if something has changed. check for changes using timestamps
	}
	bundle.Objects[index].object = object
	bundle.Objects[index].lastUpdateTimestamp = lastObjUpdateTimestamp
	bundle.updateBundleTimestamp(lastObjUpdateTimestamp)
}

func (bundle *StatusBundle) DeleteObject(object Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()
	index, err := bundle.getObjectIndexByUID(object.GetUID())
	if err != nil { // trying to delete object which doesn't exist - return with no error
		return
	}
	lastObjUpdateTimestamp := getLastUpdateTimestamp(object)
	bundle.DeletedObjectsUIDs = append(bundle.DeletedObjectsUIDs, bundle.Objects[index].object.GetUID())
	bundle.Objects = append(bundle.Objects[:index], bundle.Objects[index+1:]...) // remove from objects
	bundle.updateBundleTimestamp(lastObjUpdateTimestamp)
}

func (bundle *StatusBundle) GetBundleTimestamp() *time.Time {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.lastUpdateTimestamp
}

// in k8s, events can get out of order (for example if reconciliation failed and was re-queued)
// therefore, it's possible to update a bundle with object A with timestamp A and then object B with timestamp B,
// such that, timestamp A is after timestamp B. so the last update doesn't have the last update timestamp.
// because of that, we cannot use the max timestamp inside the bundle as an indication if bundle has changed or not.

// in order to overcome this issue, we take the following approach:
// if the new update is after the bundle max timestamp - update the bundle last update timestamp
// otherwise, we increase the bundle timestamp with one microsecond, just to mark that the bundle has changed.

// in a different thread we will check if the bundle has changed based on the timestamp comparison.
func (bundle *StatusBundle) updateBundleTimestamp(timestamp *time.Time) {
	if timestamp.After(*(bundle.lastUpdateTimestamp)) {
		bundle.lastUpdateTimestamp = timestamp
	} else {
		newTimestamp := bundle.lastUpdateTimestamp.Add(time.Microsecond)
		bundle.lastUpdateTimestamp = &newTimestamp
	}
}

func (bundle *StatusBundle) getObjectIndexByUID(uid types.UID) (int, error) {
	for i, timestampedObject := range bundle.Objects {
		if timestampedObject.object.GetUID() == uid {
			return i, nil
		}
	}
	return -1, errors.New("object not found")
}

func getLastUpdateTimestamp(object Object) *time.Time {
	lastUpdateTimestamp := object.GetCreationTimestamp().Time // init last update of an object with creation timestamp
	// all changes of a CR are specified in the managed fields timestamps, each change is a different entry
	for _, managedField := range object.GetManagedFields() {
		if managedField.Time.After(lastUpdateTimestamp) {
			lastUpdateTimestamp = managedField.Time.Time
		}
	}
	// compare with deletion timestamp as well
	if object.GetDeletionTimestamp().After(lastUpdateTimestamp) {
		lastUpdateTimestamp = object.GetDeletionTimestamp().Time
	}
	return &lastUpdateTimestamp
}
