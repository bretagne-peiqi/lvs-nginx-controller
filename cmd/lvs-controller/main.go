package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"runtime"
	"strconv"

	ipvs "github.com/bretagne-peiqi/lvs-nginx-controller/pkg/controller"

	glog "github.com/zoumo/logdog"
	"gopkg.in/urfave/cli.v1"

	"net/http"
	_ "net/http/pprof"
	//pprof "runtime/pprof"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func RunController(opts *Options, stopCh <-chan struct{}) error {

	glog.Infof("The ipvs controller started ...")

	if opts.Debug {
		glog.ApplyOptions(glog.DebugLevel)
	} else {
		glog.ApplyOptions(glog.InfoLevel)
	}

	//build config
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
	if err := controller.Initial(); err != nil {
		glog.Fatalf("Failed initial ipvs controller, err %v\n", err)
	}
	controller.Run(5, wait.NeverStop)

	return nil
}

func main() {
	//FIXME
	flag.CommandLine.Parse([]string{})

	app := cli.NewApp()
	app.Name = "ipvs-controller"
	app.Usage = "sync k8s ingress-controller resources for ip_vs loadbalancer\n    This controller is intented to used at L4 level, but we need to config ports 80&443 definitivily\n    when combining used with nginx-ingress-controller"
	app.Version = "beta 1.0"

	opts := NewOptions()
	opts.AddFlags(app)

	go func() {
		http.ListenAndServe("localhost:6060", nil)
	}()

	app.Action = func(c *cli.Context) error {
		if err := RunController(opts, wait.NeverStop); err != nil {
			msg := fmt.Sprintf("failed to run ipvs controller, err is %s\n", err)
			return cli.NewExitError(msg, 1)
		}
		return nil
	}

	go func() {
		http.HandleFunc("/goroutines", func(w http.ResponseWriter, r *http.Request) {
			num := strconv.FormatInt(int64(runtime.NumGoroutine()), 10)
			w.Write([]byte(num))
		})
		http.ListenAndServe("localhost:8081", http.DefaultServeMux)
		glog.Info("goroutine stats and pprof listen on 8081")

	}()

	sort.Sort(cli.FlagsByName(app.Flags))
	app.Run(os.Args)

}
