package tox

import (
	"testing"
	"time"
)

func TestCongestionWindow(t *testing.T) {
	c := &cryptoConn{cwnd: cwndInit, ssthresh: maxCwnd()}

	// Slow start: cwnd grows by the number of packets acked.
	c.onAck(30)
	if c.cwnd != cwndInit+30 {
		t.Fatalf("slow-start cwnd = %v, want %v", c.cwnd, cwndInit+30)
	}

	// A loss halves the window (multiplicative decrease), well above the floor.
	before := c.cwnd
	c.onLoss(time.Now())
	if c.cwnd != before/2 {
		t.Fatalf("after loss cwnd = %v, want %v", c.cwnd, before/2)
	}

	// Congestion avoidance: additive, well under one packet per ack.
	c.cwnd = c.ssthresh + 10 // above threshold
	prev := c.cwnd
	c.onAck(1)
	if d := c.cwnd - prev; d <= 0 || d > 1 {
		t.Fatalf("congestion-avoidance increment = %v, want (0,1]", d)
	}

	// Never grows past the hard ceiling.
	c.cwnd = maxCwnd() - 1
	c.ssthresh = 0
	c.onAck(100)
	if c.cwnd > maxCwnd() {
		t.Fatalf("cwnd %v exceeded ceiling %v", c.cwnd, maxCwnd())
	}

	// Never shrinks below the floor.
	c.cwnd = cwndMin
	c.lastShrink = time.Time{}
	c.onLoss(time.Now().Add(time.Hour))
	if c.cwnd < cwndMin {
		t.Fatalf("cwnd %v fell below floor %v", c.cwnd, cwndMin)
	}
}

func TestCongestionLossOncePerRTT(t *testing.T) {
	c := &cryptoConn{cwnd: 100, ssthresh: maxCwnd(), srtt: 200 * time.Millisecond}
	now := time.Now()
	c.onLoss(now)
	got := c.cwnd // halved once
	c.onLoss(now.Add(10 * time.Millisecond)) // same episode: no further cut
	if c.cwnd != got {
		t.Fatalf("second loss in the same RTT changed cwnd %v -> %v", got, c.cwnd)
	}
	c.onLoss(now.Add(time.Second)) // a later episode halves again
	if c.cwnd != got/2 {
		t.Fatalf("later loss cwnd = %v, want %v", c.cwnd, got/2)
	}
}

func TestRTO(t *testing.T) {
	c := &cryptoConn{}
	if c.rto() != rtoInit {
		t.Fatalf("rto without a sample = %v, want %v", c.rto(), rtoInit)
	}
	c.onRTT(400 * time.Millisecond)
	if c.srtt != 400*time.Millisecond {
		t.Fatalf("first srtt = %v, want 400ms", c.srtt)
	}
	// RTO tracks the smoothed RTT and stays within the clamps.
	for i := 0; i < 20; i++ {
		c.onRTT(400 * time.Millisecond)
	}
	if c.rto() < rtoMin || c.rto() > rtoMax {
		t.Fatalf("rto %v outside [%v,%v]", c.rto(), rtoMin, rtoMax)
	}
	if c.rto() < 400*time.Millisecond {
		t.Fatalf("rto %v should be >= smoothed RTT", c.rto())
	}
}
