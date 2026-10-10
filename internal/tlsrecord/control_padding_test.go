package tlsrecord

import (
 "bytes"
 "errors"
 "testing"
)

func TestN1ControlHealthPaddingZeroMaximumAndAuthenticatedRoundTrip(t *testing.T) {
 keys:=testKeys(t)
 for _,bodyLen:=range []int{9,104,128} {
  body:=make([]byte,bodyLen)
  body[0]=byte(bodyLen)
  zero,_:=NewSealer(keys,FixedWireOverhead+bodyLen+73)
  ref,_:=NewSealer(keys,FixedWireOverhead+bodyLen+73)
  base,pn,err:=ref.SealHealth(body)
  if err!=nil{t.Fatal(err)}
  unpadded,pn2,err:=zero.SealHealthWithPadding(body,0)
  if err!=nil || pn!=pn2 || !bytes.Equal(base,unpadded) {
   t.Fatalf("size %d zero-padding changed original wire: %v",bodyLen,err)
  }
  sealer,_:=NewSealer(keys,FixedWireOverhead+bodyLen+73)
  opener,_:=NewOpener(keys,FixedWireOverhead+bodyLen+73)
  for _,padding:=range []int{1,37,73}{
   wire,pn,err:=sealer.SealHealthWithPadding(body,padding)
   if err!=nil{t.Fatal(err)}
   if len(wire)!=FixedWireOverhead+bodyLen+padding {
    t.Fatalf("size %d padded=%d wire=%d",bodyLen,padding,len(wire))
   }
   rec,err:=opener.OpenRecord(wire)
   if err!=nil || rec.Kind!=KindHealth || rec.PN!=pn || !bytes.Equal(rec.Payload,body){
    t.Fatalf("size %d padded %d open=%+v err=%v",bodyLen,padding,rec,err)
   }
   tampered:=bytes.Clone(wire)
   tampered[len(tampered)-1]^=1
   if _,err:=opener.OpenRecord(tampered);!errors.Is(err,ErrAuthentication) {
    t.Fatalf("tamper accepted for size=%d padding=%d err=%v",bodyLen,padding,err)
   }
  }
  if _,_,err:=sealer.SealHealthWithPadding(body,74);!errors.Is(err,ErrPaddingTooLarge){
   t.Fatalf("oversize health padding accepted: size=%d err=%v",bodyLen,err)
  }
  if _,_,err:=sealer.SealHealthWithPadding(body,-1);!errors.Is(err,ErrInvalidPadding){
   t.Fatalf("negative health padding accepted: %v",err)
  }
  short,_:=NewSealer(keys,FixedWireOverhead+bodyLen-1)
  if _,_,err:=short.SealHealthWithPadding(body,0);!errors.Is(err,ErrPayloadTooLarge){
   t.Fatalf("base health truncated: size=%d err=%v",bodyLen,err)
  }
 }
}
