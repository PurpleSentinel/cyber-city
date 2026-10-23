package pipeline

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/pcapgo"
)

// readFixture decodes every packet of testdata/pcap/<name>.pcap.
func readFixture(t testing.TB, name string) []Record {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "testdata", "pcap", name+".pcap"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	r, err := pcapgo.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	d := NewDecoder()
	var out []Record
	for {
		data, ci, err := r.ReadPacketData()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		if rec, ok := d.Decode(data, ci); ok {
			out = append(out, rec)
		}
	}
}

func TestDecodeTCPUsesHeaderLengths(t *testing.T) {
	recs := readFixture(t, "web_https")
	syn := recs[0]
	if syn.Proto != ProtoTCP || syn.Flags != FlagSYN || syn.DstPort != 443 || syn.Src.String() != "10.0.1.10" {
		t.Fatalf("first record %+v", syn)
	}
	var full int
	for _, r := range recs {
		if r.PayloadLen == mssForTest {
			full++
			if r.WireLen != 14+20+20+mssForTest {
				t.Fatalf("wire length %d for a full segment", r.WireLen)
			}
		}
	}
	if full == 0 {
		t.Fatal("no full-size segments: payload length must come from headers, not captured bytes")
	}
}

const mssForTest = 1448

func TestDecodeIPv6AndICMPv6(t *testing.T) {
	recs := readFixture(t, "ipv6")
	if recs[0].Proto != ProtoICMPv6 || !recs[0].Src.Is6() {
		t.Fatalf("first record %+v, want ICMPv6", recs[0])
	}
	var tcp6 bool
	for _, r := range recs {
		if r.Proto == ProtoTCP && r.Dst.String() == "2001:db8:ffff::1" {
			tcp6 = true
		}
	}
	if !tcp6 {
		t.Fatal("no IPv6 TCP records")
	}
}

func TestDecodeVLAN(t *testing.T) {
	seen := map[uint16]bool{}
	for _, r := range readFixture(t, "vlan") {
		seen[r.VLAN] = true
	}
	if !seen[10] || !seen[20] || seen[0] {
		t.Fatalf("VLANs %v, want exactly 10 and 20", seen)
	}
}

func TestDecodeUDPPayloadLen(t *testing.T) {
	r := readFixture(t, "dns")[0]
	if r.Proto != ProtoUDP || r.DstPort != 53 || r.PayloadLen != 40 {
		t.Fatalf("dns query %+v", r)
	}
}

func TestDecodeSkipsNonIP(t *testing.T) {
	if _, ok := NewDecoder().Decode([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 0x08, 0x06, 0}, gopacket.CaptureInfo{}); ok {
		t.Fatal("ARP-type frame decoded as IP")
	}
}

func TestIsLocal(t *testing.T) {
	for addr, want := range map[string]bool{
		"10.0.1.10": true, "192.168.1.1": true, "127.0.0.1": true, "169.254.1.1": true,
		"fd00:1::10": true, "fe80::1": true, "203.0.113.10": false, "2001:db8::1": false, "8.8.8.8": false,
	} {
		if got := IsLocal(mustAddr(addr)); got != want {
			t.Errorf("IsLocal(%s) = %v, want %v", addr, got, want)
		}
	}
}

func BenchmarkDecode(b *testing.B) {
	f, err := os.Open(filepath.Join("..", "..", "testdata", "pcap", "demo.pcap"))
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	r, err := pcapgo.NewReader(f)
	if err != nil {
		b.Fatal(err)
	}
	type frame struct {
		data []byte
		ci   gopacket.CaptureInfo
	}
	var frames []frame
	for {
		data, ci, err := r.ReadPacketData()
		if err != nil {
			break
		}
		frames = append(frames, frame{data, ci})
	}
	d := NewDecoder()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fr := frames[i%len(frames)]
		d.Decode(fr.data, fr.ci)
	}
}
