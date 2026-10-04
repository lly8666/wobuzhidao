//go:build windows

package main

import (
 "context"
 "os"
 "testing"
 "time"
 "golang.org/x/sys/windows"
)

func TestGUIControlStopAndOwnerEOF(t *testing.T) {
 for _, stop:=range []bool{false,true} {
  r,w,err:=os.Pipe(); if err!=nil {t.Fatal(err)}
  ctx,cancel:=context.WithCancel(context.Background())
  go watchGUIControl(r,cancel)
  if stop { if _,err=w.WriteString("ignored\nstop\n");err!=nil {t.Fatal(err)} } else {w.Close()}
  select {case <-ctx.Done():case <-time.After(time.Second):t.Fatal("GUI stop/owner EOF did not cancel")}
  r.Close();w.Close();cancel()
 }
}

func TestGUIClientSlotFencesUntilCleanupEnds(t *testing.T) {
 h,err:=acquireGUIClientSlot();if err!=nil {t.Fatal(err)}
 if h2,err:=acquireGUIClientSlot();err==nil {windows.CloseHandle(h2);windows.CloseHandle(h);t.Fatal("overlapping GUI client allowed")}
 windows.CloseHandle(h)
 h,err=acquireGUIClientSlot();if err!=nil {t.Fatal(err)};windows.CloseHandle(h)
}
