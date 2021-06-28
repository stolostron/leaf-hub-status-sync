package predicate

import (
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// a struct that implements controller-runtime Predicate interface
type NotHohNamespacePredicate struct {
	predicate.Funcs
}

func (p *NotHohNamespacePredicate) Create(e event.CreateEvent) bool {
	return e.Meta.GetNamespace() != datatypes.HohSystemNamespace
}

func (p *NotHohNamespacePredicate) Delete(e event.DeleteEvent) bool {
	return e.Meta.GetNamespace() != datatypes.HohSystemNamespace
}

func (p *NotHohNamespacePredicate) Update(e event.UpdateEvent) bool {
	return e.MetaOld.GetNamespace() != datatypes.HohSystemNamespace &&
		e.MetaNew.GetNamespace() != datatypes.HohSystemNamespace
}

func (p *NotHohNamespacePredicate) Generic(e event.GenericEvent) bool {
	return e.Meta.GetNamespace() != datatypes.HohSystemNamespace
}
