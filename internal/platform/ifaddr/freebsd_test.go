//go:build freebsd

package ifaddr

import (
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func TestNlmsgAlign(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{0, 0},
		{1, 4},
		{4, 4},
		{5, 8},
		{16, 16},
		{17, 20},
	}
	for _, tt := range tests {
		got := nlmsgAlign(tt.input)
		if got != tt.want {
			t.Errorf("nlmsgAlign(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestPutNlMsghdr(t *testing.T) {
	b := make([]byte, sizeofNlMsghdr)
	h := nlMsghdr{
		Len:   24,
		Type:  rtmGetAddr,
		Flags: nlmFRequest | nlmFDump,
		Seq:   42,
		Pid:   1234,
	}
	putNlMsghdr(b, &h)

	if binary.LittleEndian.Uint32(b[0:4]) != 24 {
		t.Errorf("Len = %d, want 24", binary.LittleEndian.Uint32(b[0:4]))
	}
	if binary.LittleEndian.Uint16(b[4:6]) != rtmGetAddr {
		t.Errorf("Type = %d, want %d", binary.LittleEndian.Uint16(b[4:6]), rtmGetAddr)
	}
	if binary.LittleEndian.Uint16(b[6:8]) != (nlmFRequest | nlmFDump) {
		t.Errorf("Flags = %d, want %d", binary.LittleEndian.Uint16(b[6:8]), nlmFRequest|nlmFDump)
	}
	if binary.LittleEndian.Uint32(b[8:12]) != 42 {
		t.Errorf("Seq = %d, want 42", binary.LittleEndian.Uint32(b[8:12]))
	}
	if binary.LittleEndian.Uint32(b[12:16]) != 1234 {
		t.Errorf("Pid = %d, want 1234", binary.LittleEndian.Uint32(b[12:16]))
	}
}

func TestParseAddrAttrs(t *testing.T) {
	var buf []byte

	addAttr := func(attrType uint16, data []byte) {
		attrLen := uint16(4 + len(data))
		hdr := make([]byte, 4)
		binary.LittleEndian.PutUint16(hdr[0:2], attrLen)
		binary.LittleEndian.PutUint16(hdr[2:4], attrType)
		buf = append(buf, hdr...)
		buf = append(buf, data...)
		for len(buf)%4 != 0 {
			buf = append(buf, 0)
		}
	}

	addrIP := net.ParseIP("2001:db8::1").To16()
	localIP := net.ParseIP("2001:db8::2").To16()

	addAttr(ifaAddress, addrIP)
	addAttr(ifaLocal, localIP)

	ciBytes := make([]byte, 16)
	binary.LittleEndian.PutUint32(ciBytes[0:4], 3600)  // Prefered
	binary.LittleEndian.PutUint32(ciBytes[4:8], 7200)  // Valid
	binary.LittleEndian.PutUint32(ciBytes[8:12], 100)  // Cstamp
	binary.LittleEndian.PutUint32(ciBytes[12:16], 200) // Tstamp
	addAttr(attrCacheinfo, ciBytes)

	var parsed parsedAddr
	parseAddrAttrs(buf, &parsed)

	if !parsed.Address.Equal(net.IP(addrIP)) {
		t.Errorf("Address = %v, want %v", parsed.Address, addrIP)
	}
	if !parsed.Local.Equal(net.IP(localIP)) {
		t.Errorf("Local = %v, want %v", parsed.Local, localIP)
	}
	if parsed.Cacheinfo == nil {
		t.Fatalf("Cacheinfo is nil")
	}
	if parsed.Cacheinfo.Prefered != 3600 {
		t.Errorf("Prefered = %d, want 3600", parsed.Cacheinfo.Prefered)
	}
	if parsed.Cacheinfo.Valid != 7200 {
		t.Errorf("Valid = %d, want 7200", parsed.Cacheinfo.Valid)
	}
}

func TestNlmFDump(t *testing.T) {
	// FreeBSD NLM_F_DUMP is NLM_F_ROOT|NLM_F_MATCH, not NLM_F_ROOT alone.
	if nlmFDump != 0x300 {
		t.Fatalf("NLM_F_DUMP = %#x, want 0x300", nlmFDump)
	}
	if nlmFRequest|nlmFDump != 0x301 {
		t.Fatalf("request flags = %#x, want 0x301", nlmFRequest|nlmFDump)
	}
}

func TestCacheinfoLifetimes(t *testing.T) {
	pref, valid := cacheinfoLifetimes(nil)
	if pref != InfiniteLifetime || valid != InfiniteLifetime {
		t.Fatalf("nil cacheinfo = %v %v, want infinite", pref, valid)
	}

	pref, valid = cacheinfoLifetimes(&ifaCacheinfo{Prefered: nd6InfiniteLifetime, Valid: nd6InfiniteLifetime})
	if pref != InfiniteLifetime || valid != InfiniteLifetime {
		t.Fatalf("0xffffffff = %v %v, want infinite", pref, valid)
	}

	pref, valid = cacheinfoLifetimes(&ifaCacheinfo{Prefered: 0, Valid: 7200})
	if pref != 0 || valid != 7200*time.Second {
		t.Fatalf("prefered 0 = %v %v, want 0 and 7200s", pref, valid)
	}

	pref, valid = cacheinfoLifetimes(&ifaCacheinfo{Prefered: 53196, Valid: 139596})
	if pref != 53196*time.Second || valid != 139596*time.Second {
		t.Fatalf("finite = %v %v", pref, valid)
	}
}

func TestParseAddrAttrsUnalignedTail(t *testing.T) {
	// Last attribute has no trailing pad. Must not panic.
	data := []byte{5, 0, 1, 0, 0x20}
	var parsed parsedAddr
	parseAddrAttrs(data, &parsed)
}
