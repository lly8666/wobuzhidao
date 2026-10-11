package datapath

import (
	"github.com/lly8666/wobuzhidao/internal/adaptivefec"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

// N2PreparedPolicyForTests validates a negotiated protected V3 Normal policy
// without advertising the incomplete production auto mode. Even when min=off,
// the initial and MAX wire/TUN profile is fixed at 20:20 for this incarnation.
// This stage is deliberately unusable from public admission: SupportedNow
// still rejects these modes on both TLS endpoints.
func N2PreparedPolicyForTests(policy realityfront.AdmissionPolicy) (adaptivefec.Config,error) {
	if policy.Validate()!=nil || policy.Schema!=realityfront.PolicySchemaV3 ||
		policy.Transport!=realityfront.TransportNormal ||
		policy.DesiredLanes!=1 || policy.Quality!=1 ||
		policy.Cipher!=realityfront.CipherChaCha ||
		(policy.FECMode!=realityfront.FECAuto &&
			policy.FECMode!=realityfront.FECAggressive) {
		return adaptivefec.Config{},ErrAdmissionHandoff
	}
	mode:=adaptivefec.Auto
	if policy.FECMode==realityfront.FECAggressive {
		mode=adaptivefec.Aggressive
	}
	cfg:=adaptivefec.Config{
		Mode:mode,
		InitialParity:int(policy.InitialParity),
		MinParity:int(policy.MinParity),
		MaxParity:int(policy.MaxParity),
	}
	if cfg.Validate()!=nil {return adaptivefec.Config{},ErrAdmissionHandoff}
	return cfg,nil
}

func n2FixedBudgetEquivalent(p realityfront.AdmissionPolicy) realityfront.AdmissionPolicy {
	// This copy is ONLY an internal staged MTU calculator and is never
	// echoed, exported, or written to the authenticated admission session.
	p.FECMode=realityfront.FECFixed
	p.FixedParity=p.MaxParity
	p.InitialParity=0
	p.MinParity=0
	p.MaxParity=0
	p.Quality=0
	return p
}

// N2ClientLaneFromProtectedPolicyForTests constructs the real encrypted,
// mixed-profile endpoint after validating the exact negotiated V3 policy.
// Preserve its original authenticated session; use an ephemeral FIXED 20:20
// equivalent solely to derive existing direction-specific peer-MSS budgets.
func N2ClientLaneFromProtectedPolicyForTests(
	session *realityfront.ClientAdmissionSession,params ClientLaneParams,
) (*Lane,adaptivefec.Config,error) {
	if session==nil || session.Negotiated.RecordVersion!=realityfront.RecordVersionV3 {
		return nil,adaptivefec.Config{},ErrAdmissionHandoff
	}
	plan,err:=N2PreparedPolicyForTests(session.Negotiated.Policy)
	if err!=nil{return nil,adaptivefec.Config{},err}
	shadow:=*session
	shadow.Negotiated=session.Negotiated
	shadow.Negotiated.Policy=n2FixedBudgetEquivalent(shadow.Negotiated.Policy)
	laneCfg,err:=ClientLaneConfigFromAdmission(&shadow,params)
	if err!=nil{return nil,adaptivefec.Config{},err}
	lane,err:=NewN2AdaptiveLaneForTests(laneCfg)
	if err!=nil{return nil,adaptivefec.Config{},err}
	// Immutable exact negotiated policy belongs to this lane instance.
	// No copy of another customer's min/mode may arm its TX controller.
	lane.n2ProtectedPolicy=session.Negotiated.Policy
	return lane,plan,nil
}

// Server side binds the SAME original policy, but its outbound S2C record
// budget is ClientLimit and its peer MSS comes from the accepted SYN. There
// is no global server FEC override or per-client MTU mutation.
func N2ServerLaneFromProtectedPolicyForTests(
	session *realityfront.ServerAdmissionSession,assoc *faketcp.ServerAssociation,
	params ServerLaneParams,
) (*Lane,adaptivefec.Config,error) {
	if session==nil||assoc==nil||
		session.Negotiated.RecordVersion!=realityfront.RecordVersionV3 {
		return nil,adaptivefec.Config{},ErrAdmissionHandoff
	}
	plan,err:=N2PreparedPolicyForTests(session.Negotiated.Policy)
	if err!=nil{return nil,adaptivefec.Config{},err}
	shadow:=*session
	shadow.Negotiated=session.Negotiated
	shadow.Negotiated.Policy=n2FixedBudgetEquivalent(shadow.Negotiated.Policy)
	laneCfg,err:=ServerLaneConfigFromAdmission(&shadow,assoc,params)
	if err!=nil{return nil,adaptivefec.Config{},err}
	lane,err:=NewN2AdaptiveLaneForTests(laneCfg)
	if err!=nil{return nil,adaptivefec.Config{},err}
	// Immutable exact negotiated policy belongs to this lane instance.
	// No copy of another customer's min/mode may arm its TX controller.
	lane.n2ProtectedPolicy=session.Negotiated.Policy
	return lane,plan,nil
}
