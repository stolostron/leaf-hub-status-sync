package bundle

import (
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sync"
)

type Object interface {
	metav1.Object
	runtime.Object
}

func NewStatusBundle(leafHubId string) *StatusBundle {
	return &StatusBundle{
		Objects:    make([]Object, 0),
		LeafHubId:  leafHubId,
		generation: 0,
		lock:       sync.Mutex{},
	}
}

type StatusBundle struct {
	Objects    []Object `json:"objects"`
	LeafHubId  string   `json:"leafHubId"`
	generation uint64
	lock       sync.Mutex
}

func (bundle *StatusBundle) UpdateObject(object Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	index, err := bundle.getObjectIndexByUID(object.GetUID())
	if err != nil { // object not found, need to add it to the bundle
		bundle.Objects = append(bundle.Objects, object)
		bundle.generation++
		return
	}

	// if we reached here, object already exists in the bundle.. check if we need to update the object
	if object.GetResourceVersion() == bundle.Objects[index].GetResourceVersion() {
		return // update object only if something has changed. check for changes using resource version field
	}
	bundle.Objects[index] = object
	bundle.generation++
}

func (bundle *StatusBundle) DeleteObject(object Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	index, err := bundle.getObjectIndexByUID(object.GetUID())
	if err != nil { // trying to delete object which doesn't exist - return with no error
		return
	}
	bundle.Objects = append(bundle.Objects[:index], bundle.Objects[index+1:]...) // remove from objects
	bundle.generation++
}

func (bundle *StatusBundle) GetBundleGeneration() uint64 {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.generation
}
func (bundle *StatusBundle) getObjectIndexByUID(uid types.UID) (int, error) {
	for i, object := range bundle.Objects {
		if object.GetUID() == uid {
			return i, nil
		}
	}
	return -1, errors.New("object not found")
}
