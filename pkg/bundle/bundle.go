package bundle

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type Object interface {
	metav1.Object
	runtime.Object
}

type Bundle interface {
	UpdateObject(object Object)
	DeleteObject(object Object)
	GetBundleGeneration() uint64
}
