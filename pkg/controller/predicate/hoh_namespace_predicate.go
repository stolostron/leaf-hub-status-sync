package predicate

import (
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// a struct that implements controller-runtime Predicate interface
type HohNamespacePredicate struct {
	predicate.Funcs
}

func (p *HohNamespacePredicate) Create(e event.CreateEvent) bool {
	return e.Meta.GetNamespace() == datatypes.HohSystemNamespace
}

func (p *HohNamespacePredicate) Delete(e event.DeleteEvent) bool {
	return e.Meta.GetNamespace() == datatypes.HohSystemNamespace
}

func (p *HohNamespacePredicate) Update(e event.UpdateEvent) bool {
	return e.MetaOld.GetNamespace() == datatypes.HohSystemNamespace ||
		e.MetaNew.GetNamespace() == datatypes.HohSystemNamespace
}

func (p *HohNamespacePredicate) Generic(e event.GenericEvent) bool {
	return e.Meta.GetNamespace() == datatypes.HohSystemNamespace
}
