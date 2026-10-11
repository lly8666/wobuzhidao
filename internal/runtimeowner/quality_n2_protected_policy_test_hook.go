package runtimeowner

import (
	"github.com/lly8666/wobuzhidao/internal/adaptivefec"
	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

// ConfigureN2ProtectedPolicyForTests binds only the EXACT authenticated
// policy already saved on the active Normal AEAD Lane to its local-TX
// first-attempt controller. Must occur after ConfigureQualityV3 with the
// SAME protected incarnation nonce. No product entrypoint calls this, and
// SupportedNow still refuses auto/aggressive on the wire.
func (r *Runtime) ConfigureN2ProtectedPolicyForTests(
	ref logicaltunnel.LaneRef,accepted realityfront.AdmissionResult,
) error {
	if r==nil{return ErrRuntimeClosed}
	if accepted.RecordVersion!=realityfront.RecordVersionV3 {return ErrTransportConfig}
	cfg,err:=datapath.N2PreparedPolicyForTests(accepted.Policy)
	if err!=nil {return err}
	// Allocate outside all transport locks; no packet or replacement waits
	// behind cold controller allocation.
	controller,err:=adaptivefec.New(cfg,ref.Generation)
	if err!=nil {return err}
	r.outboundMu.RLock()
	defer r.outboundMu.RUnlock()
	if err:=r.owner.N2VerifyActiveProtectedPolicyForTests(ref,accepted.Policy);err!=nil {
		return err
	}
	r.mu.Lock()
	t:=r.lanes[ref]
	if r.closed||r.active[ref.ID]!=ref||t==nil {
		r.mu.Unlock()
		return ErrTransportMissing
	}
	t.mu.Lock()
	r.mu.Unlock()
	defer t.mu.Unlock()
	if t.closed||!t.quality.enabled||t.quality.mailbox==nil||
		t.quality.nonce!=accepted.IncarnationNonce||
		t.quality.n2!=nil {
		return ErrTransportConfig
	}
	t.quality.n2=controller
	return nil
}
