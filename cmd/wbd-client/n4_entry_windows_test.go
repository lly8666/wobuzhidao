//go:build windows

package main

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/windowsclient"
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
	for _,name:=range []string{"BuildCompactNetworkPlan","NewTUNClassifier","NewTUNScopedFlows",
		"NewUserspaceTCPDirect","NewNativeUDPDirect","NewN4NativeDirect",
		"NewTUNPacketDispatcher","ForwardOrProxy"} {
		if calls[name]!=1 {t.Fatalf("actual entry missing or duplicates %s: %d",name,calls[name])}
	}
	if calls["RouteFromTUN"]!=0 || calls["SendPacket"]!=1 {
		t.Fatalf("double-seal or protected send missing: route=%d sends=%d",
			calls["RouteFromTUN"],calls["SendPacket"])
	}
}

func TestN4WindowsLegacyNetworkRunnerNeverExpandsCaptureCIDRs(t *testing.T){
 for _,tc:=range []struct{name string;plan windowsclient.NetworkPlan}{
  {"capture-explosion",windowsclient.NetworkPlan{CaptureRoutes:make([]windowsclient.OwnedRoute,9)}},
  {"direct-host-routes",windowsclient.NetworkPlan{DirectRoutes:[]windowsclient.OwnedRoute{{Kind:"direct"}}}},
  {"capture-file-bypass",windowsclient.NetworkPlan{CapturePrefixFile4:"huge-capture-cidrs.txt"}},
 }{
  t.Run(tc.name,func(t *testing.T){
   // The guard is evaluated before script arguments or PowerShell execution.
   if err:=runNetworkActionContext(context.Background(),tc.plan,"Apply","network.ps1");!errors.Is(err,windowsclient.ErrNetworkPlan){
    t.Fatalf("unsafe route explosion was not rejected: %v",err)
   }
  })
 }
}
