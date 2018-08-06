package ipvs

import (
	"fmt"
	"net"
	"strings"
	"strconv"
	"syscall"

	//"github.com/lvs-controller/pkg/config"

	"github.com/golang/glog"
	"github.com/pshi/libipvs"
)

const (
	providerName = "ipvsdr"
	vip          = "10.135.135.37"
)

type LvsManager struct {
	initialized bool
	name        string
	handle      libipvs.IPVSHandle
}

func NewLvsManager() *LvsManager {
	lm := &LvsManager{}
	return lm
}

func (lm *LvsManager) Init() {
	if lm.initialized {
		return
	}
	lm.initialized = true
	glog.Info("Initialize the ipvs provider ...")

	lm.name = "ipvs-provider"

	handle, err := libipvs.New()
	lm.handle = handle
	if err != nil {
		glog.Info("Failed to create new ipvs provider ... exited -1")
		panic(err)
	}
	if err := lm.handle.Flush(); err != nil {
		glog.Info("Failed to flush old ipvs hash table ... exited -1")
		panic(err)
	}
}

//###fixme the triggle should be port ???
func (lm *LvsManager) Sync(daddr, dport, proto string) error {
	if ok := checkArgs(daddr, dport); !ok {
		fmt.Printf("Failed, illegal addr&port args in add %v, %v\n", daddr, dport)
		return nil
	}

	//for test purpose
	vport, _ := strconv.Atoi(dport)
	if proto == "udp" {
		svc := libipvs.Service{
			Address:       net.ParseIP(vip),
			AddressFamily: syscall.AF_INET,
			Protocol:      libipvs.Protocol(syscall.IPPROTO_UDP),
			Port:          uint16(vport),
			SchedName:     libipvs.RoundRobin,
		}
		if _, err := lm.EnsureService(svc); err != nil {
			return fmt.Errorf("Failed to update tcp svc existed %v\n", err)
		}
		dst := libipvs.Destination{
			Address:       net.ParseIP(daddr),
			AddressFamily: syscall.AF_INET,
			Port:          uint16(vport),
		}
		if _, err := lm.EnsureDestination(svc, dst); err != nil {
			return fmt.Errorf("Failed to update dst in tcp svc existed %v\n", err)
		}

		return nil

	} else if proto == "tcp" {
		svc := libipvs.Service{
			Address:       net.ParseIP(vip),
			AddressFamily: syscall.AF_INET,
			Protocol:      libipvs.Protocol(syscall.IPPROTO_TCP),
			Port:          uint16(vport),
			SchedName:     libipvs.RoundRobin,
		}
		if _, err := lm.EnsureService(svc); err != nil {
			return fmt.Errorf("Failed to update udp svc existed %v\n", err)
		}

		dst := libipvs.Destination{
			Address:       net.ParseIP(daddr),
			AddressFamily: syscall.AF_INET,
			Port:          uint16(vport),
		}
		if _, err := lm.EnsureDestination(svc, dst); err != nil {
			return fmt.Errorf("Failed to update dst in udp svc existed %v\n", err)
		}

		return nil

	} else {
		return fmt.Errorf("Error, got proto type %+v\n", proto)
	}
	return nil
}

func (lm *LvsManager) Del(daddr, dport, proto string) error {
	if ok := checkArgs(daddr, dport); !ok {
		fmt.Printf("Failed, illegal addr&port args in del %v, %v\n", daddr, dport)
		return nil
	}
	//for test purpose
	vport, _ := strconv.Atoi(dport)

	switch proto {
	case "udp":
		{
			svc := libipvs.Service{
				Address:       net.ParseIP(vip),
				AddressFamily: syscall.AF_INET,
				Protocol:      libipvs.Protocol(syscall.IPPROTO_UDP),
				Port:          uint16(vport),
				SchedName:     libipvs.RoundRobin,
			}
			dst := libipvs.Destination{
				Address:       net.ParseIP(daddr),
				AddressFamily: syscall.AF_INET,
				Port:          uint16(vport),
			}
			if _, err := lm.DeleteDestination(svc, dst); err != nil {
				return fmt.Errorf("Failed to delete dst in udp svc existed %v\n", err)
			}

		}
	case "tcp":
		{
			svc := libipvs.Service{
				Address:       net.ParseIP(vip),
				AddressFamily: syscall.AF_INET,
				Protocol:      libipvs.Protocol(syscall.IPPROTO_TCP),
				Port:          uint16(vport),
				SchedName:     libipvs.RoundRobin,
			}
			dst := libipvs.Destination{
				Address:       net.ParseIP(daddr),
				AddressFamily: syscall.AF_INET,
				Port:          uint16(vport),
			}
			if _, err := lm.DeleteDestination(svc, dst); err != nil {
				return fmt.Errorf("Failed to delete dst in tcp svc existed %v\n", err)
			}
		}
	default:
		return fmt.Errorf("Failed, incorrect proto type in delete %v\n", proto)
	}
	return nil
}

