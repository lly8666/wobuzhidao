//go:build windows

package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/windowsclient"
)

func TestN4OwnedApplyFailureOrStopAlwaysRollsBack(t *testing.T) {
	ctx,cancel:=context.WithCancel(context.Background())
	calls:=make([]string,0,2)
	run:=func(c context.Context,_ windowsclient.NetworkPlan,action,script string)error {
		calls=append(calls,action)
		if action=="Apply" {
			cancel() // a GUI-owned Stop during a multi-route Apply
			return context.Canceled
		}
		if action!="Cleanup" || script!="owned-network.ps1" || c.Err()!=nil {
			t.Fatalf("owned rollback used cancelled context or wrong action: %s %v",action,c.Err())
		}
		return nil
	}
	cleanup,err:=applyOwnedNetworkWith(ctx,windowsclient.NetworkPlan{},"owned-network.ps1",run)
	if cleanup!=nil || !errors.Is(err,context.Canceled) ||
		strings.Join(calls,",")!="Apply,Cleanup" {
		t.Fatalf("cancelled Apply skipped owned rollback: err=%v calls=%v",err,calls)
	}
}

func TestN4OwnedCleanupIdempotentAfterSuccessAndSurvivesStop(t *testing.T) {
	ctx,cancel:=context.WithCancel(context.Background())
	var mu sync.Mutex
	calls:=make([]string,0,2)
	run:=func(c context.Context,_ windowsclient.NetworkPlan,action,script string)error {
		mu.Lock();calls=append(calls,action);mu.Unlock()
		if c.Err()!=nil {t.Fatalf("run %s inherited canceled context",action)}
		return nil
	}
	cleanup,err:=applyOwnedNetworkWith(ctx,windowsclient.NetworkPlan{},"owned-network.ps1",run)
	if err!=nil || cleanup==nil {t.Fatalf("owned Apply: %v",err)}
	cancel()
	if err:=cleanup();err!=nil {t.Fatal(err)}
	if err:=cleanup();err!=nil {t.Fatal(err)}
	mu.Lock();defer mu.Unlock()
	if strings.Join(calls,",")!="Apply,Cleanup" {
		t.Fatalf("duplicate or missing cleanup: %v",calls)
	}
}

func TestN4OwnedApplyAndCleanupErrorsRemainVisible(t *testing.T) {
	applyErr:=errors.New("owned Apply failed")
	cleanupErr:=errors.New("owned rollback failed")
	run:=func(_ context.Context,_ windowsclient.NetworkPlan,action,_ string)error {
		if action=="Apply" {return applyErr}
		return cleanupErr
	}
	cleanup,err:=applyOwnedNetworkWith(context.Background(),windowsclient.NetworkPlan{},"owned-network.ps1",run)
	if cleanup!=nil || !errors.Is(err,applyErr) || !errors.Is(err,cleanupErr) {
		t.Fatalf("lost original Apply or owned rollback failure: %v",err)
	}
}

func TestN4PowerShellOSCommandHonorsGUIStopContext(t *testing.T) {
	ctx,cancel:=context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	started:=time.Now()
	cmd:=networkCommandContext(ctx,[]string{"-NoProfile","-NonInteractive","-Command","Start-Sleep -Seconds 60"})
	if err:=cmd.Run();err==nil {
		t.Fatal("canceled PowerShell returned success")
	}
	if time.Since(started)>9*time.Second {
		t.Fatal("PowerShell process remained stuck after GUI cancellation")
	}
}
