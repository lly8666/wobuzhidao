package realityfront

import (
    "errors"
    "testing"
)

func TestN3RecordCipherNamesExplicitAndNoUnknownFallback(t *testing.T) {
    for _,tc:=range []struct{name string;want uint8}{
        {"chacha20-poly1305",CipherChaCha},
        {"aes-128-gcm",CipherAES128},
        {"aes-256-gcm",CipherAES256},
    } {
        got,err:=ParseRecordCipherName(tc.name)
        if err!=nil || got!=tc.want {t.Fatalf("cipher %q => %d, %v",tc.name,got,err)}
        p,err:=PolicyForClientWithInitialAndCipher(1,"fixed",4,4,20,got)
        if err!=nil||p.Cipher!=tc.want {t.Fatalf("cipher %q policy: %+v %v",tc.name,p,err)}
    }
    for _,name:=range []string{"","AES-128-GCM","aes128","aes-512-gcm","tls-aes-256-gcm-sha384","chacha20","chacha20-poly1305 ","unknown"} {
        if _,err:=ParseRecordCipherName(name);!errors.Is(err,ErrAdmissionParams) {
            t.Fatalf("accepted bad or outer TLS suite name %q: %v",name,err)
        }
    }
}
