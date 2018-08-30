package controller

import (
	//"encoding/json"
	"sync"

	//"k8s.io/api/core/v1"
)


type AddrStore struct {
	lock   sync.RWMutex
	Name   string
	Addr   string
	Action string
}

type DeltaAddr struct {
	lock   sync.RWMutex
	addrs  map[string]string
}

type PortToNginx struct {

	Port			string	`json:"port"` //udp&tcp port
	Proto			string  `json:"proto"`
	Nginx		    string  `json:"nginxIpv4"`
	ServiceName	    string	`json:"serviceName, omitempty"`
}

type PortToNginxList  struct {

	Items	[]PortToNginx	 `json:"items"`
}

type ProtoToPort struct {
	core map[string]string
	lock sync.RWMutex
}

func newProtoToPort() *ProtoToPort {
	var p ProtoToPort
	p.core = make(map[string]string)
	return &p
}

func (p *ProtoToPort) Read(port string) (string, bool) {
	p.lock.RLock()
	defer p.lock.RUnlock()
	proto, ok := p.core[port]
	delete(p.core, port)
	return proto, ok
}

func (p *ProtoToPort) Write(proto string, port string) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.core[proto] = port
}

