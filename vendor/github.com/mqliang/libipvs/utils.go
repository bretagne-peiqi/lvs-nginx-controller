package libipvs

import (
	"fmt"
	"net"
	"syscall"

	"unsafe"

	"github.com/hkwi/nlgo"
)

// Helper to build an nlgo.Attr
func nlattr(typ uint16, value nlgo.NlaValue) nlgo.Attr {
	return nlgo.Attr{Header: syscall.NlAttr{Type: typ}, Value: value}
}

const sizeOfFlags = 0x8

func (f *Flags) unpack(value nlgo.Binary) error {
	if len(value) != sizeOfFlags {
		return fmt.Errorf("ipvs: unexpected flags=%v", value)
	}
	var buf [sizeOfFlags]byte
	copy(buf[:], value)
	tmp := (*Flags)(unsafe.Pointer(&buf))
	f.Flags = tmp.Flags
	f.Mask = tmp.Mask
	return nil
}

func (f *Flags) pack() nlgo.Binary {
	buf := make([]byte, sizeOfFlags)
	copy(buf, (*[sizeOfFlags]byte)(unsafe.Pointer(f))[:])
	return nlgo.Binary(buf)
}

// Helpers for net.IP <-> nlgo.Binary
func unpackAddr(value nlgo.Binary, af AddressFamily) (net.IP, error) {
	buf := ([]byte)(value)
	size := 0

	switch af {
	case syscall.AF_INET:
		size = 4
	case syscall.AF_INET6:
		size = 16
	default:
		return nil, fmt.Errorf("ipvs: unknown af=%d addr=%v", af, buf)
	}

	if size > len(buf) {
		return nil, fmt.Errorf("ipvs: short af=%d addr=%v", af, buf)
	}

	return (net.IP)(buf[:size]), nil
}

func packAddr(af AddressFamily, addr net.IP) nlgo.Binary {
	var ip net.IP

	switch af {
	case syscall.AF_INET:
		ip = addr.To4()
	case syscall.AF_INET6:
		ip = addr.To16()
	default:
		panic(fmt.Errorf("ipvs:packAddr: unknown af=%d addr=%v", af, addr))
	}

	if ip == nil {
		panic(fmt.Errorf("ipvs:packAddr: invalid af=%d addr=%v", af, addr))
	}

	return (nlgo.Binary)(ip)
}

// Helpers for uint16 port <-> nlgo.U16
func htons(value uint16) uint16 {
	return ((value & 0x00ff) << 8) | ((value & 0xff00) >> 8)
}
func ntohs(value uint16) uint16 {
	return ((value & 0x00ff) << 8) | ((value & 0xff00) >> 8)
}

func unpackPort(val nlgo.U16) uint16 {
	return ntohs((uint16)(val))
}
func packPort(port uint16) nlgo.U16 {
	return nlgo.U16(htons(port))
}

func unpackService(attrs nlgo.AttrMap) (Service, error) {
	var service Service

	var addr nlgo.Binary
	var flags nlgo.Binary

	for _, attr := range attrs.Slice() {
		switch attr.Field() {
		case IPVS_SVC_ATTR_AF:
			service.AddressFamily = (AddressFamily)(attr.Value.(nlgo.U16))
		case IPVS_SVC_ATTR_PROTOCOL:
			service.Protocol = (Protocol)(attr.Value.(nlgo.U16))
		case IPVS_SVC_ATTR_ADDR:
			addr = attr.Value.(nlgo.Binary)
		case IPVS_SVC_ATTR_PORT:
			service.Port = unpackPort(attr.Value.(nlgo.U16))
		case IPVS_SVC_ATTR_FWMARK:
			service.FWMark = (uint32)(attr.Value.(nlgo.U32))
		case IPVS_SVC_ATTR_SCHED_NAME:
			service.SchedName = (string)(attr.Value.(nlgo.NulString))
		case IPVS_SVC_ATTR_FLAGS:
			flags = attr.Value.(nlgo.Binary)
		case IPVS_SVC_ATTR_TIMEOUT:
			service.Timeout = (uint32)(attr.Value.(nlgo.U32))
		case IPVS_SVC_ATTR_NETMASK:
			service.Netmask = (uint32)(attr.Value.(nlgo.U32))
		case IPVS_SVC_ATTR_STATS:
			service.Stats = unpackStats(attr)
		}
	}

	// TOTE: ipvs Service with Fwmarks has no Address, so we just ignore the error
	if addrIP, err := unpackAddr(addr, service.AddressFamily); err != nil && service.FWMark == 0 {
		return service, fmt.Errorf("ipvs:Service.unpack: addr: %s", err)
	} else {
		service.Address = addrIP
	}

	if err := service.Flags.unpack(flags); err != nil {
		return service, fmt.Errorf("ipvs:Service.unpack: flags: %s", err)
	}

	return service, nil
}

