package options

import (
	"github.com/bretagne-peiqi/lvs-nginx-controller/pkg/config"
	"gopkg.in/urfave/cli.v1"
)

type Options struct {
	Kubeconfig string
	Debug      bool
	Cfg        config.Config
}

// NewOptions reutrns a new Options
func NewOptions() *Options {
	return &Options{}
}

// AddFlags add flags to app
func (opts *Options) AddFlags(app *cli.App) {

	opts.Cfg.AddFlags(app)

	flags := []cli.Flag{
		cli.StringFlag{
			Name:        "kubeconfig",
			Usage:       "Path to a kube config.",
			Destination: &opts.Kubeconfig,
		},
		cli.BoolFlag{
			Name:        "debug",
			Usage:       "Run with debug mode",
			Destination: &opts.Debug,
		},
	}

	app.Flags = append(app.Flags, flags...)
}
