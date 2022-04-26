package connectinject

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	consulv1alpha1 "github.com/hashicorp/consul-k8s/control-plane/api/v1alpha1"
	"github.com/hashicorp/consul-k8s/control-plane/consul"
	"github.com/hashicorp/consul/api"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PeeringTokenController struct {
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
	Log        logr.Logger
	Scheme     *runtime.Scheme
}

//+kubebuilder:rbac:groups=consul.hashicorp.com,resources=peeringtokens,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=consul.hashicorp.com,resources=peeringtokens/status,verbs=get;update;patch

// Reconcile reconciles peering state.
func (r *PeeringTokenController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log.Info("got a request for peering token:", "name", req.Name)
	token := &consulv1alpha1.PeeringToken{}
	err := r.Get(ctx, req.NamespacedName, token)
	if k8serrors.IsNotFound(err) {
		r.Log.Info("PeeringToken was deleted", "name", req.Name, "namespace", req.Namespace)
		// call delete endpoint in consul
		// delete secret created by this peeringtoken
	} else if err != nil {
		r.Log.Error(err, "failed to get PeeringToken", "name", req.Name, "ns", req.Namespace)
		return ctrl.Result{}, err
	}
	r.Log.Info("found PeeringToken:", "token", token.Name)
	http.Post("http://consul-server:8500/v1/peering/token", "application/json", bytes.NewReader([]byte("{\"PeerName\":\"foo\"}")))
	//httpReq, err := http.NewRequest("POST", "v1/peering/token", bytes.NewReader([]byte("{\"PeerName\":\"foo\"}")))
	return ctrl.Result{}, nil
}

func (r *PeeringTokenController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&consulv1alpha1.PeeringToken{}).
		Complete(r)
}

// remoteConsulClient returns an *api.Client that points at the consul agent local to the pod for a provided namespace.
func (r *PeeringTokenController) remoteConsulClient(ip string, namespace string) (*api.Client, error) {
	newAddr := fmt.Sprintf("%s://%s:%s", r.ConsulScheme, ip, r.ConsulPort)
	localConfig := r.ConsulClientCfg
	localConfig.Address = newAddr
	localConfig.Namespace = namespace
	return consul.NewClient(localConfig)
}
