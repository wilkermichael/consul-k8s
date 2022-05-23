package ui

import (
	"os"
	"strings"
	"sync"

	"github.com/hashicorp/consul-k8s/cli/common"
	"github.com/hashicorp/consul-k8s/cli/common/flag"
	"github.com/hashicorp/consul-k8s/cli/common/terminal"
	"github.com/skratchdot/open-golang/open"
	helmCLI "helm.sh/helm/v3/pkg/cli"
	"k8s.io/client-go/kubernetes"
)

var (
	kubecontext = "teckert@hashicorp.com@thomas-eks-test.us-east-2.eksctl.io"
)

func defaultKubeConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return home + "/.kube/config", nil
}

type Command struct {
	*common.BaseCommand

	kubernetes kubernetes.Interface

	set *flag.Sets

	flagKubeConfig  string
	flagKubeContext string

	once sync.Once
	help string
}

func (c *Command) init() {
	kubeconfig, err := defaultKubeConfigPath()
	if err != nil {
		panic(err)
	}

	c.set = flag.NewSets()

	f := c.set.NewSet("GlobalOptions")
	f.StringVar(&flag.StringVar{
		Name:    "kubeconfig",
		Aliases: []string{"c"},
		Target:  &c.flagKubeConfig,
		Default: kubeconfig,
		Usage:   "Set the path to kubeconfig file.",
	})
	f.StringVar(&flag.StringVar{
		Name:    "context",
		Target:  &c.flagKubeContext,
		Default: kubecontext,
		Usage:   "Set the Kubernetes context to use.",
	})

	c.help = c.set.Help()

	c.Init()
}

func (c *Command) Run(args []string) int {
	c.once.Do(c.init)
	c.Log.ResetNamed("ui")
	defer common.CloseWithError(c.BaseCommand)

	if err := c.set.Parse(args); err != nil {
		c.UI.Output(err.Error())
		return 1
	}

	settings := helmCLI.New()
	if c.flagKubeConfig != "" {
		settings.KubeConfig = c.flagKubeConfig
	}
	if c.flagKubeContext != "" {
		settings.KubeContext = c.flagKubeContext
	}
	if c.kubernetes == nil {
		restConfig, err := settings.RESTClientGetter().ToRESTConfig()
		if err != nil {
			c.UI.Output("Error retrieving Kubernetes authentication:\n%v", err, terminal.WithErrorStyle())
			return 1
		}
		c.kubernetes, err = kubernetes.NewForConfig(restConfig)
		if err != nil {
			c.UI.Output("Error initializing Kubernetes client:\n%v", err, terminal.WithErrorStyle())
			return 1
		}
	}

	pf := common.PortForward{
		Namespace:   "consul",
		PodName:     "consul-server-0",
		RemotePort:  8501,
		KubeClient:  c.kubernetes,
		KubeConfig:  settings.KubeConfig,
		KubeContext: settings.KubeContext,
	}
	if err := pf.Open(); err != nil {
		c.UI.Output("Error opening port forward:\n%v", err, terminal.WithErrorStyle())
		return 1
	}

	endpoint, err := pf.Endpoint()
	if err != nil {
		c.UI.Output("Error getting endpoint:\n%v", err, terminal.WithErrorStyle())
		return 1
	}

	open.Run(strings.Replace(endpoint, "http://", "https://", 1))

	for {
	}

	return 0
}

func (c *Command) Synopsis() string {
	return ""
}

func (c *Command) Help() string {
	return ""
}
