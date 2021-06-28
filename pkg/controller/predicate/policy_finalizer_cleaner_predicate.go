package predicate

import (
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// a struct that implements controller-runtime Predicate interface
type PolicyFinalizerCleanerPredicate struct {
	predicate.Funcs
	FinalizerName string
}

func (p *PolicyFinalizerCleanerPredicate) Create(e event.CreateEvent) bool {
	return e.Meta.GetNamespace() != datatypes.HohSystemNamespace && hasFinalizer(e.Meta, p.FinalizerName)
}

func (p *PolicyFinalizerCleanerPredicate) Delete(e event.DeleteEvent) bool {
	return e.Meta.GetNamespace() != datatypes.HohSystemNamespace && hasFinalizer(e.Meta, p.FinalizerName)
}

func (p *PolicyFinalizerCleanerPredicate) Update(e event.UpdateEvent) bool {
	return e.MetaOld.GetNamespace() != datatypes.HohSystemNamespace &&
		e.MetaNew.GetNamespace() != datatypes.HohSystemNamespace &&
		(hasFinalizer(e.MetaOld, p.FinalizerName) || hasFinalizer(e.MetaNew, p.FinalizerName))
}

func (p *PolicyFinalizerCleanerPredicate) Generic(e event.GenericEvent) bool {
	return e.Meta.GetNamespace() != datatypes.HohSystemNamespace && hasFinalizer(e.Meta, p.FinalizerName)
}

func (p *PolicyFinalizerCleanerPredicate) hasFinalizer(obj metav1.Object) bool {
	if hasFinalizer(obj, p.FinalizerName) {
		return true
	}
	return false
}
