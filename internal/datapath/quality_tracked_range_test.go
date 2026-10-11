package datapath

import ("testing";"time")

func TestN1QualityReceivePNTrackedBoundedLateReorderAndOverflow(t *testing.T) {
 now:=time.Unix(444,0)
 var w qualityRXWindow
 for pn:=uint64(0);pn<30;pn++ {if pn!=9 {w.observe(pn,false,now)}}
 w.trackRange(1,0,29,now)
 first:=w.trackedRange(1,now)
 if first.Unique!=29||first.CapacityLimited||!first.HaveAnchor {t.Fatalf("bad closed sample %+v",first)}
 for pn:=uint64(30);pn<700;pn++ {w.observe(pn,false,now.Add(time.Second))}
 w.observe(9,false,now.Add(2*time.Second))
 w.observe(9,true,now.Add(2*time.Second))
 mature:=w.trackedRange(1,now.Add(4*time.Second))
 if mature.Unique!=30||mature.Late!=1||mature.Duplicate!=1||mature.CapacityLimited {
  t.Fatalf("late first arrival/duplicate counts on closed sample %+v",mature)
 }
 for seq:=uint64(2);seq<=5;seq++ {w.trackRange(seq,650,699,now.Add(5*time.Second))}
 if evicted:=w.trackedRange(1,now.Add(5*time.Second));!evicted.CapacityLimited {
  t.Fatalf("evicted source must be CAPACITY: %+v",evicted)
 }
 w.observe(700,false,now.Add(6*time.Second))
 if fresh:=w.snapshot(now.Add(6*time.Second));fresh.CapacityLimited {t.Fatalf("old pressure poisoned current window: %+v",fresh)}
}
