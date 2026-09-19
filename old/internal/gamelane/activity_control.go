package gamelane

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const LaneControlActivity = "activity"

// LaneControlEnvelope is used only to dispatch the local Game control-plane
// request. The operation-specific parser remains authoritative and performs
// strict validation after dispatch.
type LaneControlEnvelope struct {
	Op string `json:"op"`
}

// LaneActivityCommand queries real application/TUN payload activity observed by
// the shared Game process. Transport PING/PONG/control never enter this path.
type LaneActivityCommand struct {
	Op string `json:"op"`
}

// PayloadActivity is monotonic for one Game process lifetime. Sequence advances
// once for every accepted non-empty application payload, including payload that
// arrives while DORMANT and is locally dropped because no public lane exists.
type PayloadActivity struct {
	Sequence                    uint64 `json:"sequence"`
	LastPayloadActivityUnixNano int64  `json:"last_payload_activity_unix_nano"`
}

type LaneActivityReply struct {
	OK       bool            `json:"ok"`
	Error    string          `json:"error,omitempty"`
	Activity PayloadActivity `json:"activity"`
}

func ParseLaneControlOp(raw []byte) (string, error) {
	var env LaneControlEnvelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(raw))), &env); err != nil {
		return "", fmt.Errorf("decode lane control envelope: %w", err)
	}
	op := strings.TrimSpace(env.Op)
	if op == "" {
		return "", errors.New("game lane control op is required")
	}
	return op, nil
}

func ParseLaneActivityCommand(raw []byte) (LaneActivityCommand, error) {
	var cmd LaneActivityCommand
	dec := json.NewDecoder(strings.NewReader(strings.TrimSpace(string(raw))))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cmd); err != nil {
		return cmd, fmt.Errorf("decode lane activity control: %w", err)
	}
	if err := ensureActivityControlEOF(dec); err != nil {
		return cmd, err
	}
	if cmd.Op != LaneControlActivity {
		return cmd, fmt.Errorf("game lane activity op must be %q", LaneControlActivity)
	}
	return cmd, nil
}

func ensureActivityControlEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode lane activity control trailer: %w", err)
	}
	return errors.New("lane activity control must contain exactly one JSON object")
}
