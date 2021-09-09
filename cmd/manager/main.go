// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/go-logr/logr"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	lhSyncService "github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport/sync-service"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"github.com/spf13/pflag"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	metricsHost                     = "0.0.0.0"
	metricsPort               int32 = 8527
	envVarSyncInterval              = "PERIODIC_SYNC_INTERVAL"
	envVarLeafHubName               = "LH_ID"
	envVarControllerNamespace       = "POD_NAMESPACE"
	leaderElectionLockName          = "leaf-hub-status-sync-lock"
)

var (
	errEnvVarNotFound  = errors.New("not found environment variable")
	errEnvVarWrongType = errors.New("wrong type of environment variable")
)

func printVersion(log logr.Logger) {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	log.Info(fmt.Sprintf("Version of operator-sdk: %s", sdkVersion.Version))
}

func doMain() int {
	// initialize log
	log := initializeLog()

	// read environment variables
	leaderElectionNamespace, syncInterval, leafHubName, err := readEnvVars()
	if err != nil {
		log.Error(err, "failed to read environment variables- %w", err)
		return 1
	}

	// transport layer initialization
	syncService, err := lhSyncService.NewSyncService(ctrl.Log.WithName("sync-service"))
	if err != nil {
		log.Error(err, "failed to initialize sync service")
		return 1
	}

	syncService.Start()
	defer syncService.Stop()

	mgr, err := createManager(leaderElectionNamespace, metricsHost, metricsPort, syncService, syncInterval, leafHubName)
	if err != nil {
		log.Error(err, "failed to create manager")
		return 1
	}

	// control info controller initialization
	controlInfoController, err := controller.NewControlInfoController(syncService, leafHubName, ctrl.Log)
	if err != nil {
		log.Error(err, "failed to initialize control info controller")
		return 1
	}

	if err := mgr.Add(controlInfoController); err != nil {
		log.Error(err, "failed to add control info controller to the manager")
		return 1
	}

	log.Info("Starting the Cmd.")

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "Manager exited non-zero")
		return 1
	}

	return 0
}

func initializeLog() logr.Logger {
	pflag.CommandLine.AddFlagSet(zap.FlagSet())
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	ctrl.SetLogger(zap.Logger())
	log := ctrl.Log.WithName("cmd")

	printVersion(log)

	return log
}

func readEnvVars() (string, time.Duration, string, error) {
	leaderElectionNamespace, found := os.LookupEnv(envVarControllerNamespace)
	if !found {
		return "", 0, "", fmt.Errorf("%w: %s", errEnvVarNotFound, envVarControllerNamespace)
	}

	syncIntervalString, found := os.LookupEnv(envVarSyncInterval)
	if !found {
		return "", 0, "", fmt.Errorf("%w: %s", errEnvVarNotFound, envVarSyncInterval)
	}

	syncInterval, err := time.ParseDuration(syncIntervalString)
	if err != nil {
		return "", 0, "", fmt.Errorf("%w: %s must be an integer", errEnvVarWrongType, envVarSyncInterval)
	}

	leafHubName, found := os.LookupEnv(envVarLeafHubName)
	if !found {
		return "", 0, "", fmt.Errorf("%w: %s", errEnvVarNotFound, envVarLeafHubName)
	}

	return leaderElectionNamespace, syncInterval, leafHubName, nil
}

func createManager(leaderElectionNamespace, metricsHost string, metricsPort int32, transport transport.Transport,
	syncInterval time.Duration, leafHubName string) (ctrl.Manager, error) {
	options := ctrl.Options{
		MetricsBindAddress:      fmt.Sprintf("%s:%d", metricsHost, metricsPort),
		LeaderElection:          true,
		LeaderElectionID:        leaderElectionLockName,
		LeaderElectionNamespace: leaderElectionNamespace,
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new manager: %w", err)
	}

	if err := controller.AddToScheme(mgr.GetScheme()); err != nil {
		return nil, fmt.Errorf("failed to add schemes: %w", err)
	}

	if err := controller.AddControllers(mgr, transport, syncInterval, leafHubName); err != nil {
		return nil, fmt.Errorf("failed to add controllers: %w", err)
	}

	return mgr, nil
}

func main() {
	os.Exit(doMain())
}
