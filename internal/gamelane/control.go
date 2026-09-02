package gamelane

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

const LaneControlSet = "set"

// LaneTarget identifies one healthy local LINK proxy that may participate in
// the Logical Tunnel race. The address is deliberately loopback-only: this is a
// product control plane between the runtime and the local Game/race process,
// never a new public transport or wire protocol.
type LaneTarget struct {
	ID      uint8  `json:"id"`
	Address string `json:"address"`
}

// LaneSetCommand atomically replaces the active Game/race membership. An empty
// lane set is valid and represents DORMANT: the shared TUN/Game process remains
// alive while all public Transport Lanes are absent. During planned replacement
// one logical LaneID may temporarily appear twice with two distinct loopback
// targets. That is bounded transport-incarnation overlap, not a fifth logical
// product lane and not a second PacketID namespace.
type LaneSetCommand struct {
	Op    string       `json:"op"`
	Lanes []LaneTarget `json:"lanes"`
}

type LaneControlReply struct {
	OK     bool    `json:"ok"`
	Error  string  `json:"error,omitempty"`
	Active []uint8 `json:"active,omitempty"`
}

func ParseLaneSetCommand(raw []byte) (LaneSetCommand, error) {
	var cmd LaneSetCommand
	dec := json.NewDecoder(strings.NewReader(strings.TrimSpace(string(raw))))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cmd); err != nil {
		return cmd, fmt.Errorf("decode lane control: %w", err)
	}
	if err := cmd.Validate(); err != nil {
		return cmd, err
	}
	return cmd, nil
}

func (c LaneSetCommand) Validate() error {
	if c.Op != LaneControlSet {
		return fmt.Errorf("game lane control op must be %q", LaneControlSet)
	}
	// Four unique product LaneIDs remain the architectural maximum. The one
	// additional local target is reserved only for old+candidate overlap of one
	// of those existing LaneIDs during make-before-break.
	if len(c.Lanes) > MaxLanes+1 {
		return ErrLanes
	}
	counts := make(map[uint8]int, MaxLanes)
	seenAddr := make(map[string]bool, len(c.Lanes))
	overlapIDs := 0
	for _, lane := range c.Lanes {
		if lane.ID == 0 || int(lane.ID) > MaxLanes {
			return ErrLanes
		}
		counts[lane.ID]++
		if counts[lane.ID] > 2 {
			return fmt.Errorf("lane id %d has more than two transport incarnations", lane.ID)
		}
		if counts[lane.ID] == 2 {
			overlapIDs++
			if overlapIDs > 1 {
				return errors.New("only one logical lane may overlap a replacement candidate")
			}
		}
		addr, err := netip.ParseAddrPort(strings.TrimSpace(lane.Address))
		if err != nil || !addr.Addr().Is4() || !addr.Addr().IsLoopback() || addr.Port() == 0 {
			return errors.New("game lane target must be an IPv4 loopback address:port")
		}
		key := addr.String()
		if seenAddr[key] {
			return fmt.Errorf("duplicate game lane target %s", key)
		}
		seenAddr[key] = true
	}
	return nil
}

func CanonicalLaneTargets(in []LaneTarget) []LaneTarget {
	out := append([]LaneTarget(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Address < out[j].Address
	})
	return out
}
