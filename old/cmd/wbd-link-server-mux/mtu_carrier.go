package main

import (
	"flag"
	"fmt"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/pathmtu"
)

var serverConnectionMTU = pathmtu.DefaultConnectionMTU

func init() {
	flag.IntVar(&serverConnectionMTU, "connection-mtu", pathmtu.DefaultConnectionMTU, "local outer connection MTU ceiling used to admit each negotiated LINK config")
}

// linkConfigCarrierValidator returns a config-aware admission check for one
// local carrier ceiling. LINK plaintext capacity depends on the association's
// negotiated FEC mode, so this must run per LINK_INIT rather than narrowing the
// protocol-wide LinkPolicy.MaxMTU to one static value.
func linkConfigCarrierValidator(connectionMTU int) (control.LinkConfigValidator, error) {
	if err := pathmtu.ValidateConnectionMTU(connectionMTU); err != nil {
		return nil, err
	}
	return func(cfg control.LinkConfig) error {
		budget, err := pathmtu.Derive(connectionMTU, pathmtu.Features{
			FEC:  cfg.FECMode == control.FECFixed,
			Game: true,
		})
		if err != nil {
			return err
		}
		if int(cfg.MTU) > budget.LinkPlaintextMTU {
			return fmt.Errorf("%w: negotiated LINK plaintext MTU %d exceeds local carrier ceiling %d (connection MTU %d, fec=%v)", control.ErrLimit, cfg.MTU, budget.LinkPlaintextMTU, connectionMTU, cfg.FECMode == control.FECFixed)
		}
		return nil
	}, nil
}
