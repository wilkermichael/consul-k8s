package meshgateway

import (
	"context"
	"fmt"
	"testing"

	terratestk8s "github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/hashicorp/consul-k8s/acceptance/framework/consul"
	"github.com/hashicorp/consul-k8s/acceptance/framework/environment"
	"github.com/hashicorp/consul-k8s/acceptance/framework/helpers"
	"github.com/hashicorp/consul-k8s/acceptance/framework/k8s"
	"github.com/hashicorp/consul-k8s/acceptance/framework/logger"
	"github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const staticServerName = "static-server"
const staticServerNamespace = "ns1"
const lambdaNamespace = "ns2"

// Test that Connect works in a default and ACLsAndAutoEncryptEnabled installations for X-Partition and in-partition networking.
func TestPartitions(t *testing.T) {
	env := suite.Environment()
	cfg := suite.Config()

	if !cfg.EnableEnterprise {
		t.Skipf("skipping this test because -enable-enterprise is not set")
	}

	const defaultPartition = "default"
	const secondaryPartition = "secondary"
	const defaultNamespace = "default"
	serverClusterContext := env.DefaultContext(t)
	clientClusterContext := env.Context(t, environment.SecondaryContextName)

	ctx := context.Background()

	commonHelmValues := map[string]string{
		"global.adminPartitions.enabled": "true",

		"global.enableConsulNamespaces": "true",

		"global.tls.enabled":           "true",
		"global.tls.httpsOnly":         "false",
		"global.tls.enableAutoEncrypt": "false",

		"global.acls.manageSystemACLs": "false",

		"connectInject.enabled": "true",
		"connectInject.consulNamespaces.consulDestinationNamespace": defaultPartition,
		"connectInject.consulNamespaces.mirroringK8S":               "false",

		"meshGateway.enabled":  "true",
		"meshGateway.replicas": "1",

		"controller.enabled": "true",

		"dns.enabled":           "true",
		"dns.enableRedirection": "false",
	}

	serverHelmValues := map[string]string{
		"server.exposeGossipAndRPCPorts": "true",
	}

	serverHelmValues["global.adminPartitions.service.type"] = "NodePort"
	serverHelmValues["global.adminPartitions.service.nodePort.https"] = "30000"
	serverHelmValues["meshGateway.service.type"] = "NodePort"
	serverHelmValues["meshGateway.service.nodePort"] = "30100"

	releaseName := helpers.RandomName()

	helpers.MergeMaps(serverHelmValues, commonHelmValues)

	// Install the consul cluster with servers in the default kubernetes context.
	serverConsulCluster := consul.NewHelmCluster(t, serverHelmValues, serverClusterContext, cfg, releaseName)
	serverConsulCluster.Create(t)

	// Get the TLS CA certificate and key secret from the server cluster and apply it to the client cluster.
	caCertSecretName := fmt.Sprintf("%s-consul-ca-cert", releaseName)
	caKeySecretName := fmt.Sprintf("%s-consul-ca-key", releaseName)

	logger.Logf(t, "retrieving ca cert secret %s from the server cluster and applying to the client cluster", caCertSecretName)
	moveSecret(t, serverClusterContext, clientClusterContext, caCertSecretName)

	logger.Logf(t, "retrieving ca key secret %s from the server cluster and applying to the client cluster", caKeySecretName)
	moveSecret(t, serverClusterContext, clientClusterContext, caKeySecretName)

	partitionServiceName := fmt.Sprintf("%s-consul-partition", releaseName)
	partitionSvcAddress := k8s.ServiceHost(t, cfg, serverClusterContext, partitionServiceName)

	// Create client cluster.
	clientHelmValues := map[string]string{
		"global.enabled": "false",

		"global.adminPartitions.name": secondaryPartition,

		"global.tls.caCert.secretName": caCertSecretName,
		"global.tls.caCert.secretKey":  "tls.crt",

		"externalServers.enabled":       "true",
		"externalServers.hosts[0]":      partitionSvcAddress,
		"externalServers.tlsServerName": "server.dc1.consul",

		"client.enabled":           "true",
		"client.exposeGossipPorts": "true",
		"client.join[0]":           partitionSvcAddress,
	}

	// Provide CA key when auto-encrypt is disabled.
	clientHelmValues["global.tls.caKey.secretName"] = caKeySecretName
	clientHelmValues["global.tls.caKey.secretKey"] = "tls.key"

	clientHelmValues["externalServers.httpsPort"] = "30000"
	clientHelmValues["meshGateway.service.type"] = "NodePort"
	clientHelmValues["meshGateway.service.nodePort"] = "30100"

	helpers.MergeMaps(clientHelmValues, commonHelmValues)

	// Install the consul cluster without servers in the client cluster kubernetes context.
	clientConsulCluster := consul.NewHelmCluster(t, clientHelmValues, clientClusterContext, cfg, releaseName)
	clientConsulCluster.Create(t)

	// Ensure consul clients are created.
	agentPodList, err := clientClusterContext.KubernetesClient(t).CoreV1().Pods(clientClusterContext.KubectlOptions(t).Namespace).List(ctx, metav1.ListOptions{LabelSelector: "app=consul,component=client"})
	require.NoError(t, err)
	require.NotEmpty(t, agentPodList.Items)

	output, err := k8s.RunKubectlAndGetOutputE(t, clientClusterContext.KubectlOptions(t), "logs", agentPodList.Items[0].Name, "-n", clientClusterContext.KubectlOptions(t).Namespace)
	require.NoError(t, err)
	require.Contains(t, output, "Partition: 'secondary'")

	serverClusterStaticServerOpts := &terratestk8s.KubectlOptions{
		ContextName: serverClusterContext.KubectlOptions(t).ContextName,
		ConfigPath:  serverClusterContext.KubectlOptions(t).ConfigPath,
		Namespace:   staticServerNamespace,
	}
	serverClusterStaticClientOpts := &terratestk8s.KubectlOptions{
		ContextName: serverClusterContext.KubectlOptions(t).ContextName,
		ConfigPath:  serverClusterContext.KubectlOptions(t).ConfigPath,
		Namespace:   lambdaNamespace,
	}
	clientClusterStaticServerOpts := &terratestk8s.KubectlOptions{
		ContextName: clientClusterContext.KubectlOptions(t).ContextName,
		ConfigPath:  clientClusterContext.KubectlOptions(t).ConfigPath,
		Namespace:   staticServerNamespace,
	}
	clientClusterStaticClientOpts := &terratestk8s.KubectlOptions{
		ContextName: clientClusterContext.KubectlOptions(t).ContextName,
		ConfigPath:  clientClusterContext.KubectlOptions(t).ConfigPath,
		Namespace:   lambdaNamespace,
	}

	logger.Logf(t, "creating namespaces %s and %s in servers cluster", staticServerNamespace, lambdaNamespace)
	k8s.RunKubectl(t, serverClusterContext.KubectlOptions(t), "create", "ns", staticServerNamespace)
	k8s.RunKubectl(t, serverClusterContext.KubectlOptions(t), "create", "ns", lambdaNamespace)
	helpers.Cleanup(t, cfg.NoCleanupOnFailure, func() {
		k8s.RunKubectl(t, serverClusterContext.KubectlOptions(t), "delete", "ns", staticServerNamespace, lambdaNamespace)
	})

	logger.Logf(t, "creating namespaces %s and %s in clients cluster", staticServerNamespace, lambdaNamespace)
	k8s.RunKubectl(t, clientClusterContext.KubectlOptions(t), "create", "ns", staticServerNamespace)
	k8s.RunKubectl(t, clientClusterContext.KubectlOptions(t), "create", "ns", lambdaNamespace)
	helpers.Cleanup(t, cfg.NoCleanupOnFailure, func() {
		k8s.RunKubectl(t, clientClusterContext.KubectlOptions(t), "delete", "ns", staticServerNamespace, lambdaNamespace)
	})

	consulClient := serverConsulCluster.SetupConsulClient(t, false)

	serverQueryServerOpts := &api.QueryOptions{Namespace: staticServerNamespace, Partition: defaultPartition}
	clientQueryServerOpts := &api.QueryOptions{Namespace: lambdaNamespace, Partition: defaultPartition}

	serverQueryClientOpts := &api.QueryOptions{Namespace: staticServerNamespace, Partition: secondaryPartition}
	clientQueryClientOpts := &api.QueryOptions{Namespace: lambdaNamespace, Partition: secondaryPartition}

	// Create a ProxyDefaults resource to configure services to use the mesh
	// gateways.
	logger.Log(t, "creating proxy-defaults config")
	kustomizeDir := "../fixtures/bases/mesh-gateway"

	k8s.KubectlApplyK(t, serverClusterContext.KubectlOptions(t), kustomizeDir)
	helpers.Cleanup(t, cfg.NoCleanupOnFailure, func() {
		k8s.KubectlDeleteK(t, serverClusterContext.KubectlOptions(t), kustomizeDir)
	})

	k8s.KubectlApplyK(t, clientClusterContext.KubectlOptions(t), kustomizeDir)
	helpers.Cleanup(t, cfg.NoCleanupOnFailure, func() {
		k8s.KubectlDeleteK(t, clientClusterContext.KubectlOptions(t), kustomizeDir)
	})

	logger.Log(t, "test cross-partition networking")
	logger.Log(t, "creating static-server and static-client deployments in server cluster")
	k8s.DeployKustomize(t, serverClusterStaticServerOpts, cfg.NoCleanupOnFailure, cfg.DebugDirectory, "../fixtures/cases/static-server-inject")
	k8s.DeployKustomize(t, serverClusterStaticClientOpts, cfg.NoCleanupOnFailure, cfg.DebugDirectory, "../fixtures/cases/static-client-partitions/default-ns-partition")
	logger.Log(t, "creating static-server and static-client deployments in client cluster")

	// TODO lambda stuff
	k8s.DeployKustomize(t, clientClusterStaticServerOpts, cfg.NoCleanupOnFailure, cfg.DebugDirectory, "../fixtures/cases/static-server-inject")
	k8s.DeployKustomize(t, clientClusterStaticClientOpts, cfg.NoCleanupOnFailure, cfg.DebugDirectory, "../fixtures/cases/static-client-partitions/default-ns-default-partition")
	// TODO end TODO

	// Check that both static-server and static-client have been injected and now have 2 containers in server cluster.
	for _, labelSelector := range []string{"app=static-server", "app=static-client"} {
		podList, err := serverClusterContext.KubernetesClient(t).CoreV1().Pods(metav1.NamespaceAll).List(context.Background(), metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		require.NoError(t, err)
		require.Len(t, podList.Items, 1)
		require.Len(t, podList.Items[0].Spec.Containers, 2)
	}

	// Check that both static-server and static-client have been injected and now have 2 containers in client cluster.
	for _, labelSelector := range []string{"app=static-server", "app=static-client"} {
		podList, err := clientClusterContext.KubernetesClient(t).CoreV1().Pods(metav1.NamespaceAll).List(context.Background(), metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		require.NoError(t, err)
		require.Len(t, podList.Items, 1)
		require.Len(t, podList.Items[0].Spec.Containers, 2)
	}

	services, _, err := consulClient.Catalog().Service(staticServerName, "", serverQueryServerOpts)
	require.NoError(t, err)
	require.Len(t, services, 1)

	services, _, err = consulClient.Catalog().Service(staticClientName, "", clientQueryServerOpts)
	require.NoError(t, err)
	require.Len(t, services, 1)

	services, _, err = consulClient.Catalog().Service(staticServerName, "", serverQueryClientOpts)
	require.NoError(t, err)
	require.Len(t, services, 1)

	services, _, err = consulClient.Catalog().Service(staticClientName, "", clientQueryClientOpts)
	require.NoError(t, err)
	require.Len(t, services, 1)

	k8s.KubectlApplyK(t, serverClusterContext.KubectlOptions(t), "../fixtures/cases/crd-partitions/default-partition-default")
	k8s.KubectlApplyK(t, clientClusterContext.KubectlOptions(t), "../fixtures/cases/crd-partitions/secondary-partition-default")
	helpers.Cleanup(t, cfg.NoCleanupOnFailure, func() {
		k8s.KubectlDeleteK(t, serverClusterContext.KubectlOptions(t), "../fixtures/cases/crd-partitions/default-partition-default")
		k8s.KubectlDeleteK(t, clientClusterContext.KubectlOptions(t), "../fixtures/cases/crd-partitions/secondary-partition-default")
	})

	logger.Log(t, "checking that connection is successful")
	k8s.CheckStaticServerConnectionSuccessful(t, serverClusterStaticClientOpts, staticClientName, "http://localhost:1234")
	k8s.CheckStaticServerConnectionSuccessful(t, clientClusterStaticClientOpts, staticClientName, "http://localhost:1234")
}

func moveSecret(t *testing.T, sourceContext, destContext environment.TestContext, secretName string) {
	t.Helper()

	secret, err := sourceContext.KubernetesClient(t).CoreV1().Secrets(sourceContext.KubectlOptions(t).Namespace).Get(context.Background(), secretName, metav1.GetOptions{})
	secret.ResourceVersion = ""
	require.NoError(t, err)
	_, err = destContext.KubernetesClient(t).CoreV1().Secrets(destContext.KubectlOptions(t).Namespace).Create(context.Background(), secret, metav1.CreateOptions{})
	require.NoError(t, err)
}
