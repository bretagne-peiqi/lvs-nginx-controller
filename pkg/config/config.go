package config

import (
	"k8s.io/client-go/kubernetes"
	cli "gopkg.in/urfave/cli.v1"
)

type Config struct {
	Client	  kubernetes.Interface

	SchedName string
	VIP		  string
}

// AddFlags add flags to app
func (c *Config) AddFlags(app *cli.App) {

	flags := []cli.Flag{
		// ipvsdr
		cli.StringFlag{
			Name:		 "SchedName",
			Usage:		 "Ipvs Sched Algorithm Method",
			Destination: &c.SchedName,
		},

		cli.StringFlag {
			Name:			"VIP",
			Usage:			"Virtual IP of our linux virtual server",
			Destination:	&c.VIP,
		},
	}
	app.Flags = append(app.Flags, flags...)
}
