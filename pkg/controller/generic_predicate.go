package controller

import (
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// a struct that implements controller-runtime Predicate interface
type GenericPredicate struct {
	predicate.Funcs
}

func (p *GenericPredicate) Create(e event.CreateEvent) bool {
	return p.ownerReferenceUidAnnotationExists(e.Meta)
}

func (p *GenericPredicate) Delete(e event.DeleteEvent) bool {
	return p.ownerReferenceUidAnnotationExists(e.Meta)
}

func (p *GenericPredicate) Update(e event.UpdateEvent) bool {
	return p.ownerReferenceUidAnnotationExists(e.MetaOld) || p.ownerReferenceUidAnnotationExists(e.MetaNew)
}

func (p *GenericPredicate) Generic(e event.GenericEvent) bool {
	return p.ownerReferenceUidAnnotationExists(e.Meta)
}

func (p *GenericPredicate) ownerReferenceUidAnnotationExists(obj metav1.Object) bool {
	if hasAnnotation(obj, datatypes.OriginOwnerReferenceAnnotation) {
		return true
	}
	return false
}

// hasAnnotation returns a bool if the given annotation exists in annotations
func hasAnnotation(obj metav1.Object, annotation string) bool {
	if obj == nil || obj.GetAnnotations() == nil {
		return false
	}
	_, found := obj.GetAnnotations()[annotation]
	return found
}
