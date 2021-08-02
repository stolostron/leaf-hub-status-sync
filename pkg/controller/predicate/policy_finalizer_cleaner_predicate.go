package predicate

import (
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// PolicyFinalizerCleanerPredicate a struct that implements controller-runtime Predicate interface
type PolicyFinalizerCleanerPredicate struct {
	predicate.Funcs
	FinalizerName string
}

// Create is a function to filter k8s Create events.
func (p *PolicyFinalizerCleanerPredicate) Create(e event.CreateEvent) bool {
	return e.Meta.GetNamespace() != datatypes.HohSystemNamespace && hasFinalizer(e.Meta, p.FinalizerName)
}

// Delete is a function to filter k8s Delete events.
func (p *PolicyFinalizerCleanerPredicate) Delete(e event.DeleteEvent) bool {
	return e.Meta.GetNamespace() != datatypes.HohSystemNamespace && hasFinalizer(e.Meta, p.FinalizerName)
}

// Update is a function to filter k8s Update events.
func (p *PolicyFinalizerCleanerPredicate) Update(e event.UpdateEvent) bool {
	return e.MetaOld.GetNamespace() != datatypes.HohSystemNamespace &&
		e.MetaNew.GetNamespace() != datatypes.HohSystemNamespace &&
		(hasFinalizer(e.MetaOld, p.FinalizerName) || hasFinalizer(e.MetaNew, p.FinalizerName))
}

// Generic is a function to filter k8s Generic events.
func (p *PolicyFinalizerCleanerPredicate) Generic(e event.GenericEvent) bool {
	return e.Meta.GetNamespace() != datatypes.HohSystemNamespace && hasFinalizer(e.Meta, p.FinalizerName)
}
