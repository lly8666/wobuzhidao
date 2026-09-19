package faketcp

// seqLT compares TCP sequence numbers in the usual half-range serial number
// space. Bootstrap and transition code use it across uint32 wrap.
func seqLT(a, b uint32) bool { return int32(a-b) < 0 }

func seqLE(a, b uint32) bool { return a == b || seqLT(a, b) }
