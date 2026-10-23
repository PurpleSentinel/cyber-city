// SPDX-License-Identifier: MIT

package detect

import (
	"net/netip"
	"time"

	"github.com/PurpleSentinel/cyber-city/internal/protocol"
)

// HostIdle is the event for a local host that went silent (docs/CONFIG.md
// "Host idle"); its removal is reported in the same tick.
func HostIdle(host netip.Addr, idle time.Duration) Candidate {
	return Candidate{
		Kind:     protocol.EventHostIdle,
		Key:      host.String(),
		Src:      &host,
		Evidence: map[string]float64{"idle_s": idle.Seconds()},
		Severity: protocol.SeverityInfo,
		Duration: 5 * time.Second,
	}
}
