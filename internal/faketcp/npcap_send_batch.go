package faketcp

import (
	"encoding/binary"
	"errors"
	"io"
)

// Send only the caller's already-ready segments. This scratch is not an
// asynchronous transmit backlog; sparse/control calls continue to send one.
const (
	npcapSendBatchSize    = 8
	npcapSendQueueBytes   = 16 * 1024
	npcapQueueHeaderBytes = 16 // Windows timeval(int32,int32), caplen, len.
)

var errNpcapBatchReceipt = errors.New("faketcp: invalid Npcap send-queue receipt")

// npcapQueuePrefix translates the driver byte receipt, including packet
// headers, to the successfully consumed packet prefix. A malformed partial
// packet receipt is an error, never permission to retry the uncertain suffix.
func npcapQueuePrefix(bytes uint32, ends []int) (int, error) {
	if len(ends) == 0 {
		if bytes == 0 {
			return 0, nil
		}
		return 0, errNpcapBatchReceipt
	}
	if uint64(bytes) > uint64(ends[len(ends)-1]) {
		return 0, errNpcapBatchReceipt
	}
	n := 0
	for n < len(ends) && uint64(ends[n]) <= uint64(bytes) {
		n++
	}
	if bytes != 0 && (n == 0 || uint32(ends[n-1]) != bytes) {
		return n, errNpcapBatchReceipt
	}
	if n != len(ends) {
		return n, io.ErrShortWrite
	}
	return n, nil
}

func writeNpcapReadySegments(segments []Segment, cfg NpcapConfig, ipID *uint16,
	scratch []byte, single func([]byte) error, batch func([]byte) (uint32, error),
	written func(packets int, bytes int)) (int, error) {
	if err := cfg.Validate(); err != nil {
		return 0, err
	}
	for _, seg := range segments {
		if !cfg.Flow.matchesSegment(seg, false) {
			return 0, ErrNpcapOutboundFlow
		}
	}
	total := 0
	for total < len(segments) {
		var frames [npcapSendBatchSize][]byte
		var packetBytes [npcapSendBatchSize]int
		var ends [npcapSendBatchSize]int
		used, count := 0, 0
		for count < npcapSendBatchSize && total+count < len(segments) {
			packet, frame, err := encodeNpcapOutbound(segments[total+count], cfg, *ipID)
			if err != nil {
				return total, err
			}
			if count > 0 && (batch == nil || used+npcapQueueHeaderBytes+len(frame) > len(scratch)) {
				break
			}
			*ipID++
			frames[count], packetBytes[count] = frame, len(packet)
			if batch != nil && used+npcapQueueHeaderBytes+len(frame) <= len(scratch) {
				header := scratch[used : used+npcapQueueHeaderBytes]
				clear(header) // sync=0; no timestamp-based pacing or busy waits.
				binary.LittleEndian.PutUint32(header[8:12], uint32(len(frame)))
				binary.LittleEndian.PutUint32(header[12:16], uint32(len(frame)))
				copy(scratch[used+npcapQueueHeaderBytes:], frame)
				used += npcapQueueHeaderBytes + len(frame)
				ends[count] = used
			} else {
				count++
				break
			} // Oversize or unavailable queue: single packet.
			count++
		}
		if count == 1 {
			if err := single(frames[0]); err != nil {
				return total, err
			}
			if written != nil {
				written(1, packetBytes[0])
			}
			total++
			continue
		}
		bytes, err := batch(scratch[:used])
		n, receiptErr := npcapQueuePrefix(bytes, ends[:count])
		if written != nil && n > 0 {
			sum := 0
			for _, size := range packetBytes[:n] {
				sum += size
			}
			written(n, sum)
		}
		total += n
		if err != nil {
			return total, err
		}
		if receiptErr != nil {
			return total, receiptErr
		}
	}
	return total, nil
}
