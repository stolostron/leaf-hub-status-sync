// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/go-logr/logr"
	compressor "github.com/open-cluster-management/hub-of-hubs-message-compression"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	syncservice "github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport/sync-service"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"github.com/spf13/pflag"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	metricsHost                             = "0.0.0.0"
	metricsPort                       int32 = 8527
	envVarLeafHubName                       = "LH_ID"
	envVarControllerNamespace               = "POD_NAMESPACE"
	envVarTransportMsgCompressionType       = "TRANSPORT_MESSAGE_COMPRESSION_TYPE"
	envVarTransportType                     = "TRANSPORT_TYPE"
	syncServiceTransportTypeName            = "sync-service"
	leaderElectionLockName                  = "leaf-hub-status-sync-lock"
)

var (
	errEnvVarNotFound     = errors.New("environment variable not found")
	errEnvVarIllegalValue = errors.New("environment variable illegal value")
)

func printVersion(log logr.Logger) {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	log.Info(fmt.Sprintf("Version of operator-sdk: %s", sdkVersion.Version))
}

// function to choose transport type based on env var.
func getTransport(transportType string, transportMsgCompressorType string) (transport.Transport, error) {
	msgCompressor, err := compressor.NewCompressor(compressor.CompressionType(transportMsgCompressorType))
	if err != nil {
		return nil, fmt.Errorf("failed to create message-compressor: %w", err)
	}

	switch transportType {
	case syncServiceTransportTypeName:
		syncService, err := syncservice.NewSyncService(msgCompressor, ctrl.Log.WithName("sync-service"))
		if err != nil {
			return nil, fmt.Errorf("failed to create sync-service: %w", err)
		}

		return syncService, nil
	default:
		return nil, fmt.Errorf("%w: %s", errEnvVarIllegalValue, transportType)
	}
}

func readEnvVars() (string, string, string, string, error) {
	leaderElectionNamespace, found := os.LookupEnv(envVarControllerNamespace)
	if !found {
		return "", "", "", "", fmt.Errorf("%w: %s", errEnvVarNotFound, envVarControllerNamespace)
	}

	leafHubName, found := os.LookupEnv(envVarLeafHubName)
	if !found {
		return "", "", "", "", fmt.Errorf("%w: %s", errEnvVarNotFound, envVarLeafHubName)
	}

	transportType, found := os.LookupEnv(envVarTransportType)
	if !found {
		return "", "", "", "", fmt.Errorf("%w: %s", errEnvVarNotFound, envVarTransportType)
	}

	transportMsgCompressionType, found := os.LookupEnv(envVarTransportMsgCompressionType)
	if !found {
		return "", "", "", "", fmt.Errorf("%w: %s", errEnvVarNotFound, envVarTransportMsgCompressionType)
	}

	return leaderElectionNamespace, leafHubName, transportType, transportMsgCompressionType, nil
}

func doMain() int {
	pflag.CommandLine.AddFlagSet(zap.FlagSet())
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	ctrl.SetLogger(zap.Logger())
	log := ctrl.Log.WithName("cmd")

	printVersion(log)

	leaderElectionNamespace, leafHubName, transportType, transportMsgCompressionType, err := readEnvVars()
	if err != nil {
		log.Error(err, "initialization error")
		return 1
	}

	// transport layer initialization
	transportObj, err := getTransport(transportType, transportMsgCompressionType)
	if err != nil {
		log.Error(err, "transport initialization error")
		return 1
	}

	transportObj.Start()
	defer transportObj.Stop()

	mgr, err := createManager(leaderElectionNamespace, transportObj, leafHubName)
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

func createManager(leaderElectionNamespace string, transport transport.Transport,
	leafHubName string) (ctrl.Manager, error) {
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

	if err := controller.AddControllers(mgr, transport, leafHubName); err != nil {
		return nil, fmt.Errorf("failed to add controllers: %w", err)
	}

	return mgr, nil
}

func main() {
	os.Exit(doMain())
}
