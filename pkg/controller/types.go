package controller

import (
	"encoding/json"
	"sync"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PortToNginx struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        v1.ObjectMeta `json:"metadata"`

	Port			string	`json:"port"` //udp&tcp port
	Proto			string  `json:"proto"`
	Nginx		    string  `json:"nginxIpv4"`
	ServiceName	    string	`json:"serviceName, omitempty"`
}

type PortToNginxList  struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ListMeta `json:"metadata"`

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

// note {port: "action-proto"}
