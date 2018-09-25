package config

import (
	"k8s.io/client-go/kubernetes"
	cli "gopkg.in/urfave/cli.v1"
)

type Config struct {
	Client			  kubernetes.Interface

	nginxServer		  string

	SchedName		  string
	Vip		          string
	Pcc				  string
	Pnmpp			  string
	Idle_Timeout	  string
}

// AddFlags add flags to app
func (c *Config) AddFlags(app *cli.App) {

	flags := []cli.Flag{
		// ipvsdr
		cli.StringFlag{
			Name:			"schedname",
			Usage:			"Ipvs Sched Algorithm Method, currently supported: rr, lc, sh, dh",
			Destination:	&c.SchedName,
		},
		cli.StringFlag{
			Name:			"nginxServer",
			Usage:			"Namespace in which Nginx Controller is deployed",
			Destination:	&c.nginxServer,
		},
		cli.StringFlag {
			Name:		    "vip",
			Usage:		    "Virtual IP of our linux virtual server",
			Destination:	&c.Vip,
		},
		cli.StringFlag {
			Name:			"timeout",
			Usage:			"Idle Timeout for tcp tcpfin udp session, e.x: 120-50-50 will set ipvsadm --set tcp tcpfin udp",
			Destination:	&c.Idle_Timeout,
		},
		cli.StringFlag {
			Name:			"pcc",
			Usage:			"Persistent client connections",
			Destination:	&c.Pcc,
		},
		cli.StringFlag {
			Name:			"Pnmpp",
			Usage:			"Persistent Netfilter Marked Packet Persistence",
			Destination:	&c.Pnmpp,
		},
	}

	app.Flags = append(app.Flags, flags...)
}
