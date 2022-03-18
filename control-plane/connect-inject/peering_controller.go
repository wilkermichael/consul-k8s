package connectinject

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	consulv1alpha1 "github.com/hashicorp/consul-k8s/control-plane/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PeeringTokenController struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=consul.hashicorp.com,resources=peeringtokens,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=consul.hashicorp.com,resources=peeringtokens/status,verbs=get;update;patch

// Reconcile reconciles peering state.
func (r *PeeringTokenController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	fmt.Printf("Got a req: %s\n", req.Name)
	r.Log.Info("Got a req:", "name", req.Name)
	token := &consulv1alpha1.PeeringToken{}
	err := r.Get(ctx, req.NamespacedName, token)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.Log.Info("found token:", "token", token.Name)
	return ctrl.Result{}, nil
}

func (r *PeeringTokenController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&consulv1alpha1.PeeringToken{}).
		Complete(r)
}
