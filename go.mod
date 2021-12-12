module github.com/open-cluster-management/leaf-hub-status-sync

go 1.16

require (
	github.com/deckarep/golang-set v1.7.1
	github.com/go-logr/logr v0.3.0
	github.com/open-cluster-management/api v0.0.0-20210527013639-a6845f2ebcb1
	github.com/open-cluster-management/governance-policy-propagator v0.0.0-20210520203318-a78632de1e26
	github.com/open-cluster-management/hub-of-hubs-data-types v0.2.3-0.20211210144238-c4dfc562678e
	github.com/open-cluster-management/hub-of-hubs-data-types/apis/config v0.2.2
	github.com/open-cluster-management/hub-of-hubs-message-compression v0.0.0-20211122122545-c99e2d497b43
	github.com/open-cluster-management/multicloud-operators-subscription v1.2.2-2-20201130-59f96
	github.com/open-horizon/edge-sync-service-client v0.0.0-20190711093406-dc3a19905da2
	github.com/open-horizon/edge-utilities v0.0.0-20190711093331-0908b45a7152 // indirect
	github.com/operator-framework/operator-sdk v0.19.4
	github.com/spf13/pflag v1.0.5
	k8s.io/api v0.20.5
	k8s.io/apimachinery v0.20.5
	k8s.io/client-go v12.0.0+incompatible
	sigs.k8s.io/controller-runtime v0.6.3
)

replace k8s.io/client-go => k8s.io/client-go v0.20.5
