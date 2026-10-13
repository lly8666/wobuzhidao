//go:build windows

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// The Windows entry must never pre-seal an IPv4 packet in Router then pass
// the same bytes to TunnelClient.SendPacket. The dispatcher only classifies
// and forwards Direct; protected packets go through one client sender.
func TestN4WindowsEntryUsesSingleProtectedSenderAndNativeDirectIngress(t *testing.T) {
	file,err:=parser.ParseFile(token.NewFileSet(),"main_windows.go",nil,0)
	if err!=nil {t.Fatal(err)}
	calls:=make(map[string]int)
	var run *ast.FuncDecl
	for _,decl:=range file.Decls {
		if fn,ok:=decl.(*ast.FuncDecl);ok&&fn.Name.Name=="runWindows" {run=fn;break}
	}
	if run==nil {t.Fatal("missing actual Windows entry")}
	ast.Inspect(run.Body,func(n ast.Node)bool {
		call,ok:=n.(*ast.CallExpr)
		if !ok {return true}
		switch fn:=call.Fun.(type) {
		case *ast.SelectorExpr:
			calls[fn.Sel.Name]++
		case *ast.Ident:
			calls[fn.Name]++
		}
		return true
	})
	for _,name:=range []string{"NewTUNClassifier","NewTUNScopedFlows",
		"NewNativeUDPDirect","NewTUNPacketDispatcher","ForwardOrProxy"} {
		if calls[name]!=1 {t.Fatalf("actual entry missing or duplicates %s: %d",name,calls[name])}
	}
	if calls["RouteFromTUN"]!=0 || calls["SendPacket"]!=1 {
		t.Fatalf("double-seal or protected send missing: route=%d sends=%d",
			calls["RouteFromTUN"],calls["SendPacket"])
	}
}
