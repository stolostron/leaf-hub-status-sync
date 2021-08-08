package predicate

import (
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// GenericPredicate is a predicate to check if event is for object that contains hub of hubs owner reference annotation.
var GenericPredicate = predicate.NewPredicateFuncs(func(meta v1.Object, object runtime.Object) bool {
	return hasAnnotation(meta, datatypes.OriginOwnerReferenceAnnotation)
})
