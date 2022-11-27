package lib

import "time"

// WindowElement represents a element in sender window
type WindowElement struct {
	// Sending packet
	Packet *Packet
	// Timeout for packet
	Timer *time.Timer
	// Ack channel
	Ack chan *Packet
}

func NewWindowElement(packet *Packet, timeout time.Duration) *WindowElement {
	w := WindowElement{Packet: packet, Timer: time.NewTimer(timeout), Ack: make(chan *Packet, 1)}
	return &w
}
