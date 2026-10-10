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
           p.InitialParity != 20 || p.MaxParity != 20 || p.MinParity > p.InitialParity {
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
    if p.Cipher!=CipherChaCha || p.Quality!=0 ||
       (p.FECMode!=FECOff && p.FECMode!=FECFixed) {
        return ErrAdmissionUnsupported
    }
    return nil
}
func (p AdmissionPolicy) EffectiveParity() int {
    if p.FECMode==FECFixed { return int(p.FixedParity) }
    return 0
}
