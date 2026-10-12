package realityfront

import (
    "errors"
    "github.com/lly8666/wobuzhidao/internal/fec"
)

// V3 capability is a fixed ten-byte block within TLS-protected admission.
const (
    RecordVersionV3 uint16 = 3
    policyV3Len = 10
    PolicySchemaV3 uint8 = 1
    TransportNormal uint8 = 1
    TransportGame uint8 = 2
    FECOff uint8 = 0
    FECFixed uint8 = 1
    FECAuto uint8 = 2
    FECAggressive uint8 = 3
    CipherChaCha uint8 = 1
    CipherAES128 uint8 = 2
    CipherAES256 uint8 = 3
)
var ErrAdmissionUnsupported = errors.New("realityfront: V3 policy unsupported by this build")

type AdmissionPolicy struct {
    Schema uint8
    Transport uint8
    DesiredLanes uint8
    FECMode uint8
    FixedParity uint8
    InitialParity uint8
    MinParity uint8
    MaxParity uint8
    Cipher uint8
    Quality uint8
}
func allowedParity(r uint8) bool {
    return r == 0 || fec.IsSupportedParityShards(int(r))
}
func (p AdmissionPolicy) Validate() error {
    if p.Schema != PolicySchemaV3 || p.Quality > 1 ||
       p.Cipher < CipherChaCha || p.Cipher > CipherAES256 {
        return ErrAdmissionParams
    }
    if !(p.Transport == TransportNormal && p.DesiredLanes == 1 ||
         p.Transport == TransportGame && p.DesiredLanes >= 2 && p.DesiredLanes <= 4) {
        return ErrAdmissionParams
    }
    switch p.FECMode {
    case FECOff:
        if p.FixedParity != 0 || p.InitialParity != 0 || p.MinParity != 0 || p.MaxParity != 0 {
            return ErrAdmissionParams
        }
    case FECFixed:
        if !allowedParity(p.FixedParity) || p.FixedParity == 0 ||
           p.InitialParity != 0 || p.MinParity != 0 || p.MaxParity != 0 {
            return ErrAdmissionParams
        }
    case FECAuto, FECAggressive:
        if p.Transport != TransportNormal || p.FixedParity != 0 ||
           !allowedParity(p.InitialParity) || !allowedParity(p.MinParity) || !allowedParity(p.MaxParity) ||
           p.MaxParity != 20 ||
           !(p.InitialParity==20 ||
             (p.InitialParity==0 && p.FECMode==FECAuto && p.MinParity==0)) ||
           p.MinParity > p.InitialParity {
            return ErrAdmissionParams
        }
    default:
        return ErrAdmissionParams
    }
    return nil
}
func (p AdmissionPolicy) Encode() ([policyV3Len]byte, error) {
    var out [policyV3Len]byte
    if err := p.Validate(); err != nil { return out, err }
    return [policyV3Len]byte{p.Schema,p.Transport,p.DesiredLanes,p.FECMode,p.FixedParity,
        p.InitialParity,p.MinParity,p.MaxParity,p.Cipher,p.Quality},nil
}
func decodePolicyV3(b []byte) (AdmissionPolicy,error) {
    if len(b) != policyV3Len { return AdmissionPolicy{}, ErrAdmissionParams }
    p:=AdmissionPolicy{b[0],b[1],b[2],b[3],b[4],b[5],b[6],b[7],b[8],b[9]}
    if err:=p.Validate(); err!=nil { return AdmissionPolicy{},err }
    return p,nil
}
// Do not advertise until the corresponding data plane stage is implemented.
func (p AdmissionPolicy) SupportedNow() error {
    if err:=p.Validate();err!=nil { return err }
    // N3 record suite is now bound to the same authenticated V3 policy
    // on both TX/RX lanes. No unauthenticated transport flag selects AES.
    // N2 requires authenticated low-frequency feedback for its own TX
    // direction. Fixed/off can continue to collect quality for GUI later,
    // but never construct an automatic controller.
    if (p.FECMode==FECAuto || p.FECMode==FECAggressive) && p.Quality!=1 {
        return ErrAdmissionUnsupported
    }
    return nil
}
func (p AdmissionPolicy) EffectiveParity() int {
    if p.FECMode==FECFixed { return int(p.FixedParity) }
    if p.FECMode==FECAuto || p.FECMode==FECAggressive {
        // Reserve the immutable max20 MTU/MSS envelope even when the
        // selected sending profile has later stepped down to off.
        return int(p.MaxParity)
    }
    return 0
}

