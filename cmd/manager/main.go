// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"

	"github.com/go-logr/logr"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	compressor "github.com/open-cluster-management/hub-of-hubs-message-compression"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport/kafka"
	syncservice "github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport/sync-service"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"github.com/spf13/pflag"
	v1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	metricsHost                             = "0.0.0.0"
	metricsPort                       int32 = 8527
	envVarLeafHubName                       = "LH_ID"
	envVarControllerNamespace               = "POD_NAMESPACE"
	envVarTransportMsgCompressionType       = "TRANSPORT_MESSAGE_COMPRESSION_TYPE"
	envVarTransportType                     = "TRANSPORT_TYPE"
	kafkaTransportTypeName                  = "kafka"
	syncServiceTransportTypeName            = "sync-service"
	leaderElectionLockName                  = "leaf-hub-status-sync-lock"
	incarnationConfigMapKey                 = "incarnation"
	base10                                  = 10
	uint64Size                              = 64
)

var (
	errEnvVarNotFound       = errors.New("environment variable not found")
	errEnvVarIllegalValue   = errors.New("environment variable illegal value")
	errMapDoesNotContainKey = errors.New("map does not contain key")
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
	case kafkaTransportTypeName:
		kafkaProducer, err := kafka.NewProducer(msgCompressor, ctrl.Log.WithName("kafka"))
		if err != nil {
			return nil, fmt.Errorf("failed to create kafka-producer: %w", err)
		}

		return kafkaProducer, nil
	case syncServiceTransportTypeName:
		syncService, err := syncservice.NewSyncService(msgCompressor, ctrl.Log.WithName("sync-service"))
		if err != nil {
			return nil, fmt.Errorf("failed to create sync-service: %w", err)
		}

		return syncService, nil
	default:
		return nil, fmt.Errorf("%w: %s - %s is not a valid option", errEnvVarIllegalValue, envVarTransportType,
			transportType)
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

	mgr, incarnation, err := createManager(leaderElectionNamespace, transportObj, leafHubName)
	if err != nil {
		log.Error(err, "Failed to create manager")
		return 1
	}

	log.Info("Starting the Cmd", "incarnation", incarnation)

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "Manager exited non-zero")
		return 1
	}

	return 0
}

func createManager(leaderElectionNamespace string, transport transport.Transport,
	leafHubName string) (ctrl.Manager, uint64, error) {
	options := ctrl.Options{
		MetricsBindAddress:      fmt.Sprintf("%s:%d", metricsHost, metricsPort),
		LeaderElection:          true,
		LeaderElectionID:        leaderElectionLockName,
		LeaderElectionNamespace: leaderElectionNamespace,
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create a new manager: %w", err)
	}

	if err := controller.AddToScheme(mgr.GetScheme()); err != nil {
		return nil, 0, fmt.Errorf("failed to add schemes: %w", err)
	}

	// incarnation version
	incarnation, err := getIncarnation(mgr)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get incarnation version: %w", err)
	}

	if err := controller.AddControllers(mgr, transport, leafHubName, incarnation); err != nil {
		return nil, 0, fmt.Errorf("failed to add controllers: %w", err)
	}

	return mgr, incarnation, nil
}

// Incarnation is a part of the version of all the messages this process will transport.
// The motivation behind this logic is allowing the message receivers/consumers to infer that messages transmitted
// from this instance are more recent than all other existing ones, regardless of their instance-specific generations.
func getIncarnation(mgr ctrl.Manager) (uint64, error) {
	client, err := ctrlClient.New(mgr.GetConfig(), ctrlClient.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		return 0, fmt.Errorf("failed to start k8s client - %w", err)
	}

	ctx := context.Background()
	configMap := &v1.ConfigMap{}

	// try to get ConfigMap
	objKey := ctrlClient.ObjectKey{
		Namespace: datatypes.HohSystemNamespace,
		Name:      incarnationConfigMapKey,
	}
	if err := client.Get(ctx, objKey, configMap); err != nil {
		if !k8sErrors.IsNotFound(err) {
			return 0, fmt.Errorf("failed to get incarnation ConfigMap - %w", err)
		}

		// incarnation ConfigMap does not exist, create it with incarnation = 0
		configMap = createIncarnationConfigMap(datatypes.HohSystemNamespace, 0)
		if err := client.Create(ctx, configMap); err != nil {
			return 0, fmt.Errorf("failed to create incarnation ConfigMap obj - %w", err)
		}

		return 0, nil
	}

	// incarnation configMap exists, get incarnation, increment it and update object
	incarnationString, exists := configMap.Data[incarnationConfigMapKey]
	if !exists {
		return 0, fmt.Errorf("%w - ConfigMap (%s): key (%s)", errMapDoesNotContainKey,
			incarnationConfigMapKey, incarnationConfigMapKey)
	}

	lastIncarnation, err := strconv.ParseUint(incarnationString, base10, uint64Size)
	if err != nil {
		return 0, fmt.Errorf("failed to parse value of key %s in ConfigMap %s - %w", incarnationConfigMapKey,
			incarnationConfigMapKey, err)
	}

	newConfigMap := createIncarnationConfigMap(datatypes.HohSystemNamespace, lastIncarnation+1)
	if err := client.Patch(ctx, newConfigMap, ctrlClient.MergeFrom(configMap)); err != nil {
		return 0, fmt.Errorf("failed to update incarnation version - %w", err)
	}

	return lastIncarnation + 1, nil
}

func createIncarnationConfigMap(namespace string, incarnation uint64) *v1.ConfigMap {
	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      incarnationConfigMapKey,
			Namespace: namespace,
		},
		Data: map[string]string{incarnationConfigMapKey: strconv.FormatUint(incarnation, base10)},
	}
}

func main() {
	os.Exit(doMain())
}
