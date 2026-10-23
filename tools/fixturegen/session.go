// SPDX-License-Identifier: MIT

package main

import "time"

const mss = 1448

// conn is a TCP connection between client c and server s.
type conn struct {
	c, s         host
	cport, sport uint16
	cseq, sseq   uint32 // next sequence number each side will send
}

func (g *gen) newConn(c, s host, sport uint16) *conn {
	return &conn{
		c: c, s: s, sport: sport,
		cport: uint16(32768 + g.rng.IntN(28000)), //nolint:gosec // < 65536
		cseq:  g.rng.Uint32(),
		sseq:  g.rng.Uint32(),
	}
}

func (k *conn) toServer(seq tcpSeg) tcpSeg { seq.sport, seq.dport = k.cport, k.sport; return seq }
func (k *conn) toClient(seq tcpSeg) tcpSeg { seq.sport, seq.dport = k.sport, k.cport; return seq }

// handshake sends SYN, SYN-ACK after d1 and ACK after a further d2. The
// sensor-observed RTT is d1+d2 (docs/CONFIG.md "Handshake RTT").
func (g *gen) handshake(at time.Duration, k *conn, d1, d2 time.Duration) time.Duration {
	g.tcp(at, k.c, k.s, k.toServer(tcpSeg{seq: k.cseq, syn: true}))
	k.cseq++
	g.tcp(at+d1, k.s, k.c, k.toClient(tcpSeg{seq: k.sseq, ack: k.cseq, syn: true, ackF: true}))
	k.sseq++
	end := at + d1 + d2
	g.tcp(end, k.c, k.s, k.toServer(tcpSeg{seq: k.cseq, ack: k.sseq, ackF: true}))
	return end
}

// send transmits n payload bytes from the client (toServer) or server.
func (g *gen) send(at time.Duration, k *conn, fromClient bool, n int) {
	if fromClient {
		g.tcp(at, k.c, k.s, k.toServer(tcpSeg{seq: k.cseq, ack: k.sseq, ackF: true, psh: true, payload: n}))
		k.cseq += uint32(n) //nolint:gosec // small payloads
		return
	}
	g.tcp(at, k.s, k.c, k.toClient(tcpSeg{seq: k.sseq, ack: k.cseq, ackF: true, psh: true, payload: n}))
	k.sseq += uint32(n) //nolint:gosec // small payloads
}

// ack sends a bare ACK from one side.
func (g *gen) ack(at time.Duration, k *conn, fromClient bool) {
	if fromClient {
		g.tcp(at, k.c, k.s, k.toServer(tcpSeg{seq: k.cseq, ack: k.sseq, ackF: true}))
		return
	}
	g.tcp(at, k.s, k.c, k.toClient(tcpSeg{seq: k.sseq, ack: k.cseq, ackF: true}))
}

// download streams total bytes from server to client in MSS segments every
// gap, the client ACKing every second segment. With retransEvery > 0, every
// Nth segment is sent again 3 ms later with the same sequence number.
func (g *gen) download(at time.Duration, k *conn, total int, gap time.Duration, retransEvery int) time.Duration {
	t := at
	for i := 1; total > 0; i++ {
		n := min(total, mss)
		seq := k.sseq
		g.tcp(t, k.s, k.c, k.toClient(tcpSeg{seq: seq, ack: k.cseq, ackF: true, payload: n}))
		if retransEvery > 0 && i%retransEvery == 0 {
			g.tcp(t+3*time.Millisecond, k.s, k.c, k.toClient(tcpSeg{seq: seq, ack: k.cseq, ackF: true, payload: n}))
		}
		k.sseq += uint32(n) //nolint:gosec // n <= mss
		total -= n
		if i%2 == 0 || total == 0 {
			g.ack(t+ms(0.2), k, true)
		}
		t += gap
	}
	return t
}

// close performs client FIN, server FIN, client ACK.
func (g *gen) close(at time.Duration, k *conn) time.Duration {
	g.tcp(at, k.c, k.s, k.toServer(tcpSeg{seq: k.cseq, ack: k.sseq, fin: true, ackF: true}))
	k.cseq++
	g.tcp(at+ms(1), k.s, k.c, k.toClient(tcpSeg{seq: k.sseq, ack: k.cseq, fin: true, ackF: true}))
	k.sseq++
	g.tcp(at+ms(2), k.c, k.s, k.toServer(tcpSeg{seq: k.cseq, ack: k.sseq, ackF: true}))
	return at + ms(2)
}

// webSession is handshake, request, download, close. It returns the end time.
func (g *gen) webSession(at time.Duration, c, s host, port uint16, d1, d2 time.Duration, request, response, retransEvery int) time.Duration {
	k := g.newConn(c, s, port)
	t := g.handshake(at, k, d1, d2)
	g.send(t+ms(0.5), k, true, request)
	t = g.download(t+d1+d2, k, response, ms(0.4), retransEvery)
	return g.close(t+ms(5), k)
}

// dns sends a query from c to s:53 and the answer after delay.
func (g *gen) dns(at time.Duration, c, s host, delay time.Duration) {
	sport := uint16(32768 + g.rng.IntN(28000)) //nolint:gosec // < 65536
	g.udp(at, c, s, sport, 53, 40)
	g.udp(at+delay, s, c, 53, sport, 88)
}

// synProbe sends a SYN; open ports answer SYN-ACK (the prober then resets),
// closed ports answer RST-ACK, and nil reply means no answer.
func (g *gen) synProbe(at time.Duration, c, s host, cport, port uint16, open *bool) {
	seq := g.rng.Uint32()
	g.tcp(at, c, s, tcpSeg{sport: cport, dport: port, seq: seq, syn: true})
	if open == nil {
		return
	}
	if *open {
		g.tcp(at+ms(0.3), s, c, tcpSeg{sport: port, dport: cport, seq: g.rng.Uint32(), ack: seq + 1, syn: true, ackF: true})
		g.tcp(at+ms(0.5), c, s, tcpSeg{sport: cport, dport: port, seq: seq + 1, rst: true})
		return
	}
	g.tcp(at+ms(0.3), s, c, tcpSeg{sport: port, dport: cport, ack: seq + 1, rst: true, ackF: true})
}