// FixedPolicyForClient maps the already-shipped fec-parity configuration to
// the V3 authenticated policy without silently moving an old fixed tunnel
// into automatic control. This policy must be retained across rotation.
func FixedPolicyForClient(desiredLanes, parity int) (AdmissionPolicy, error) {
    p := AdmissionPolicy{
        Schema: PolicySchemaV3, Transport: TransportNormal,
        DesiredLanes: uint8(desiredLanes), FECMode: FECFixed,
        FixedParity: uint8(parity), Cipher: CipherChaCha,
    }
    if desiredLanes > 1 { p.Transport = TransportGame }
    if parity == 0 { p.FECMode = FECOff }
    if desiredLanes < 1 || desiredLanes > 4 || parity < 0 || parity > 20 {
        return AdmissionPolicy{}, ErrAdmissionParams
    }
    if err := p.SupportedNow(); err != nil { return AdmissionPolicy{}, err }
    return p,nil
}

// PolicyForClient keeps all existing empty-mode configurations pinned to
// their historical explicit fixed/off fec-parity semantics. Auto requires
// an explicit Normal-only choice; it does not infer mode from parity=20.
func PolicyForClient(lanes int, mode string, parity, minimum int) (AdmissionPolicy,error) {
	return PolicyForClientWithInitial(lanes,mode,parity,minimum,20)
}

// PolicyForClientWithInitial is a deliberate opt-in Normal auto experiment.
// Existing legacy/fixed/off and auto configurations WITHOUT an initial flag
// still start at 20:20. Only Normal ordinary auto with explicit min=0 may
// start FEC-off; its protected max20 envelope stays fixed for both MTUs.
func PolicyForClientWithInitial(lanes int, mode string, parity, minimum, initial int) (AdmissionPolicy,error) {
	if initial!=20 && !(mode=="auto" && initial==0 && minimum==0) {
		return AdmissionPolicy{},ErrAdmissionParams
	}
	if mode=="" || mode=="legacy" {
		if minimum!=4 {return AdmissionPolicy{},ErrAdmissionParams}
		return FixedPolicyForClient(lanes,parity)
	}
	switch mode {
	case "fixed":
		if parity==0 || minimum!=4 {return AdmissionPolicy{},ErrAdmissionParams}
		return FixedPolicyForClient(lanes,parity)
	case "off":
		if parity!=0 || minimum!=4 {return AdmissionPolicy{},ErrAdmissionParams}
		return FixedPolicyForClient(lanes,0)
	case "auto","auto-aggressive":
		if lanes!=1 || parity!=0 {return AdmissionPolicy{},ErrAdmissionParams}
		p:=AdmissionPolicy{
			Schema:PolicySchemaV3,Transport:TransportNormal,DesiredLanes:1,
			FECMode:FECAuto,InitialParity:uint8(initial),MinParity:uint8(minimum),
			MaxParity:20,Cipher:CipherChaCha,Quality:1,
		}
		if mode=="auto-aggressive" {p.FECMode=FECAggressive}
		if minimum<0 || minimum>20 {return AdmissionPolicy{},ErrAdmissionParams}
		if initial!=0 && initial!=20 {return AdmissionPolicy{},ErrAdmissionParams}
		if err:=p.SupportedNow();err!=nil {return AdmissionPolicy{},err}
		return p,nil
	default:
		return AdmissionPolicy{},ErrAdmissionParams
	}
}

// PolicyForClientWithInitialAndCipher is the explicit V3 suite opt-in.
// Existing config defaults remain byte-for-byte ChaCha and legacy mode never
// silently upgrades its cipher. A validated Cipher 1/2/3 is carried in the
// ten-byte authenticated admission policy and suite-separated HKDF context.
func PolicyForClientWithInitialAndCipher(lanes int, mode string, parity, minimum, initial int, suite uint8) (AdmissionPolicy,error) {
    if (mode=="" || mode=="legacy") && suite!=CipherChaCha { return AdmissionPolicy{},ErrAdmissionParams }
    p,err:=PolicyForClientWithInitial(lanes,mode,parity,minimum,initial)
    if err!=nil { return AdmissionPolicy{},err }
    p.Cipher=suite
    if err:=p.SupportedNow();err!=nil { return AdmissionPolicy{},err }
    return p,nil
}
