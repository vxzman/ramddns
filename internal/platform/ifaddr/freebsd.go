//go:build freebsd

package ifaddr

import (
	"encoding/binary"
	"errors"
	"net"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Netlink constants — FreeBSD values.
// FreeBSD 14+ netlink is wire-compatible with Linux netlink.
const (
	afNetlink    = 38
	netlinkRoute = 0

	// Netlink message types
	nlmsgDone  = 3
	nlmsgError = 2

	// Netlink message flags
	nlmFRequest = 0x01
	nlmFMulti   = 0x02
	nlmFDump    = 0x100

	// Route message types
	rtmNewAddr = 0x14
	rtmGetAddr = 0x16

	// Address attributes (same as Linux IFA_*)
	ifaAddress    = 1
	ifaLocal      = 2
	attrCacheinfo = 6 // IFA_CACHEINFO attribute type

	nd6InfiniteLifetime = 0xffffffff

	// Structure sizes on the wire
	sizeofNlMsghdr      = 16
	sizeofIfAddrmsg     = 8
	sizeofIfaCacheinfo  = 16
	sizeofSockaddrNl    = 12

	nlmsgAlignTo = 4
)

// --- Netlink wire structures (little-endian, same as Linux) ---

type nlMsghdr struct {
	Len   uint32 // total message length including header
	Type  uint16 // message type, e.g. RTM_NEWADDR
	Flags uint16 // NLM_F_*
	Seq   uint32 // sequence number
	Pid   uint32 // sender port ID
}

type ifAddrmsg struct {
	Family    uint8
	Prefixlen uint8
	Flags     uint8
	Scope     uint8
	Index     uint32
}

type ifaCacheinfo struct {
	Prefered uint32
	Valid    uint32
	Cstamp   uint32
	Tstamp   uint32
}

// sockaddrNl is the BSD sockaddr_nl (12 bytes).
// Layout: len(u8) + family(u8) + pad(2) + pid(u32) + groups(u32).
type sockaddrNl struct {
	Len    uint8
	Family uint8
	Pad    [2]byte
	Pid    uint32
	Groups uint32
}

// nlmsgAlign rounds up to the nearest NLMSG_ALIGN boundary.
func nlmsgAlign(n int) int {
	return (n + nlmsgAlignTo - 1) &^ (nlmsgAlignTo - 1)
}

// parsedAddr holds a single address extracted from a netlink RTM_NEWADDR message.
type parsedAddr struct {
	Family    uint8
	Prefixlen uint8
	Index     uint32
	IP        net.IP          // from IFA_ADDRESS or IFA_LOCAL
	Cacheinfo *ifaCacheinfo   // from IFA_CACHEINFO
}

// --- Public API ---

// GetAvailableIPv6 returns IPv6 addresses and lifetimes from the named
// interface using the Netlink RTM_GETADDR dump — the same path ifconfig
// uses on FreeBSD 14+.
func GetAvailableIPv6(ifaceName string) ([]IPv6Info, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, errors.New("interface not found or inaccessible")
	}
	ifaceIndex := uint32(iface.Index)

	// 1. Open netlink socket.
	fd, err := unix.Socket(afNetlink, unix.SOCK_RAW, netlinkRoute)
	if err != nil {
		return nil, errors.New("system error while querying interface addresses")
	}
	defer unix.Close(fd)

	// 2. Bind the socket.
	pid := uint32(unix.Getpid())
	lsa := sockaddrNl{Len: sizeofSockaddrNl, Family: afNetlink, Pid: pid}
	_, _, errno := unix.Syscall(
		unix.SYS_BIND, uintptr(fd),
		uintptr(unsafe.Pointer(&lsa)),
		sizeofSockaddrNl,
	)
	if errno != 0 {
		return nil, errors.New("system error while querying interface addresses")
	}

	// 3. Build RTM_GETADDR dump request.
	seq := uint32(1)
	nlh := nlMsghdr{
		Len:   sizeofNlMsghdr + sizeofIfAddrmsg,
		Type:  rtmGetAddr,
		Flags: nlmFRequest | nlmFDump,
		Seq:   seq,
		Pid:   pid,
	}
	ifam := ifAddrmsg{Family: unix.AF_INET6}

	req := make([]byte, nlmsgAlign(int(nlh.Len)))
	putNlMsghdr(req, &nlh)
	putIfAddrmsg(req[sizeofNlMsghdr:], &ifam)

	// 4. Send request.
	sa := sockaddrNl{Len: sizeofSockaddrNl, Family: afNetlink}
	_, _, errno = unix.Syscall6(
		unix.SYS_SENDTO, uintptr(fd),
		uintptr(unsafe.Pointer(&req[0])), uintptr(len(req)),
		0, // flags
		uintptr(unsafe.Pointer(&sa)), sizeofSockaddrNl,
	)
	if errno != 0 {
		return nil, errors.New("system error while querying interface addresses")
	}

	// 5. Receive multi-part reply.
	buf := make([]byte, 32768)
	var infos []IPv6Info

	for {
		n, _, errno := unix.Syscall6(
			unix.SYS_RECVFROM, uintptr(fd),
			uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)),
			0, // flags
			0, 0, // from / fromlen (NULL)
		)
		if errno != 0 || n == 0 {
			break
		}

		addrs := parseAddrs(buf[:n])
		for _, a := range addrs {
			// Filter by interface.
			if a.Index != ifaceIndex {
				continue
			}
			if a.IP == nil || a.IP.IsLinkLocalUnicast() {
				continue
			}

			var pltime uint32 = nd6InfiniteLifetime
			var vltime uint32 = nd6InfiniteLifetime

			if a.Cacheinfo != nil {
				ci := a.Cacheinfo
				// 0 and ND6_INFINITE_LIFETIME both mean "forever".
				// Other values are remaining seconds (cf. tutorial direct mode).
				if ci.Prefered != 0 && ci.Prefered != nd6InfiniteLifetime {
					pltime = ci.Prefered
				}
				if ci.Valid != 0 && ci.Valid != nd6InfiniteLifetime {
					vltime = ci.Valid
				}
			}

			// Map sentinel to a long duration so PopulateInfo treats the
			// address as Preferred/Static.
			if pltime == nd6InfiniteLifetime {
				pltime = uint32((365 * 10 * 24 * time.Hour).Seconds())
			}
			if vltime == nd6InfiniteLifetime {
				vltime = uint32((365 * 10 * 24 * time.Hour).Seconds())
			}

			info := IPv6Info{
				IP:           a.IP,
				PreferredLft: time.Duration(pltime) * time.Second,
				ValidLft:     time.Duration(vltime) * time.Second,
			}
			PopulateInfo(&info)
			infos = append(infos, info)
		}

		// Check for NLMSG_DONE in the batch (end of multi-part dump).
		if hasDone(buf[:n], seq) {
			break
		}
	}

	if len(infos) == 0 {
		return nil, errors.New("no global IPv6 address found on interface")
	}
	return infos, nil
}

