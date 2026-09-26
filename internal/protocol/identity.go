package protocol

// FlowID identifies one logical WBD flow inside a session.
type FlowID uint64

// TransmissionID identifies one transmission attempt of logical data.
// Reinjection keeps the logical FlowID/offset/datagram identity but allocates
// a new TransmissionID.
type TransmissionID uint64

// LaneID identifies one real TCP carrier in the local session state.
// LaneID is intentionally not encoded in DATA/DATAGRAM frames because the
// carrier connection itself supplies lane context. This lets the same logical
// data be reinjected on a different lane without changing logical identity.
type LaneID uint64

// StreamOffset is the byte offset of reliable stream data.
type StreamOffset uint64

// DatagramID identifies one unreliable/expiring datagram within a flow.
type DatagramID uint64
