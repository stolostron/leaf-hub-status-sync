package predicate

import (
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// GenericPredicate is a predicate to check if event is for object that contains hub of hubs owner reference annotation.
type GenericPredicate struct {
	predicate.Funcs
}

// Create is a function to filter k8s Create events.
func (p *GenericPredicate) Create(e event.CreateEvent) bool {
	return hasAnnotation(e.Meta, datatypes.OriginOwnerReferenceAnnotation)
}

// Delete is a function to filter k8s Delete events.
func (p *GenericPredicate) Delete(e event.DeleteEvent) bool {
	return hasAnnotation(e.Meta, datatypes.OriginOwnerReferenceAnnotation)
}

// Update is a function to filter k8s Update events.
func (p *GenericPredicate) Update(e event.UpdateEvent) bool {
	return hasAnnotation(e.MetaOld, datatypes.OriginOwnerReferenceAnnotation) ||
		hasAnnotation(e.MetaNew, datatypes.OriginOwnerReferenceAnnotation)
}

// Generic is a function to filter k8s Generic events.
func (p *GenericPredicate) Generic(e event.GenericEvent) bool {
	return hasAnnotation(e.Meta, datatypes.OriginOwnerReferenceAnnotation)
}
