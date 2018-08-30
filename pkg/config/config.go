package config

import (
	"k8s.io/client-go/kubernetes"
	cli "gopkg.in/urfave/cli.v1"
)

type Config struct {
	Client	  kubernetes.Interface

	SchedName string
	Vip		  string
}

// AddFlags add flags to app
func (c *Config) AddFlags(app *cli.App) {

	flags := []cli.Flag{
		// ipvsdr
		cli.StringFlag{
			Name:		 "schedname",
			Usage:		 "Ipvs Sched Algorithm Method, currently supported: rr, lc, sh, dh",
			Destination: &c.SchedName,
		},

		cli.StringFlag {
			Name:		    "vip",
			Usage:		    "Virtual IP of our linux virtual server",
			Destination:	&c.Vip,
		},
	}

	app.Flags = append(app.Flags, flags...)
}