//this is used to update tcp/udp --> udp/tcp only
func (lm *LvsManager) Update(daddr, dport, proto string) error {
	if ok := checkArgs(daddr, dport); !ok {
		fmt.Printf("Failed, illegal addr&port args in update %v, %v\n", daddr, dport)
		return nil
	}
	//for test purpose
	vport, _ := strconv.Atoi(dport)

	switch proto {
	case "udp":
		{
			svc := libipvs.Service{
				Address:       net.ParseIP(vip),
				AddressFamily: syscall.AF_INET,
				Protocol:      libipvs.Protocol(syscall.IPPROTO_TCP),
				Port:          uint16(vport),
				SchedName:     libipvs.RoundRobin,
			}
			if err := lm.handle.UpdateService(&svc); err != nil {
				return fmt.Errorf("Failed to update udp svc to tcp svc existed %v\n", err)
			}

		}
	case "tcp":
		{
			svc := libipvs.Service{
				Address:       net.ParseIP(vip),
				AddressFamily: syscall.AF_INET,
				Protocol:      libipvs.Protocol(syscall.IPPROTO_UDP),
				Port:          uint16(vport),
				SchedName:     libipvs.RoundRobin,
			}
			if err := lm.handle.UpdateService(&svc); err != nil {
				return fmt.Errorf("Failed to update tcp svc to udp svc existed %v\n", err)
			}
		}
	default:
		return fmt.Errorf("Failed, incorrect proto type in update %v\n", proto)
	}
	return nil
}

//this is used to FWDMethod and Weight
func (lm *LvsManager) UpdateDst(daddr, dport, proto string) error { return nil }

// functionallities
func (lm *LvsManager) EnsureService(svc libipvs.Service) (bool, error) {
	svcs, err := lm.handle.ListServices()
	if err != nil {
		return false, fmt.Errorf("Failed to list existed svc, err: %+v\n", err)
	}

	for _, s := range svcs {
		if s.Port == svc.Port && s.Protocol == svc.Protocol {
			return true, nil
		}
	}
	if err = lm.handle.NewService(&svc); err != nil {
		return false, fmt.Errorf("Failed to create new svc, err: %+v\n", err)
	}
	return false, nil
}

func (lm *LvsManager) EnsureDestination(svc libipvs.Service, dst libipvs.Destination) (bool, error) {
	dsts, err := lm.handle.ListDestinations(&svc)
	if err != nil {
		return false, fmt.Errorf("Failed to list existed dsts for svc %v, err: %+v\n", svc, err)
	}

	for _, d := range dsts {
		if d.Address.Equal(dst.Address) && d.Port == dst.Port {
			return true, nil
		}
	}

	if err = lm.handle.NewDestination(&svc, &dst); err != nil {
		return false, fmt.Errorf("Failed to add new dst %v in svc %v, err: %+v\n", dst, svc, err)
	}
	return false, nil
}

func (lm *LvsManager) DeleteDestination(svc libipvs.Service, dst libipvs.Destination) (bool, error) {
	dsts, err := lm.handle.ListDestinations(&svc)
	if err != nil {
		return false, fmt.Errorf("Failed to list existed dsts for svc %v, err: %+v\n", svc, err)
	}

	for _, d := range dsts {
		if d.Address.Equal(dst.Address) && d.Port == dst.Port {
			if err := lm.handle.DelDestination(&svc, &dst); err != nil {
				return false, fmt.Errorf("Failed to del dst %v in svc %v, err: %+v\n", dst, svc, err)
			}
		}
	}
	if _, err := lm.DeleteDeadSvc(svc); err != nil {
		return false, fmt.Errorf("Failed to clear %v\n", err)
	}
	return true, nil
}

func (lm *LvsManager) DeleteDeadSvc(svc libipvs.Service) (bool, error) {
	dsts, err := lm.handle.ListDestinations(&svc)
	if err != nil {
		return false, fmt.Errorf("Failed to list existed dsts for svc %v, err: %+v\n", svc, err)
	}
	if len(dsts) != 0 {
		if err := lm.handle.DelService(&svc); err != nil {
			return false, fmt.Errorf("Failed to clear dead svc %v, err: %+v\n", svc, err)
		}
	}
	return true, nil
}

func checkArgs(ipv4, port string) bool {

	parts := strings.Split(ipv4, ".")

	if len(parts) < 4 {
		return false
	}

	for _, x := range parts {
		if i, err := strconv.Atoi(x); err == nil {
			if i < 0 || i > 255 {
				return false
			}
		} else {
			return false
		}

	}

	if p, err := strconv.Atoi(port); err == nil {
		if p < 0 || p > 99999 {
			return false
		}
	} else {
		return false
	}

	return true
}
