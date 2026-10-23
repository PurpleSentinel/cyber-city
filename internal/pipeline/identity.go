package pipeline

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/netip"

	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

// identity maps addresses to the ids sent on the wire. With anonymize, host
// ids are "anon-" plus the first 8 hex digits of HMAC-SHA256(key, ip) and auto
// districts are numbered in order of first sight (docs/PROTOCOL.md).
type identity struct {
	anonymize bool
	key       []byte
	districts map[string]protocol.District // auto district by real prefix id
}

func newIdentity(anonymize bool, key []byte) *identity {
	if anonymize && key == nil {
		key = make([]byte, 32)
		_, _ = rand.Read(key) // never fails since Go 1.24
	}
	return &identity{anonymize: anonymize, key: key, districts: make(map[string]protocol.District)}
}

// hostID returns the wire id of a host address. Group addresses are public
// constants, not hosts, and are never pseudonymised.
func (id *identity) hostID(a netip.Addr) string {
	if !id.anonymize || isGroup(a) {
		return a.String()
	}
	mac := hmac.New(sha256.New, id.key)
	mac.Write(a.AsSlice())
	return "anon-" + hex.EncodeToString(mac.Sum(nil))[:8]
}

// hostIP returns the ip field: the address, or nil when anonymised.
func (id *identity) hostIP(a netip.Addr) *string {
	if id.anonymize {
		return nil
	}
	return protocol.Ptr(a.String())
}

// district returns the auto district for a local address and whether it is new.
func (id *identity) district(a netip.Addr) (protocol.District, bool) {
	actual := autoDistrict(a)
	if d, ok := id.districts[actual.ID]; ok {
		return d, false
	}
	d := actual
	if id.anonymize {
		n := len(id.districts) + 1
		d = protocol.District{ID: fmt.Sprintf("lan-%d", n), Name: fmt.Sprintf("LAN %d", n), Kind: protocol.KindLAN, CIDRs: []string{}}
	}
	id.districts[actual.ID] = d
	return d, true
}
