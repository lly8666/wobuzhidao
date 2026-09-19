package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lly8666/wobuzhidao/internal/ipset"
)

func main() {
	var (
		action = flag.String("action", "install", "install, verify, or rollback")
		input  = flag.String("input", "", "CIDR or APNIC delegated input file for install")
		out    = flag.String("dir", defaultDir(), "WBD CN ipset directory")
		source = flag.String("source", "manual", "human-readable source label recorded in manifest")
	)
	flag.Parse()

	var err error
	switch *action {
	case "install":
		if *input == "" {
			err = fmt.Errorf("-input is required for install")
			break
		}
		f, openErr := os.Open(*input)
		if openErr != nil {
			err = openErr
			break
		}
		prefixes, parseErr := ipset.ParseCN(f)
		_ = f.Close()
		if parseErr != nil {
			err = parseErr
			break
		}
		m, writeErr := ipset.WriteCNBundle(*out, *source+":"+filepath.Base(*input), prefixes)
		if writeErr != nil {
			err = writeErr
			break
		}
		fmt.Printf("WBD_CN_IPSET_INSTALL_PASS ipv4=%d ipv6=%d source=%q dir=%q\n", m.IPv4Count, m.IPv6Count, m.Source, *out)
	case "verify":
		m, verifyErr := ipset.VerifyCNBundle(*out)
		if verifyErr != nil {
			err = verifyErr
			break
		}
		fmt.Printf("WBD_CN_IPSET_VERIFY_PASS ipv4=%d ipv6=%d source=%q generated=%s\n", m.IPv4Count, m.IPv6Count, m.Source, m.GeneratedUTC)
	case "rollback":
		if restoreErr := ipset.RestorePrevious(*out); restoreErr != nil {
			err = restoreErr
			break
		}
		m, verifyErr := ipset.VerifyCNBundle(*out)
		if verifyErr != nil {
			err = verifyErr
			break
		}
		fmt.Printf("WBD_CN_IPSET_ROLLBACK_PASS ipv4=%d ipv6=%d source=%q\n", m.IPv4Count, m.IPv6Count, m.Source)
	default:
		err = fmt.Errorf("unknown -action %q", *action)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "WBD_CN_IPSET_FAIL", err)
		os.Exit(1)
	}
}

func defaultDir() string {
	if root := os.Getenv("ProgramData"); root != "" {
		return filepath.Join(root, "WBD", "ipsets", "cn")
	}
	if state := os.Getenv("XDG_STATE_HOME"); state != "" {
		return filepath.Join(state, "wbd", "ipsets", "cn")
	}
	return filepath.Join(".", "wbd-ipsets", "cn")
}
