// SPDX-License-Identifier: MIT

package enrich

import (
	"container/list"
	"net/netip"
	"time"
)

// Retrans detects TCP retransmissions passively (docs/CONFIG.md
// "Retransmission"): per direction it tracks the highest seq+len seen; a data
// segment that ends at or below it is a retransmission. Per-host rates are
// kept over a sliding window in one-second buckets.
type Retrans struct {
	window time.Duration
	max    int

	dirs  map[Direction]*dirState
	order *list.List // directions, least recently seen at the back
	hosts map[netip.Addr][]bucket
}

// Direction is one side of a TCP connection.
type Direction struct {
	From, To netip.AddrPort
}

type dirState struct {
	highest uint32
	elem    *list.Element
}

type bucket struct {
	sec            int64
	total, retrans uint32
}

// NewRetrans keeps rates over window (enrich.retrans_window) and at most
// maxDirs directions of sequence state.
func NewRetrans(window time.Duration, maxDirs int) *Retrans {
	return &Retrans{window: window, max: maxDirs, dirs: make(map[Direction]*dirState), order: list.New(), hosts: make(map[netip.Addr][]bucket)}
}

// seqAfter reports a > b in 32-bit serial-number arithmetic (RFC 1982).
func seqAfter(a, b uint32) bool { return int32(a-b) > 0 } //nolint:gosec // intentional wrap

// Segment records a TCP data segment from d.From and reports whether it is a
// retransmission. Segments without payload are ignored.
func (r *Retrans) Segment(d Direction, seq uint32, payload int, ts time.Time) bool {
	if payload <= 0 {
		return false
	}
	end := seq + uint32(payload) //nolint:gosec // payload fits a TCP segment
	st := r.dirs[d]
	retrans := false
	if st == nil {
		st = &dirState{highest: end, elem: r.order.PushFront(d)}
		r.dirs[d] = st
		for len(r.dirs) > r.max {
			oldest := r.order.Back()
			delete(r.dirs, oldest.Value.(Direction))
			r.order.Remove(oldest)
		}
	} else {
		r.order.MoveToFront(st.elem)
		if seqAfter(end, st.highest) {
			st.highest = end
		} else {
			retrans = true
		}
	}
	r.count(d.From.Addr(), ts, retrans)
	return retrans
}

func (r *Retrans) count(host netip.Addr, ts time.Time, retrans bool) {
	sec := ts.Unix()
	bs := r.hosts[host]
	if n := len(bs); n == 0 || bs[n-1].sec != sec {
		bs = append(bs, bucket{sec: sec})
	}
	b := &bs[len(bs)-1]
	b.total++
	if retrans {
		b.retrans++
	}
	r.hosts[host] = r.trim(bs, sec)
}

// trim drops buckets older than the window ending at sec.
func (r *Retrans) trim(bs []bucket, sec int64) []bucket {
	oldest := sec - int64(r.window/time.Second) + 1
	i := 0
	for i < len(bs) && bs[i].sec < oldest {
		i++
	}
	return bs[i:]
}

// Rate returns host's retransmitted / total data segments over the window
// ending at now.
func (r *Retrans) Rate(host netip.Addr, now time.Time) float64 {
	var total, retrans uint32
	for _, b := range r.trim(r.hosts[host], now.Unix()) {
		total += b.total
		retrans += b.retrans
	}
	if total == 0 {
		return 0
	}
	return float64(retrans) / float64(total)
}

// Forget drops a removed host's rate state.
func (r *Retrans) Forget(host netip.Addr) { delete(r.hosts, host) }
