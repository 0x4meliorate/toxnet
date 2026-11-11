package tox

import "time"

// Congestion control for file transfers.
//
// A bulk transfer paced only by the net_crypto window (256 packets) can overrun
// a TCP relay: packets drop, retransmits re-flood, and the transfer collapses.
// Instead the file pump is paced by a per-connection congestion window (cwnd)
// that adapts to the path, TCP-style: it grows while acknowledgements arrive
// cleanly and halves on a retransmission, settling just below the rate the relay
// can carry. Retransmission timeouts derive from the measured round-trip time,
// so a high-latency link does not mistake slow acks for loss.
//
// cwnd paces file data only; control traffic (acks, keep-alives, messages) is
// never throttled. It is capped by the net_crypto window minus a reserve.

const (
	cwndInit = 10.0 // initial congestion window, in packets
	cwndMin  = 8.0  // floor so a transfer never stalls completely

	rtoInit = 1 * time.Second        // retransmission timeout before any RTT sample
	rtoMin  = 200 * time.Millisecond // clamp so we never resend too eagerly
	rtoMax  = 8 * time.Second        // clamp so a dead link gives up in bounded time
)

// maxCwnd is the hard ceiling: the reliable window less the control reserve.
func maxCwnd() float64 { return float64(sendWindow - fileReserve) }

// rto is the retransmission timeout, from the smoothed RTT (RFC 6298 style).
func (c *cryptoConn) rto() time.Duration {
	if c.srtt == 0 {
		return rtoInit
	}
	r := c.srtt + 4*c.rttvar
	if r < rtoMin {
		r = rtoMin
	}
	if r > rtoMax {
		r = rtoMax
	}
	return r
}

// onRTT folds a round-trip sample into the smoothed estimate (RFC 6298).
func (c *cryptoConn) onRTT(sample time.Duration) {
	if sample <= 0 {
		return
	}
	if c.srtt == 0 {
		c.srtt = sample
		c.rttvar = sample / 2
		return
	}
	d := c.srtt - sample
	if d < 0 {
		d = -d
	}
	c.rttvar = (3*c.rttvar + d) / 4
	c.srtt = (7*c.srtt + sample) / 8
}

// onAck grows cwnd after `acked` packets were acknowledged without loss: slow
// start (roughly doubling per RTT) until the threshold, then additive increase
// (about one packet per RTT).
func (c *cryptoConn) onAck(acked int) {
	if acked <= 0 {
		return
	}
	if c.cwnd == 0 {
		c.cwnd = cwndInit
	}
	if c.cwnd < c.ssthresh {
		c.cwnd += float64(acked)
	} else {
		c.cwnd += float64(acked) / c.cwnd
	}
	if hi := maxCwnd(); c.cwnd > hi {
		c.cwnd = hi
	}
}

// onLoss halves cwnd on a retransmission, at most once per round trip so a burst
// of losses from one congestion episode only backs off once.
func (c *cryptoConn) onLoss(now time.Time) {
	if c.cwnd == 0 {
		c.cwnd = cwndInit
	}
	gap := c.srtt
	if gap < rtoMin {
		gap = rtoMin
	}
	if !c.lastShrink.IsZero() && now.Sub(c.lastShrink) < gap {
		return
	}
	c.lastShrink = now
	c.ssthresh = c.cwnd / 2
	if c.ssthresh < cwndMin {
		c.ssthresh = cwndMin
	}
	c.cwnd = c.ssthresh
}

// fileLimit is how many file-data packets may be in flight right now: the
// congestion window, clamped to the reliable window less the control reserve.
func (c *cryptoConn) fileLimit() int {
	if c.cwnd == 0 {
		c.cwnd = cwndInit
	}
	limit := int(c.cwnd)
	if hi := sendWindow - fileReserve; limit > hi {
		limit = hi
	}
	if limit < 1 {
		limit = 1
	}
	return limit
}
