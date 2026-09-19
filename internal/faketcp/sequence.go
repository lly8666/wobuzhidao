package faketcp

// seqLT compares TCP sequence numbers in the usual half-range serial number
// space. BootstrapStream uses it to reject data that is already behind the
// contiguous receive frontier, including across uint32 wrap.
func seqLT(a, b uint32) bool { return int32(a-b) < 0 }
