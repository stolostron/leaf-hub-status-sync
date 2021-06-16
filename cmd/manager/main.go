// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	lhSyncService "github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport/sync-service"
	"github.com/open-cluster-management/leaf-hub-status-sync/version"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"github.com/operator-framework/operator-sdk/pkg/leader"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"github.com/spf13/pflag"
	"os"
	"runtime"
	"strings"
	"time"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

const (
	metricsHost                = "0.0.0.0"
	metricsPort          int32 = 8527
	periodicSyncInterval       = "PERIODIC_SYNC_INTERVAL"
)

func printVersion(log logr.Logger) {
	log.Info(fmt.Sprintf("Operator Version: %s", version.Version))
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	log.Info(fmt.Sprintf("Version of operator-sdk: %v", sdkVersion.Version))
}

// function to handle defers with exit, see https://stackoverflow.com/a/27629493/553720.
func doMain() int {
	pflag.CommandLine.AddFlagSet(zap.FlagSet())
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	ctrl.SetLogger(zap.Logger())
	log := ctrl.Log.WithName("cmd")

	printVersion(log)

	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		log.Error(err, "Failed to get watch namespace")
		return 1
	}

	syncIntervalStr := os.Getenv(periodicSyncInterval)
	if syncIntervalStr == "" {
		log.Error(errors.New("failed to initialize, missing environment variable"), periodicSyncInterval)
		return 1
	}
	interval, err := time.ParseDuration(syncIntervalStr)
	if err != nil {
		log.Error(err, fmt.Sprintf("the expected var %s is not valid duration", periodicSyncInterval))
		return 1
	}

	// transport layer initialization
	syncServiceObj := lhSyncService.NewSyncService()
	syncServiceObj.Start()
	defer syncServiceObj.Stop()

	ctx := context.TODO()
	// Become the leader before proceeding
	err = leader.Become(ctx, "leaf-hub-status-sync-lock")
	if err != nil {
		log.Error(err, "Failed to become leader")
		return 1
	}

	mgr, err := createManager(namespace, metricsHost, metricsPort, syncServiceObj, interval)
	if err != nil {
		log.Error(err, "Failed to create manager")
		return 1
	}

	log.Info("Starting the Cmd.")

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "Manager exited non-zero")
		return 1
	}

	return 0
}

func createManager(namespace, metricsHost string, metricsPort int32, transport transport.Transport,
	syncInterval time.Duration) (ctrl.Manager, error) {
	options := ctrl.Options{
		Namespace:          namespace,
		MetricsBindAddress: fmt.Sprintf("%s:%d", metricsHost, metricsPort),
	}

	// Add support for MultiNamespace set in WATCH_NAMESPACE (e.g ns1,ns2)
	// Note that this is not intended to be used for excluding namespaces, this is better done via a Predicate
	// Also note that you may face performance issues when using this with a high number of namespaces.
	// More Info: https://godoc.org/github.com/kubernetes-sigs/controller-runtime/pkg/cache#MultiNamespacedCacheBuilder
	if strings.Contains(namespace, ",") {
		options.Namespace = ""
		options.NewCache = cache.MultiNamespacedCacheBuilder(strings.Split(namespace, ","))
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new manager: %w", err)
	}

	if err := controller.AddToScheme(mgr.GetScheme()); err != nil {
		return nil, fmt.Errorf("failed to add schemes: %w", err)
	}

	if err := controller.AddControllers(mgr, transport, syncInterval); err != nil {
		return nil, fmt.Errorf("failed to add controllers: %w", err)
	}

	return mgr, nil
}

func main() {
	os.Exit(doMain())
}
