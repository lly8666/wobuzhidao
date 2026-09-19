package fec

import "sort"

// completedBlockSet remembers completed BlockIDs without retaining one map entry
// per block for the lifetime of a LINK association. Normal in-order completion
// advances through and uses constant memory. Out-of-order completion is stored
// as merged ranges until the gaps catch up.
//
// WBD FEC BlockIDs start at 1 for each immutable LINK association. The encoder
// increments them monotonically, so an old shard at or below through can never
// belong to a future block in the same association.
type completedBlockSet struct {
	through uint32
	ranges  []completedBlockRange
}

type completedBlockRange struct {
	first uint32
	last  uint32
}

func (s *completedBlockSet) contains(id uint32) bool {
	if id != 0 && id <= s.through {
		return true
	}
	i := sort.Search(len(s.ranges), func(i int) bool {
		return s.ranges[i].last >= id
	})
	return i < len(s.ranges) && s.ranges[i].first <= id
}

func (s *completedBlockSet) add(id uint32) {
	if id == 0 || id <= s.through || s.contains(id) {
		return
	}
	if uint64(s.through)+1 == uint64(id) {
		s.through = id
		s.collapsePrefix()
		return
	}

	i := sort.Search(len(s.ranges), func(i int) bool {
		return s.ranges[i].first >= id
	})
	leftAdjacent := i > 0 && uint64(s.ranges[i-1].last)+1 == uint64(id)
	rightAdjacent := i < len(s.ranges) && uint64(id)+1 == uint64(s.ranges[i].first)

	switch {
	case leftAdjacent && rightAdjacent:
		s.ranges[i-1].last = s.ranges[i].last
		copy(s.ranges[i:], s.ranges[i+1:])
		s.ranges = s.ranges[:len(s.ranges)-1]
	case leftAdjacent:
		s.ranges[i-1].last = id
	case rightAdjacent:
		s.ranges[i].first = id
	default:
		s.ranges = append(s.ranges, completedBlockRange{})
		copy(s.ranges[i+1:], s.ranges[i:])
		s.ranges[i] = completedBlockRange{first: id, last: id}
	}
	s.collapsePrefix()
}

func (s *completedBlockSet) collapsePrefix() {
	for len(s.ranges) != 0 && uint64(s.through)+1 == uint64(s.ranges[0].first) {
		s.through = s.ranges[0].last
		copy(s.ranges, s.ranges[1:])
		s.ranges = s.ranges[:len(s.ranges)-1]
	}
}
