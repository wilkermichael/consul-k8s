package meshgateway

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/consul-k8s/acceptance/framework/consul"
	"github.com/hashicorp/consul-k8s/acceptance/framework/environment"
	"github.com/hashicorp/consul-k8s/acceptance/framework/helpers"
	"github.com/hashicorp/consul-k8s/acceptance/framework/k8s"
	"github.com/hashicorp/consul-k8s/acceptance/framework/logger"
	"github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const staticClientName = "static-client"
const terminatingLambdaName = "terminating-lambda"
const meshLambdaName = "mesh-lambda"

// Test that Connect and wan federation over mesh gateways work in a default installation
// i.e. without ACLs because TLS is required for WAN federation over mesh gateways.
func TestMeshGatewayDefault(t *testing.T) {
	env := suite.Environment()
	cfg := suite.Config()

	primaryContext := env.DefaultContext(t)
	secondaryContext := env.Context(t, environment.SecondaryContextName)

	t.Log("seetting primary values")
	primaryHelmValues := map[string]string{
		"global.datacenter":                        "dc1",
		"global.image":                             "ghcr.io/erichaberkorn/consul:latest",
		"global.tls.enabled":                       "true",
		"global.tls.httpsOnly":                     "false",
		"global.federation.enabled":                "true",
		"global.federation.createFederationSecret": "true",

		"connectInject.enabled":  "true",
		"connectInject.replicas": "1",
		"controller.enabled":     "true",

		"meshGateway.enabled":  "true",
		"meshGateway.replicas": "1",

		"terminatingGateways.enabled":              "true",
		"terminatingGateways.gateways[0].name":     "dc1-terminating-gateway",
		"terminatingGateways.gateways[0].replicas": "1",

		"client.extraConfig": `"{"connect": {"enable_serverless_plugin": true}}"`,

		"dns.enabled":           "true",
		"dns.enableRedirection": "true",
	}

	if cfg.UseKind {
		primaryHelmValues["meshGateway.service.type"] = "NodePort"
		primaryHelmValues["meshGateway.service.nodePort"] = "30000"
	}

	releaseName := helpers.RandomName()

	// Install the primary consul cluster in the default kubernetes context
	t.Log("helming primary cluster")
	primaryConsulCluster := consul.NewHelmCluster(t, primaryHelmValues, primaryContext, cfg, releaseName)
	primaryConsulCluster.Create(t)
	t.Log("doning primary cluster")

	// Get the federation secret from the primary cluster and apply it to secondary cluster
	federationSecretName := fmt.Sprintf("%s-consul-federation", releaseName)
	logger.Logf(t, "retrieving federation secret %s from the primary cluster and applying to the secondary", federationSecretName)
	federationSecret, err := primaryContext.KubernetesClient(t).CoreV1().Secrets(primaryContext.KubectlOptions(t).Namespace).Get(context.Background(), federationSecretName, metav1.GetOptions{})
	federationSecret.ResourceVersion = ""
	require.NoError(t, err)
	_, err = secondaryContext.KubernetesClient(t).CoreV1().Secrets(secondaryContext.KubectlOptions(t).Namespace).Create(context.Background(), federationSecret, metav1.CreateOptions{})
	require.NoError(t, err)

	// Create secondary cluster
	secondaryHelmValues := map[string]string{
		"global.datacenter": "dc2",
		"global.image":      "ghcr.io/erichaberkorn/consul:latest",

		"global.tls.enabled":           "true",
		"global.tls.httpsOnly":         "false",
		"global.tls.caCert.secretName": federationSecretName,
		"global.tls.caCert.secretKey":  "caCert",
		"global.tls.caKey.secretName":  federationSecretName,
		"global.tls.caKey.secretKey":   "caKey",

		"global.federation.enabled": "true",

		"server.extraVolumes[0].type":          "secret",
		"server.extraVolumes[0].name":          federationSecretName,
		"server.extraVolumes[0].load":          "true",
		"server.extraVolumes[0].items[0].key":  "serverConfigJSON",
		"server.extraVolumes[0].items[0].path": "config.json",

		"connectInject.enabled":  "true",
		"connectInject.replicas": "1",
		"controller.enabled":     "true",

		"meshGateway.enabled":  "true",
		"meshGateway.replicas": "1",

		"terminatingGateways.enabled":              "true",
		"terminatingGateways.gateways[0].name":     "dc2-terminating-gateway",
		"terminatingGateways.gateways[0].replicas": "1",

		"client.extraConfig": `"{"connect": {"enable_serverless_plugin": true}}"`,

		"dns.enabled":           "true",
		"dns.enableRedirection": "true",
	}

	if cfg.UseKind {
		secondaryHelmValues["meshGateway.service.type"] = "NodePort"
		secondaryHelmValues["meshGateway.service.nodePort"] = "30000"
	}

	// Install the secondary consul cluster in the secondary kubernetes context
	secondaryConsulCluster := consul.NewHelmCluster(t, secondaryHelmValues, secondaryContext, cfg, releaseName)
	secondaryConsulCluster.Create(t)

	if cfg.UseKind {
		// This is a temporary workaround that seems to fix mesh gateway tests on kind 1.22.x.
		// TODO (ishustava): we need to investigate this further and remove once we've found the issue.
		k8s.RunKubectl(t, primaryContext.KubectlOptions(t), "rollout", "restart", fmt.Sprintf("sts/%s-consul-server", releaseName))
		k8s.RunKubectl(t, primaryContext.KubectlOptions(t), "rollout", "status", fmt.Sprintf("sts/%s-consul-server", releaseName))
	}
	// Add AWS stuff to the environment.

	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	accessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	sessionToken := os.Getenv("AWS_SESSION_TOKEN")
	for _, c := range []struct {
		context environment.TestContext
		dcName  string
	}{
		{primaryContext, "dc1"},
		{secondaryContext, "dc2"},
	} {
		deployment := fmt.Sprintf("deployment/%s-consul-%s-terminating-gateway", releaseName, c.dcName)
		k8s.RunKubectl(t, c.context.KubectlOptions(t), "scale", deployment, "--replicas=0")
		k8s.RunKubectl(t, c.context.KubectlOptions(t), "set", "env",
			deployment,
			fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", accessKeyID),
			fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", accessKey),
			fmt.Sprintf("AWS_SESSION_TOKEN=%s", sessionToken))
		k8s.RunKubectl(t, c.context.KubectlOptions(t), "scale", deployment, "--replicas=1")
	}

	primaryClient := primaryConsulCluster.SetupConsulClient(t, false)
	secondaryClient := secondaryConsulCluster.SetupConsulClient(t, false)

	// Verify federation between servers
	logger.Log(t, "verifying federation was successful")
	helpers.VerifyFederation(t, primaryClient, secondaryClient, releaseName, false)

	// Create a ProxyDefaults resource to configure services to use the mesh
	// gateways.
	logger.Log(t, "creating proxy-defaults config")
	kustomizeDir := "../fixtures/bases/mesh-gateway"
	k8s.KubectlApplyK(t, primaryContext.KubectlOptions(t), kustomizeDir)
	helpers.Cleanup(t, cfg.NoCleanupOnFailure, func() {
		k8s.KubectlDeleteK(t, primaryContext.KubectlOptions(t), kustomizeDir)
	})

	registerLambda(t, primaryClient, terminatingLambdaName, "us-east-1", "arn:aws:lambda:us-east-1:977604411308:function:lambda-registration-1234-example1")
	storeTerminatingGatewayConfiguration(t, primaryClient, "dc1", terminatingLambdaName)
	registerLambda(t, secondaryClient, meshLambdaName, "us-east-2", "arn:aws:lambda:us-east-2:977604411308:function:consul-ecs-lambda-test")
	storeTerminatingGatewayConfiguration(t, secondaryClient, "dc2", meshLambdaName)

	intention := api.Intention{
		SourceName:      staticClientName,
		DestinationName: terminatingLambdaName,
		Action:          api.IntentionActionDeny,
		SourceType:      api.IntentionSourceConsul,
	}
	ixnID := storeIntention(t, primaryClient, intention)
	intention.ID = ixnID
	t.Cleanup(func() {
		deleteIntention(t, primaryClient, ixnID)
	})

	// TODO intentions
	// TODO L7
	// TODO Namespaces/Partitions

	logger.Log(t, "creating static-client in dc1")
	k8s.DeployKustomize(t, primaryContext.KubectlOptions(t), cfg.NoCleanupOnFailure, cfg.DebugDirectory, "../fixtures/cases/static-client-lambdas")

	expectedDC1 := "Hello foo!"
	expectedDC2 := "Hello from dc2"
	// Intentions work
	k8s.CheckStaticServerConnectionFailing(t, primaryContext.KubectlOptions(t), staticClientName, "http://localhost:1234", "-d", "{\"name\": \"foo\"}")

	logger.Log(t, "Testing explicit upstreams in the local datacenter")
	intention.Action = api.IntentionActionAllow
	updateIntention(t, primaryClient, intention)

	// Explicit upstream
	k8s.CheckStaticServerConnectionSuccessfulWithMessage(t, primaryContext.KubectlOptions(t), staticClientName, expectedDC1, "http://localhost:1234", "-d", "{\"name\": \"foo\"}")

	// Mesh gateway
	logger.Log(t, "Testing explicit upstreams in another datacenter")
	k8s.CheckStaticServerConnectionSuccessfulWithMessage(t, primaryContext.KubectlOptions(t), staticClientName, expectedDC2, "http://localhost:2345", "-d", "{\"name\": \"foo\"}")

	k8s.DeployKustomize(t, primaryContext.KubectlOptions(t), cfg.NoCleanupOnFailure, cfg.DebugDirectory, "../fixtures/cases/static-client-tproxy")
	logger.Log(t, "Testing transparent proxy")
	k8s.CheckStaticServerConnectionSuccessfulWithMessage(t,
		primaryContext.KubectlOptions(t), staticClientName, expectedDC1,
		"http://terminating-lambda.virtual.consul", "-d", "{\"name\": \"foo\"}")

	logger.Log(t, "Testing L7")
	redirectTerminatingLambdaToMeshLambda(t, primaryClient)
	k8s.CheckStaticServerConnectionSuccessfulWithMessage(t,
		primaryContext.KubectlOptions(t), staticClientName, expectedDC2,
		"http://terminating-lambda.virtual.consul", "-d", "{\"name\": \"foo\"}", "-H", "CANARY: true")
}

