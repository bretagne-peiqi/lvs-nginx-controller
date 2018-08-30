package controller

import (
	"net"
	"reflect"
	"strings"
	"strconv"

	glog "github.com/zoumo/logdog"
)

const (
	errPort uint16 = 0
)

var L7 = []string{"80", "443"}
var errAddr = net.IP("0.0.0.0")

type LBStore struct {
	Addr map[string]net.IP //example {"adding-podName": ipv4, "deleting-podName": ipv4}
	Tcp  map[uint16]string //exapmle {"port": action} adding, deleting
	Udp  map[uint16]string //
}

func NewLBStore() *LBStore {
	lb := &LBStore{}

	lb.Addr = make(map[string]net.IP)
	lb.Tcp = make(map[uint16]string)
	lb.Udp = make(map[uint16]string)

	return lb
}

func (lb *LBStore) AddrGet() map[string]net.IP {
	return nil
}

func (lb *LBStore) AddrSet(addrs map[string]net.IP) error {
	return nil
}

func (lb *LBStore) L4Set(ports map[uint16]string) error {
	return nil
}
func (lb *LBStore) L4Get() map[uint16]string {
	return nil
}

func (lb *LBStore) L4Compare(oldOne, newOne, deltaOne map[uint16]string) {

	if reflect.DeepEqual(oldOne, newOne) {
		glog.Debugf("L4Comparing, oldOne %v equals newOne %v...\n", oldOne, newOne)
		return
	}

	for key, _ := range deltaOne {
		delete(deltaOne, key)
	}

	//FIXME(peiqishi) optimize the algorithm
	for port, _ := range newOne {
		if _, ok := oldOne[port]; !ok {
			//store the new key
			deltaOne[port] = addition
		}
	}

	for port, _ := range oldOne {
		if _, ok := newOne[port]; !ok {
			//need to delete the dead
			deltaOne[port] = deletion
		}
	}

	glog.Debugf("Replace oldOne %v and clearing newOne %v...\n", oldOne, newOne)
	for key, _ := range deltaOne {
		delete(oldOne, key)
	}
	for key, _ := range newOne {
		oldOne[key] = newOne[key]
		delete(newOne, key)
	}

	return
}

func ValidateIPv4(ipv4 string) (bool, net.IP) {

	parts := strings.Split(ipv4, ".")

	if len(parts) < 4 {
		return false, errAddr
	}

	for _, x := range parts {
		if i, err := strconv.Atoi(x); err == nil {
			if i < 0 || i > 255 {
				return false, errAddr
			}
		} else {
			return false, errAddr
		}

	}

	return true, net.IP(ipv4)

}

func ValidatePort(port string) (bool, uint16) {

	if p, err := strconv.Atoi(port); err == nil {
		if p < 0 || p > 99999 {
			return false, errPort
		}
	} else {
		return false, errPort
	}

	iport, err := strconv.Atoi(port)
	if err != nil {
		glog.Errorf("Failed to transit port to type uint16, err %v\n", err)
}
	return true, uint16(iport)
}
