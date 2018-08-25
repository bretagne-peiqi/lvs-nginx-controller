package controller

import (
	"net"
	"sync"
)

const (
	L7 = []string{"80", "443"}
)

type AddrStore struct {
	//Lock   sync.RWMutex
	Name   string
	Addr   net.IP
	Action string
}

var Ports []string

type PortStore struct {
	Lock sync.RWMutex
	Name string
	L4   map[string]Ports // {"tcp-services":{80, 443, 53 ...}; "udp-services": {53, 553 ...}}
}

type LBStore struct {
	lock			  sync.RWMutex
	portStore         PortStore
	addrStore         map[string]AddrStore
}

var LBQueue <-chan LBStore

func NewLBStore() *LBStore {
	lb := &LBStore{}

	lb.addrStore = make(map[string]AddrStore)

	ports := make([]string)
	lb.portStore.L4 := make(map[string]ports)

	return &lb
}

func (lb *LBStore) Init() error {}

func (lb *LBStore) L4Set(port PortStore) error {
	port.Lock.Lock()
	defer port.Lock.Unlock()

	lb.lock.Lock()
	defer lb.Lock.Unlock()

	switch port.Name {
	case "del-tcp-services":
		lb.portStore.L4["tcp-services"] = port.L4["tcp-services"]
	case "del-udp-services":
		lb.portStore.L4["udp-services"] = port.L4["udp-services"]
	case "add-tcp-services":
		lb.portStore.L4["tcp-services"] = port.L4["tcp-services"]
	case "add-udp-services":
		lb.portStore.L4["udp-services"] = port.L4["udp-services"]
	}

	lb.portStore.Name = port.Name

	return nil
}

func (lb *LBStore) L4Get(proto string) ([]string, string, bool) {
	lb.lock.Lock()
	defer lb.Lock.Unlock()

	if !lb.portStore.L4[proto] {
		return nil,"", false
	}

	ports := lb.portStore.L4[proto]
	name := lb.portStore.Name

	lb.portStore.L4[proto] = make([]string)
	lb.portStore.Name = ""

	return ports, name, true
}

func (lb *LBStore) AddrSet(addr AddrStore) error {
	lb.lock.Lock()
	defer lb.Lock.Unlock()

	lb.addrStore[addr.Name] = addr

	return nil
}

func (lb *LBStore) AddrGet() (map[net.IP]string, bool) {
	lb.lock.Lock()
	defer lb.Lock.Unlock()

	addrs := make(map[net.IP]string)

	if len(lb.addrStore) == nil {
		return nil, false
	}

	for key, addr := range lb.addrStore {
		addrs[addr.Addr] = addr.Action
		delete(lb.addrStore, key)
	}

	return addrs, true
}
