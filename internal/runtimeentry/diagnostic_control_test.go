package runtimeentry

import (
 "bytes"
 "encoding/json"
 "testing"

 "github.com/lly8666/wobuzhidao/internal/runtimeowner"
)

func TestN1ControlDiagnosticBoundedBinsAndNoPrivateTimestampOrBody(t *testing.T) {
 counts:=runtimeowner.ControlStats{
  QualityBootstrap:1,
  QualityEmitted:2,HealthEmitted:1,MergedHealth:2,
  PaddingBytes:117,ControlWireBytes:583,
  ControlLengthBins:[8]uint64{0,1,2,0,0,0,0,0},
  ControlGapBins:[8]uint64{0,0,1,1,0,0,0,0},
 }
 lane:=LaneDiagnostic{Control:&counts}
 raw,err:=json.Marshal(lane)
 if err!=nil{t.Fatal(err)}
 if !bytes.Contains(raw,[]byte(`"control_length_bins":[0,1,2,0,0,0,0,0]`)) ||
  !bytes.Contains(raw,[]byte(`"control_gap_bins":[0,0,1,1,0,0,0,0]`)) ||
  !bytes.Contains(raw,[]byte(`"control_wire_bytes":583`)) {
  t.Fatalf("bounded control summary absent: %s",raw)
 }
 for _,needle:=range [][]byte{[]byte("lastControlAt"),[]byte("payload"),[]byte("nonce"),[]byte("secret")} {
  if bytes.Contains(raw,needle){t.Fatalf("private datum leaked: %s",needle)}
 }
 bare,err:=json.Marshal(LaneDiagnostic{})
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(bare,[]byte(`"control"`)){t.Fatalf("disabled/old lane must not invent control report: %s",bare)}
}
