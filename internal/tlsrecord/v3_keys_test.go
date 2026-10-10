package tlsrecord

import (
    "bytes"
    "testing"
)
func TestV3ExporterAndSuiteSeparation(t *testing.T) {
    var nonce [16]byte
    for i:=range nonce {nonce[i]=byte(i)}
    policy:=[]byte{1,1,1,1,4,0,0,0,1,0}
    h,err:=ExporterContextHashV3(3,nonce,[]byte("0123456789abcdef"),1400,1300,policy)
    if err!=nil{t.Fatal(err)}
    cp:=bytes.Clone(policy);cp[4]=8
    other,_:=ExporterContextHashV3(3,nonce,[]byte("0123456789abcdef"),1400,1300,cp)
    if h==other{t.Fatal("policy not exporter-bound")}
    legacy,_:=ExporterContextHash(2,nonce,[]byte("0123456789abcdef"),1400,1300)
    if h==legacy{t.Fatal("V2 context collision")}
    if _,err:=ExporterContextHashV3(3,nonce,nil,1400,1300,policy[:9]);err==nil{t.Fatal("short policy accepted")}
    master:=bytes.Repeat([]byte{0x5a},32)
    a,err:=DeriveKeysV3(master,nonce,1);if err!=nil{t.Fatal(err)}
    b,err:=DeriveKeysV3(master,nonce,2);if err!=nil{t.Fatal(err)}
    if a==b||a.C2S==a.S2C {t.Fatal("suite or direction collision")}
    c,err:=DeriveKeys(master,nonce);if err!=nil{t.Fatal(err)}
    if a==c{t.Fatal("legacy key collision")}
}