// --- Serialization helpers ---

func putNlMsghdr(b []byte, h *nlMsghdr) {
	binary.LittleEndian.PutUint32(b[0:4], h.Len)
	binary.LittleEndian.PutUint16(b[4:6], h.Type)
	binary.LittleEndian.PutUint16(b[6:8], h.Flags)
	binary.LittleEndian.PutUint32(b[8:12], h.Seq)
	binary.LittleEndian.PutUint32(b[12:16], h.Pid)
}

func putIfAddrmsg(b []byte, m *ifAddrmsg) {
	b[0] = m.Family
	b[1] = m.Prefixlen
	b[2] = m.Flags
	b[3] = m.Scope
	binary.LittleEndian.PutUint32(b[4:8], m.Index)
}

// --- Parsing ---

// parseAddrs extracts all RTM_NEWADDR entries from a raw netlink payload.
func parseAddrs(data []byte) []parsedAddr {
	var addrs []parsedAddr

	for len(data) >= sizeofNlMsghdr {
		h := nlMsghdr{
			Len:   binary.LittleEndian.Uint32(data[0:4]),
			Type:  binary.LittleEndian.Uint16(data[4:6]),
			Flags: binary.LittleEndian.Uint16(data[6:8]),
			Seq:   binary.LittleEndian.Uint32(data[8:12]),
			Pid:   binary.LittleEndian.Uint32(data[12:16]),
		}

		if h.Len < sizeofNlMsghdr || int(h.Len) > len(data) {
			break
		}

		msgEnd := nlmsgAlign(int(h.Len))
		if msgEnd > len(data) {
			msgEnd = len(data)
		}

		if h.Type == rtmNewAddr {
			payload := data[sizeofNlMsghdr:h.Len]
			if len(payload) >= sizeofIfAddrmsg {
				a := parsedAddr{
					Family:    payload[0],
					Prefixlen: payload[1],
					Index:     binary.LittleEndian.Uint32(payload[4:8]),
				}
				parseAddrAttrs(payload[sizeofIfAddrmsg:], &a)
				addrs = append(addrs, a)
			}
		}

		// Also check for NLMSG_DONE/NLMSG_ERROR at this level.
		// (parseAddrs doesn't terminate the loop — caller handles that.)

		data = data[msgEnd:]
	}
	return addrs
}

// parseAddrAttrs walks the rtattr list after ifAddrmsg.
func parseAddrAttrs(b []byte, a *parsedAddr) {
	for len(b) >= 4 {
		attrLen := binary.LittleEndian.Uint16(b[0:2])
		attrType := binary.LittleEndian.Uint16(b[2:4])
		if attrLen < 4 || int(attrLen) > len(b) {
			break
		}

		data := b[4:attrLen]

		switch attrType {
		case ifaAddress, ifaLocal:
			if len(data) >= 16 && a.IP == nil {
				ip := make(net.IP, 16)
				copy(ip, data[:16])
				a.IP = ip
			}
		case attrCacheinfo:
			if len(data) >= sizeofIfaCacheinfo {
				a.Cacheinfo = &ifaCacheinfo{
					Prefered: binary.LittleEndian.Uint32(data[0:4]),
					Valid:    binary.LittleEndian.Uint32(data[4:8]),
					Cstamp:   binary.LittleEndian.Uint32(data[8:12]),
					Tstamp:   binary.LittleEndian.Uint32(data[12:16]),
				}
			}
		}

		// Advance to next attribute (RTA_ALIGN).
		advance := nlmsgAlign(int(attrLen))
		b = b[advance:]
	}
}

// hasDone checks whether a raw netlink payload contains NLMSG_DONE.
func hasDone(data []byte, seq uint32) bool {
	for len(data) >= sizeofNlMsghdr {
		h := nlMsghdr{
			Len:   binary.LittleEndian.Uint32(data[0:4]),
			Type:  binary.LittleEndian.Uint16(data[4:6]),
			Flags: binary.LittleEndian.Uint16(data[6:8]),
			Seq:   binary.LittleEndian.Uint32(data[8:12]),
			Pid:   binary.LittleEndian.Uint32(data[12:16]),
		}
		if h.Len < sizeofNlMsghdr || int(h.Len) > len(data) {
			break
		}
		if h.Type == nlmsgDone && h.Seq == seq {
			return true
		}
		advance := nlmsgAlign(int(h.Len))
		if advance > len(data) {
			break
		}
		data = data[advance:]
	}
	return false
}