func registerLambda(t *testing.T, client *api.Client, name, region, arn string) {
	serviceDefaults := &api.ServiceConfigEntry{
		Kind:     api.ServiceDefaults,
		Name:     name,
		Protocol: "http",
		Meta: map[string]string{
			"serverless.consul.hashicorp.com/v1alpha1/lambda/enabled":              "true",
			"serverless.consul.hashicorp.com/v1alpha1/lambda/arn":                  arn,
			"serverless.consul.hashicorp.com/v1alpha1/lambda/payload-passhthrough": "true",
			"serverless.consul.hashicorp.com/v1alpha1/lambda/region":               region,
		},
	}
	_, _, err := client.ConfigEntries().Set(serviceDefaults, nil)

	registration := &api.CatalogRegistration{
		Node:           "lambdas",
		SkipNodeUpdate: true,
		NodeMeta: map[string]string{
			"external-node":  "true",
			"external-probe": "true",
		},
		Service: &api.AgentService{
			ID:      name,
			Service: name,
		},
	}
	_, err = client.Catalog().Register(registration, nil)
	require.NoError(t, err)
}

func storeTerminatingGatewayConfiguration(t *testing.T, client *api.Client, dcName, lambdaName string) {
	config := &api.TerminatingGatewayConfigEntry{
		Kind: api.TerminatingGateway,
		Name: fmt.Sprintf("%s-%s", dcName, api.TerminatingGateway),
		Services: []api.LinkedService{
			{
				Name: lambdaName,
			},
		},
	}
	_, _, err := client.ConfigEntries().Set(config, nil)
	require.NoError(t, err)
}

func storeIntention(t *testing.T, client *api.Client, intention api.Intention) string {
	id, _, err := client.Connect().IntentionCreate(&intention, nil)
	require.NoError(t, err)
	return id
}

func deleteIntention(t *testing.T, client *api.Client, id string) {
	_, err := client.Connect().IntentionDelete(id, nil)
	require.NoError(t, err)
}

func updateIntention(t *testing.T, client *api.Client, intention api.Intention) {
	_, err := client.Connect().IntentionUpdate(&intention, nil)
	require.NoError(t, err)
}

func redirectTerminatingLambdaToMeshLambda(t *testing.T, client *api.Client) {
	serviceResolver := api.ServiceResolverConfigEntry{
		Kind: api.ServiceResolver,
		Name: terminatingLambdaName,
		Redirect: &api.ServiceResolverRedirect{
			Service:    meshLambdaName,
			Datacenter: "dc2",
		},
	}

	_, _, err := client.ConfigEntries().Set(&serviceResolver, nil)
	require.NoError(t, err)
}
