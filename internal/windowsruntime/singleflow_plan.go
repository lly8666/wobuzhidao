package windowsruntime

import (
	"strconv"
	"strings"
)

// BuildSingleFlowPlan reuses the already-qualified routing/TUN/DTLS/LINK
// configuration but moves Reality-like admission into the FakeTCP process that
// owns the one public TCP-shaped flow from SYN onward. No ordinary TCP
// bootstrap command is part of the returned product plan.
func BuildSingleFlowPlan(profile Profile, underlay Underlay) (Plan, error) {
	// BuildPlan currently requires a syntactically valid ticket because the V2
	// compatibility path embeds one in LINK argv. Use a non-secret placeholder,
	// then remove that binding below. The V3 Executor injects the real one-time
	// ticket from Plan.TicketPath only after FakeTCP single-flow admission has
	// reached READY.
	plan, err := BuildPlan(profile, underlay, strings.Repeat("0", 64))
	if err != nil {
		return Plan{}, err
	}
	plan.Bootstrap = Command{}
	plan.Link.Args = removeArgPair(plan.Link.Args, "-demo-reality-ticket")
	plan.FakeTCP.Args = append(plan.FakeTCP.Args,
		"--reality-server-name", profile.ServerName,
		"--reality-route-key", profile.RouteKey,
		"--username", profile.Username,
		"--password", profile.Password,
		"--verify-server="+strconv.FormatBool(profile.VerifyServer),
		"--ticket-out", profile.TicketPath,
		"--bootstrap-timeout", "12s",
	)
	return plan, nil
}

func removeArgPair(args []string, key string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == key {
			if i+1 < len(args) {
				i++
			}
			continue
		}
		out = append(out, args[i])
	}
	return out
}
