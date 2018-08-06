package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	 ipvs "github.com/lvs-controller/pkg/controller"

	"github.com/golang/glog"
    "gopkg.in/urfave/cli.v1"

    "k8s.io/apimachinery/pkg/util/wait"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/tools/clientcmd"
)

func RunController(opts *Options, stopCh <-chan struct{}) error {
	glog.Infof("The ipvs controller started ...")

	glog.Infof("load kubeconfig from %s", opts.Kubeconfig)
	config, err := clientcmd.BuildConfigFromFlags("", opts.Kubeconfig)
	if err != nil {
		glog.Fatalf("failed to build kubernetes config due to err: %s\n", err)
		return err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Fatalf("failed to create kubernetes client due to err: %s\n", err)
		return err
	}

	opts.Cfg.Client = clientset

	controller := ipvs.NewLoadBalancerController(opts.Cfg)
	controller.Run(1,wait.NeverStop)

	return nil
}

func main() {
	//fixme
	flag.CommandLine.Parse([]string{})

	app := cli.NewApp()
	app.Name = "ipvs-controller"
	app.Usage = "sync k8s ingress-controller resources for ip_vs loadbalancer"

	opts := NewOptions()
	opts.AddFlags(app)

	app.Action = func(c *cli.Context) error {
		if err := RunController(opts, wait.NeverStop); err != nil {
			msg := fmt.Sprintf("failed to run ipvs controller, err is %s\n", err)
			return cli.NewExitError(msg, 1)
		}
		return nil
	}

	sort.Sort(cli.FlagsByName(app.Flags))

	app.Run(os.Args)
}
