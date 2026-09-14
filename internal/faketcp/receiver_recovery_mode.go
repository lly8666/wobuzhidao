package faketcp

// SetSteadyStateRecoveryMode selects whether steady-state receiver pressure may
// advance cumulative ACK across an unresolved gap. Legacy mode keeps the
// partial-reliability behavior used by the product default. SACK/RACK is the
// explicit strong-recovery mode: it may still deliver out-of-order first
// arrivals immediately, but cumulative ACK must stay behind every unresolved
// gap so the sender retains retransmission ownership until repair succeeds.
//
// Call this after EnableSteadyStateDelivery, which resets adaptive pressure
// state for the post-bootstrap phase.
func (r *Receiver) SetSteadyStateRecoveryMode(mode RecoveryMode) {
	if r == nil {
		return
	}
	r.pressure.forgivenessDisabled = mode == RecoverySACKRACK
}
