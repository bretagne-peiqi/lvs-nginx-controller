package ipvs

import (
	"fmt"
	"net"
	"syscall"

	"github.com/lvs-controller/pkg/config"

	"github.com/pshi/libipvs"
	glog "github.com/zoumo/logdog"
)

const (
	providerName = "ipvsdr"

	TCP = "tcp"
	UDP = "udp"
)

type LvsManager struct {
	initialized bool
	name        string
	handle      libipvs.IPVSHandle
	vip         net.IP
	SchedName   string
}

func NewLvsManager() *LvsManager {
	lm := &LvsManager{}
	return lm
}

func (lm *LvsManager) Init(cfg config.Config) {

	if lm.initialized {
		return
	}

	lm.initialized = true
	glog.Info("Initialize the ipvs provider ...")

	lm.name = providerName
	lm.vip = net.ParseIP(cfg.Vip)
	lm.SchedName = cfg.SchedName

	handle, err := libipvs.New()
	lm.handle = handle
	if err != nil {
		glog.Error("Failed to create new ipvs provider ... exited")
	}

	if err := lm.handle.Flush(); err != nil {
		glog.Error("Failed to flush old ipvs hash table ... exited")
	}

}

func (lm *LvsManager) Base(daddr net.IP, dport uint16, proto string) (error, libipvs.Service, libipvs.Destination) {

	var protocol libipvs.Protocol
	var svc libipvs.Service
	var dst libipvs.Destination

	switch proto {
	case UDP:
		{
			protocol = libipvs.Protocol(syscall.IPPROTO_UDP)
		}
	case TCP:
		{
			protocol = libipvs.Protocol(syscall.IPPROTO_TCP)
		}
	default:
		return fmt.Errorf("Failed, unsupport protocol type %v, only tcp, udp supported \n", proto), svc, dst
	}

	svc = libipvs.Service{
		Address:       lm.vip,
		AddressFamily: syscall.AF_INET,
		Protocol:      protocol,
		Port:          dport,
		SchedName:     lm.SchedName,
	}

	dst = libipvs.Destination{
		Address:       net.ParseIP(string(daddr)),
		AddressFamily: syscall.AF_INET,
		Port:          dport,
	}

	return nil, svc, dst
}

func (lm *LvsManager) Add(daddr net.IP, dport uint16, proto string) error {

	err, svc, dst := lm.Base(daddr, dport, proto)
	if err != nil {
		return fmt.Errorf("Failed to build struct in Add svc dst %v\n", err)
	}

	err = lm.EnsureService(svc)
	if err != nil {
		return fmt.Errorf("Failed to update svc existed %v\n", err)
	}

	err = lm.EnsureDestination(svc, dst)
	if err != nil {
		glog.Warnf("Failed to add dst in udp svc existed %v\n", err)
	}

	return nil
}

func (lm *LvsManager) Del(daddr net.IP, dport uint16, proto string) error {

	err, svc, dst := lm.Base(daddr, dport, proto)
	if err != nil {
		return fmt.Errorf("Failed to build struct in Del svc dst %v\n", err)
	}

	err = lm.DeleteDestination(svc, dst)
	if err != nil {
		glog.Warnf("Failed to delete dst in svc existed %v\n", err)
	}

	return nil
}

//This is used to FWDMethod and Weight
func (lm *LvsManager) DelRServer(addr net.IP) error {

	saddr := string(addr)
	svcs, err := lm.handle.ListServices()
	if err != nil {
		return fmt.Errorf("Failed to list existed svc, err: %+v\n", err)
	}

	glog.Infof("going to delete addr %v\n", saddr)
	for _, s := range svcs {
		dsts, err := lm.handle.ListDestinations(s)
		if err != nil {
			return fmt.Errorf("Failed to list existed dsts for svc %v, err: %+v\n", s, err)
		}

		for _, d := range dsts {
			if d.Address.Equal(net.ParseIP(saddr)) {
				glog.Debugf("Equal found! going to delete addr %v\n", saddr)
				if err := lm.handle.DelDestination(s, d); err != nil {
					return fmt.Errorf("Failed to del dst %v in svc %v, err: %+v\n", d, s, err)
				}
			}
		}

		dsts, err = lm.handle.ListDestinations(s)
		if err != nil {
			return fmt.Errorf("After operate, Failed to list existed dsts for svc %v, err: %+v\n", s, err)
		}

		if len(dsts) == 0 {
			if err := lm.handle.DelService(s); err != nil {
				return fmt.Errorf("After, operate, Failed to clear dead svc %v, err: %+v\n", s, err)
			}
		}
	}

	return nil
}

func (lm *LvsManager) EnsureService(svc libipvs.Service) error {

	svcs, err := lm.handle.ListServices()
	if err != nil {
		return fmt.Errorf("Failed to list existed svc, err: %+v\n", err)
	}

	for _, s := range svcs {
		if s.Port == svc.Port && s.Protocol == svc.Protocol {
			glog.Debug("svc already existed ...")
			return nil
		}
	}

	if err = lm.handle.NewService(&svc); err != nil {
		return fmt.Errorf("Failed to create new svc, err: %+v\n", err)
	}

	return nil
}

func (lm *LvsManager) EnsureDestination(svc libipvs.Service, dst libipvs.Destination) error {

	dsts, err := lm.handle.ListDestinations(&svc)
	if err != nil {
		return fmt.Errorf("Failed to list existed dsts for svc %v, err: %+v\n", svc, err)
	}

	for _, d := range dsts {
		if d.Address.Equal(dst.Address) && d.Port == dst.Port {
			return nil
		}
	}

	if err = lm.handle.NewDestination(&svc, &dst); err != nil {
		return fmt.Errorf("Failed to add new dst %v in svc %v, err: %+v\n", dst, svc, err)
	}

	return nil
}

func (lm *LvsManager) DeleteDestination(svc libipvs.Service, dst libipvs.Destination) error {

	if err := lm.handle.DelDestination(&svc, &dst); err != nil {
		return fmt.Errorf("Failed to del dst %v in svc %v, err: %+v\n", dst, svc, err)
	}

	if err := lm.DeleteDeadSvc(svc); err != nil {
		return fmt.Errorf("Failed to clear %v\n", err)
	}

	return nil
}

func (lm *LvsManager) DeleteDeadSvc(svc libipvs.Service) error {

	dsts, err := lm.handle.ListDestinations(&svc)
	if err != nil {
		return fmt.Errorf("Failed to list existed dsts for svc %v, err: %+v\n", svc, err)
	}

	if len(dsts) == 0 {
		if err := lm.handle.DelService(&svc); err != nil {
			return fmt.Errorf("Failed to clear dead svc %v, err: %+v\n", svc, err)
		}
	}

	return nil
}
