package config

import (
	cli "gopkg.in/urfave/cli.v1"
	"k8s.io/client-go/kubernetes"
)

type Config struct {
	Client kubernetes.Interface

	NginxServer string

	SchedName    string
	Vip          string
	Pnmpp        bool
	Idle_Timeout string
}

// AddFlags add flags to app
func (c *Config) AddFlags(app *cli.App) {

	flags := []cli.Flag{
		// ipvsdr
		cli.StringFlag{
			Name:        "schedname",
			Usage:       "Ipvs Sched Algorithm Method, currently supported: rr, lc, sh, dh",
			Destination: &c.SchedName,
		},
		cli.StringFlag{
			Name:        "nginxserver",
			Usage:       "Namespace in which Nginx Controller is deployed",
			Destination: &c.NginxServer,
		},
		cli.StringFlag{
			Name:        "vip",
			Usage:       "Virtual IP of our linux virtual server",
			Destination: &c.Vip,
		},
		cli.StringFlag{
			Name:        "timeout",
			Usage:       "Idle Timeout for tcp tcpfin udp session, e.x: 120-50-50 will set ipvsadm --set tcp tcpfin udp",
			Destination: &c.Idle_Timeout,
		},
		cli.BoolFlag{
			Name:        "pnmpp",
			Usage:       "Persistent Netfilter Marked Packet Persistence, flag: true if set",
			Destination: &c.Pnmpp,
		},
	}

	app.Flags = append(app.Flags, flags...)
}
