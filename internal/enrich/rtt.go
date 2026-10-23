// SPDX-License-Identifier: MIT

package enrich

import (
	"container/list"
	"net/netip"
	"slices"
	"time"
)

// Conn identifies a TCP connection from the client's side.
type Conn struct {
	Client, Server netip.AddrPort
}

// RTT measures handshake round-trip times passively (docs/CONFIG.md
// "Handshake RTT"): rtt = client's first ACK − client's SYN, which is the full
// client↔server round trip as seen from the sensor, wherever it sits.
type RTT struct {
	timeout time.Duration
	max     int
	samples int

	pending map[Conn]*handshake
	order   *list.List // pending connections, oldest at the back
	hosts   map[netip.Addr][]float64
}

type handshake struct {
	syn    time.Time
	synAck bool
	resent bool // SYN retransmitted: timing is ambiguous
	elem   *list.Element
}

// NewRTT keeps pending handshakes for timeout (enrich.handshake_timeout), at
// most maxPending of them, and medians over the last samples per host.
func NewRTT(timeout time.Duration, maxPending, samples int) *RTT {
	return &RTT{
		timeout: timeout, max: maxPending, samples: samples,
		pending: make(map[Conn]*handshake), order: list.New(), hosts: make(map[netip.Addr][]float64),
	}
}

// SYN records a client SYN (no ACK).
func (r *RTT) SYN(c Conn, ts time.Time) {
	r.expire(ts)
	if h, ok := r.pending[c]; ok {
		h.resent = true
		return
	}
	h := &handshake{syn: ts}
	h.elem = r.order.PushFront(c)
	r.pending[c] = h
	for len(r.pending) > r.max {
		oldest := r.order.Back()
		delete(r.pending, oldest.Value.(Conn))
		r.order.Remove(oldest)
	}
}

// SYNACK records the server's SYN-ACK.
func (r *RTT) SYNACK(c Conn) {
	if h, ok := r.pending[c]; ok {
		h.synAck = true
	}
}

// ACK handles a client ACK without SYN. It returns the RTT in milliseconds
// when it completes a clean handshake.
func (r *RTT) ACK(c Conn, ts time.Time) (float64, bool) {
	h, ok := r.pending[c]
	if !ok || !h.synAck {
		return 0, false
	}
	delete(r.pending, c)
	r.order.Remove(h.elem)
	if h.resent || ts.Sub(h.syn) > r.timeout {
		return 0, false
	}
	ms := float64(ts.Sub(h.syn)) / float64(time.Millisecond)
	for _, host := range []netip.Addr{c.Client.Addr(), c.Server.Addr()} {
		s := append(r.hosts[host], ms)
		if len(s) > r.samples {
			s = s[len(s)-r.samples:]
		}
		r.hosts[host] = s
	}
	return ms, true
}

// expire discards handshakes older than the timeout.
func (r *RTT) expire(now time.Time) {
	for e := r.order.Back(); e != nil; e = r.order.Back() {
		c := e.Value.(Conn)
		if now.Sub(r.pending[c].syn) <= r.timeout {
			return
		}
		delete(r.pending, c)
		r.order.Remove(e)
	}
}

// Pending returns the number of handshakes awaiting completion.
func (r *RTT) Pending() int { return len(r.pending) }

// HostMedian returns the median of host's recent handshake RTTs.
func (r *RTT) HostMedian(host netip.Addr) (float64, bool) {
	s := r.hosts[host]
	if len(s) == 0 {
		return 0, false
	}
	sorted := slices.Sorted(slices.Values(s))
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid], true
	}
	return (sorted[mid-1] + sorted[mid]) / 2, true
}

// Forget drops a removed host's samples.
func (r *RTT) Forget(host netip.Addr) { delete(r.hosts, host) }
