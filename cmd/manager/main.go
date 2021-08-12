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
	kafkaclient "github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport/kafka-client"
	lhSyncService "github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport/sync-service"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"github.com/spf13/pflag"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	metricsHost                        = "0.0.0.0"
	metricsPort                  int32 = 8527
	kafkaTransportTypeName             = "kafka"
	syncServiceTransportTypeName       = "syncservice"
	envVarSyncInterval                 = "PERIODIC_SYNC_INTERVAL"
	envVarLeafHubName                  = "LH_ID"
	envVarControllerNamespace          = "POD_NAMESPACE"
	envVarTransportComponent           = "LH_TRANSPORT_TYPE"
	leaderElectionLockName             = "leaf-hub-status-sync-lock"
)

var (
	errEnvVarNotFound     = errors.New("not found environment variable")
	errEnvVarIllegalValue = errors.New("environment variable illegal value")
)

func printVersion(log logr.Logger) {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	log.Info(fmt.Sprintf("Version of operator-sdk: %s", sdkVersion.Version))
}

// function to choose transport type based on env var.
func getTransport(transportType string) (transport.Transport, error) {
	switch transportType {
	case kafkaTransportTypeName:
		kafkaProducer, err := kafkaclient.NewLHProducer(ctrl.Log.WithName("kafka-client"))
		if err != nil {
			return nil, fmt.Errorf("failed to create lh-kafka-producer: %w", err)
		}

		return kafkaProducer, nil
	case syncServiceTransportTypeName:
		syncService, err := lhSyncService.NewSyncService(ctrl.Log.WithName("sync-service"))
		if err != nil {
			return nil, fmt.Errorf("failed to create sync-service: %w", err)
		}

		return syncService, nil
	default:
		return nil, errEnvVarIllegalValue
	}
}

func readEnvVars(log logr.Logger) (string, time.Duration, string, string, error) {
	leaderElectionNamespace, found := os.LookupEnv(envVarControllerNamespace)
	if !found {
		log.Error(nil, "Not found:", "environment variable", envVarControllerNamespace)
		return "", 0, "", "", errEnvVarNotFound
	}

	syncIntervalString, found := os.LookupEnv(envVarSyncInterval)
	if !found {
		log.Error(nil, "Not found:", "environment variable", envVarSyncInterval)
		return "", 0, "", "", errEnvVarNotFound
	}

	syncInterval, err := time.ParseDuration(syncIntervalString)
	if err != nil {
		log.Error(err, "the environment var ", envVarSyncInterval, " is not valid duration")
		return "", 0, "", "", errEnvVarNotFound
	}

	leafHubName, found := os.LookupEnv(envVarLeafHubName)
	if !found {
		log.Error(nil, "Not found:", "environment variable", envVarLeafHubName)
		return "", 0, "", "", errEnvVarNotFound
	}

	transportType, found := os.LookupEnv(envVarTransportComponent)
	if !found {
		log.Error(nil, "Not found:", "environment variable", envVarTransportComponent)
		return "", 0, "", "", errEnvVarNotFound
	}

	return leaderElectionNamespace, syncInterval, leafHubName, transportType, nil
}

func doMain() int {
	pflag.CommandLine.AddFlagSet(zap.FlagSet())
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	ctrl.SetLogger(zap.Logger())
	log := ctrl.Log.WithName("cmd")

	printVersion(log)

	leaderElectionNamespace, syncInterval, leafHubName, transportType, err := readEnvVars(log)
	if err != nil {
		return 1
	}

	// transport layer initialization
	transportObj, err := getTransport(transportType)
	if err != nil {
		log.Error(err, "initialization error", "failed to initialize", transportType)
		return 1
	}

	transportObj.Start()
	defer transportObj.Stop()

	mgr, err := createManager(leaderElectionNamespace, metricsHost, metricsPort, transportObj, syncInterval, leafHubName)
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
