package connectinject

import (
	"context"
	"fmt"
	mapset "github.com/deckarep/golang-set"
	"github.com/go-logr/logr"
	"github.com/hashicorp/consul/api"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"strings"
)

type AclCleanupController struct {
	client.Client
	// ConsulClient points at the agent local to the connect-inject deployment pod.
	ConsulClient *api.Client
	// ConsulClientCfg is the client config used by the ConsulClient when calling NewClient().
	ConsulClientCfg *api.Config
	// ConsulScheme is the scheme to use when making API calls to Consul,
	// i.e. "http" or "https".
	ConsulScheme string
	// ConsulPort is the port to make HTTP API calls to Consul agents on.
	ConsulPort string
	// Only endpoints in the AllowK8sNamespacesSet are reconciled.
	AllowK8sNamespacesSet mapset.Set
	// Endpoints in the DenyK8sNamespacesSet are ignored.
	DenyK8sNamespacesSet mapset.Set
	// EnableConsulPartitions indicates that a user is running Consul Enterprise
	// with version 1.11+ which supports Admin Partitions.
	EnableConsulPartitions bool
	// EnableConsulNamespaces indicates that a user is running Consul Enterprise
	// with version 1.7+ which supports namespaces.
	EnableConsulNamespaces bool
	// ConsulDestinationNamespace is the name of the Consul namespace to create
	// all config entries in. If EnableNSMirroring is true this is ignored.
	ConsulDestinationNamespace string
	// EnableNSMirroring causes Consul namespaces to be created to match the
	// k8s namespace of any config entry custom resource. Config entries will
	// be created in the matching Consul namespace.
	EnableNSMirroring bool
	// NSMirroringPrefix is an optional prefix that can be added to the Consul
	// namespaces created while mirroring. For example, if it is set to "k8s-",
	// then the k8s `default` namespace will be mirrored in Consul's
	// `k8s-default` namespace.
	NSMirroringPrefix string
	// CrossNSACLPolicy is the name of the ACL policy to attach to
	// any created Consul namespaces to allow cross namespace service discovery.
	// Only necessary if ACLs are enabled.
	CrossNSACLPolicy string
	// ReleaseName is the Consul Helm installation release.
	ReleaseName string
	// ReleaseNamespace is the namespace where Consul is installed.
	ReleaseNamespace string
	// EnableTransparentProxy controls whether transparent proxy should be enabled
	// for all proxy service registrations.
	EnableTransparentProxy bool
	// TProxyOverwriteProbes controls whether the endpoints controller should expose pod's HTTP probes
	// via Envoy proxy.
	TProxyOverwriteProbes bool
	// AuthMethod is the name of the Kubernetes Auth Method that
	// was used to login with Consul. The Endpoints controller
	// will delete any tokens associated with this auth method
	// whenever service instances are deregistered.
	AuthMethod string

	MetricsConfig MetricsConfig
	Log           logr.Logger

	Scheme *runtime.Scheme
	context.Context
}

// Reconcile reads the state of an Endpoints object for a Kubernetes Service and reconciles Consul services which
// correspond to the Kubernetes Service. These events are driven by changes to the Pods backing the Kube service.
func (r *AclCleanupController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var errs error
	var pod corev1.Pod

	err := r.Client.Get(ctx, req.NamespacedName, &pod)
	r.Log.Info("=========== ", "podName", pod.Name, "errorFromK8s", err)

	if k8serrors.IsNotFound(err) {
		r.Log.Info("=========== delete :", "pod", pod, "errorFromK8s", err)
		err = r.reconcileComponentACLTokens(ctx, req)
		return ctrl.Result{}, err
	} else if err != nil {
		r.Log.Error(err, "failed to get pod", "name", req.Name, "ns", req.Namespace)
		return ctrl.Result{}, err
	}

	r.Log.Info("retrieved", "name", pod.Name, "ns", pod.Namespace)
	return ctrl.Result{}, errs
}

// Reconcile reads the state of an Endpoints object for a Kubernetes Service and reconciles Consul services which
// correspond to the Kubernetes Service. These events are driven by changes to the Pods backing the Kube service.
func (r *AclCleanupController) reconcileComponentACLTokens(ctx context.Context, req ctrl.Request) error {
	var pod corev1.Pod

	// TODO: figure out a filter
	aclTokenList, _, err := r.ConsulClient.ACL().TokenList(&api.QueryOptions{
		AllowStale:        false,
		RequireConsistent: true,
		Filter:            "",
	})
	if err != nil {
		r.Log.Error(err, "unable to fetch TokenList", "error", err)
		return err
	}
	for _, token := range aclTokenList {
		r.Log.Error(err, "======= ACL token: ", "token", token)
		if token.AuthMethod == r.AuthMethod {
			r.Log.Error(err, "============ here ")
			if len(token.Description) > 0 {
				tokenMeta, err := getTokenMetaFromDescription(token.Description)
				if err != nil {
					r.Log.Error(err, "failed to parse token metadata")
					return fmt.Errorf("failed to parse token metadata: %s", err)
				}
				r.Log.Error(err, "======= ACL token Meta: ", "token", tokenMeta)

				tokenPodName := strings.TrimPrefix(tokenMeta[TokenMetaPodNameKey], req.Namespace+"/")
				nsPodName := types.NamespacedName{Name: tokenPodName, Namespace: req.Namespace}

				r.Log.Error(err, "========== attempting to get pod:", "podname", nsPodName)
				err = r.Client.Get(ctx, nsPodName, &pod)
				if k8serrors.IsNotFound(err) {
					r.Log.Info("=========== delete :", "pod", pod.Name, "errorFromK8s", err)
					_, err = r.ConsulClient.ACL().Logout(&api.WriteOptions{Token: token.AccessorID})
					if err != nil {
						r.Log.Error(err, "unable to delete acl token", "error", err)
					}
				} else if err != nil {
					r.Log.Error(err, "failed to get pod", "name", nsPodName)
					return err
				}
			}
		}

	}
	r.Log.Info("completed acl token cleanup")
	return err
}

func (r *AclCleanupController) Logger(name types.NamespacedName) logr.Logger {
	return r.Log.WithValues("request", name)
}

func (r *AclCleanupController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		//For(&corev1.Pod{}).Complete(r).
		For(&corev1.Pod{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				// Suppress Update/Create/Generic events to avoid filtering them out in the Reconcile function
				return false
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Suppress Update/Create/Generic events to avoid filtering them out in the Reconcile function
				return false
			},
			GenericFunc: func(e event.GenericEvent) bool {
				// Suppress Update/Create/Generic events to avoid filtering them out in the Reconcile function
				return false
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				labels := e.Object.GetLabels()
				r.Log.Info("================ delete event received:", "labels", labels)
				return r.filterComponentPods(e.Object)
			},
		}).Complete(r)
}

// filterAgentPods receives meta and object information for Kubernetes resources that are being watched,
// which in this case are Pods. It only returns true if the Pod is a Consul Client Agent Pod. It reads the labels
// from the meta of the resource and uses the values of the "app" and "component" label to validate that
// the Pod is a Consul Client Agent.
func (r *AclCleanupController) filterComponentPods(object client.Object) bool {
	podLabels := object.GetLabels()
	app, ok := podLabels["app"]
	if !ok {
		return false
	}
	component, ok := podLabels["component"]
	if !ok {
		return false
	}

	release, ok := podLabels["release"]
	if !ok {
		return false
	}

	if app == "consul" && component == "controller" && release == r.ReleaseName {
		return true
	}
	return false
}