func unpackDest(attrs nlgo.AttrMap, serverAddressFamily AddressFamily) (Destination, error) {
	var dest Destination
	var addr []byte

	for _, attr := range attrs.Slice() {
		switch attr.Field() {
		case IPVS_DEST_ATTR_ADDR_FAMILY:
			dest.AddressFamily = (AddressFamily)(attr.Value.(nlgo.U16))
		case IPVS_DEST_ATTR_ADDR:
			addr = ([]byte)(attr.Value.(nlgo.Binary))
		case IPVS_DEST_ATTR_PORT:
			dest.Port = unpackPort(attr.Value.(nlgo.U16))
		case IPVS_DEST_ATTR_FWD_METHOD:
			dest.FwdMethod = (FwdMethod)(attr.Value.(nlgo.U32))
		case IPVS_DEST_ATTR_WEIGHT:
			dest.Weight = (uint32)(attr.Value.(nlgo.U32))
		case IPVS_DEST_ATTR_U_THRESH:
			dest.UThresh = (uint32)(attr.Value.(nlgo.U32))
		case IPVS_DEST_ATTR_L_THRESH:
			dest.LThresh = (uint32)(attr.Value.(nlgo.U32))
		case IPVS_DEST_ATTR_ACTIVE_CONNS:
			dest.ActiveConns = (uint32)(attr.Value.(nlgo.U32))
		case IPVS_DEST_ATTR_INACT_CONNS:
			dest.InactConns = (uint32)(attr.Value.(nlgo.U32))
		case IPVS_DEST_ATTR_PERSIST_CONNS:
			dest.PersistConns = (uint32)(attr.Value.(nlgo.U32))
		case IPVS_DEST_ATTR_STATS:
			dest.Stats = unpackStats(attr)
		}
	}
	// Linux kernel prior v3.18-rc1 does not have the af (address family) field
	// in ip_vs_dest_user_kern, so use the address family in ip_vs_service_user_kern.
	// https://github.com/torvalds/linux/commit/6cff339bbd5f9eda7a5e8a521f91a88d046e6d0c
	if dest.AddressFamily == 0 {
		dest.AddressFamily = serverAddressFamily
	}

	if addrIP, err := unpackAddr(addr, dest.AddressFamily); err != nil {
		return dest, fmt.Errorf("ipvs:Dest.unpack: addr: %s", err)
	} else {
		dest.Address = addrIP
	}

	return dest, nil
}

func unpackInfo(attrs nlgo.AttrMap) (info Info, err error) {
	for _, attr := range attrs.Slice() {
		switch attr.Field() {
		case IPVS_INFO_ATTR_VERSION:
			info.Version = (Version)(attr.Value.(nlgo.U32))
		case IPVS_INFO_ATTR_CONN_TAB_SIZE:
			info.ConnTabSize = (uint32)(attr.Value.(nlgo.U32))
		}
	}

	return
}

func unpackStats(attrs nlgo.Attr) Stats {
	var stats Stats
	for _, attr := range attrs.Value.(nlgo.AttrMap).Slice() {
		switch attr.Field() {
		case IPVS_STATS_ATTR_CONNS:
			stats.Connections = (uint32)(attr.Value.(nlgo.U32))
		case IPVS_STATS_ATTR_INPKTS: /* incoming packets */
			stats.PacketsIn = (uint32)(attr.Value.(nlgo.U32))
		case IPVS_STATS_ATTR_OUTPKTS: /* outgoing packets */
			stats.PacketsOut = (uint32)(attr.Value.(nlgo.U32))
		case IPVS_STATS_ATTR_INBYTES: /* incoming bytes */
			stats.BytesIn = (uint64)(attr.Value.(nlgo.U64))
		case IPVS_STATS_ATTR_OUTBYTES: /* outgoing bytes */
			stats.BytesOut = (uint64)(attr.Value.(nlgo.U64))
		case IPVS_STATS_ATTR_CPS: /* current connection rate */
			stats.CPS = (uint32)(attr.Value.(nlgo.U32))
		case IPVS_STATS_ATTR_INPPS: /* current in packet rate */
			stats.PPSIn = (uint32)(attr.Value.(nlgo.U32))
		case IPVS_STATS_ATTR_OUTPPS: /* current out packet rate */
			stats.PPSOut = (uint32)(attr.Value.(nlgo.U32))
		case IPVS_STATS_ATTR_INBPS: /* current in byte rate */
			stats.BPSIn = (uint32)(attr.Value.(nlgo.U32))
		case IPVS_STATS_ATTR_OUTBPS: /* current out byte rate */
			stats.BPSOut = (uint32)(attr.Value.(nlgo.U32))
		}
	}

	return stats
}
