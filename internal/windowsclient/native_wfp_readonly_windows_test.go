//go:build windows

package windowsclient

import "testing"

func TestN4WFPReadOnlyLayersAvailable(t *testing.T){
 if err:=(N4NativeWFPReader{}).VerifyLayers();err!=nil{t.Fatal(err)}
 t.Log("WFP IPv4 connect + IPv6 connect/receive layers present; no filters installed or network mutated")
}
