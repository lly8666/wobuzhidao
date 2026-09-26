package session

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestAccountRegistryAllowsManySessionsForSameAccount(t *testing.T) {
	const n = 32
	r, err := NewAccountRegistry(n)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			var id LiveID
			id[0] = byte(i + 1)
			if err := r.Add("solo", id, time.Unix(int64(i+1), 0)); err != nil {
				errs <- err
				return
			}
			errs <- r.BindPeer(id, "127.0.0.1:"+itoa(20000+i))
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := r.Len(); got != n {
		t.Fatalf("len=%d want=%d", got, n)
	}
	if got := r.CountAccount("solo"); got != n {
		t.Fatalf("account sessions=%d want=%d", got, n)
	}
	for i := 0; i < n; i++ {
		peer := "127.0.0.1:" + itoa(20000+i)
		e, ok := r.GetByPeer(peer)
		if !ok || e.Account != "solo" || e.ID[0] != byte(i+1) {
			t.Fatalf("peer %s demux=%+v ok=%v", peer, e, ok)
		}
	}
}

func TestAccountRegistryKeysBySessionNotUsername(t *testing.T) {
	r, err := NewAccountRegistry(2)
	if err != nil {
		t.Fatal(err)
	}
	var a, b LiveID
	a[0], b[0] = 1, 2
	if err := r.Add("solo", a, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := r.Add("solo", b, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := r.BindPeer(a, "127.0.0.1:30001"); err != nil {
		t.Fatal(err)
	}
	if err := r.BindPeer(b, "127.0.0.1:30002"); err != nil {
		t.Fatal(err)
	}
	if r.CountAccount("solo") != 2 {
		t.Fatal("username incorrectly collapsed two live sessions")
	}
	if !r.Remove(a) || r.CountAccount("solo") != 1 {
		t.Fatal("removing one session affected shared-account identity")
	}
	if _, ok := r.GetByPeer("127.0.0.1:30001"); ok {
		t.Fatal("removed session left stale peer route")
	}
	if e, ok := r.GetByPeer("127.0.0.1:30002"); !ok || e.ID != b {
		t.Fatal("removing one shared-account session damaged the other peer route")
	}
}

func TestAccountRegistryRejectsPeerCollisionWithoutCollapsingAccount(t *testing.T) {
	r, err := NewAccountRegistry(2)
	if err != nil {
		t.Fatal(err)
	}
	var a, b LiveID
	a[0], b[0] = 1, 2
	if err := r.Add("solo", a, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := r.Add("solo", b, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := r.BindPeer(a, "10.0.0.1:5000"); err != nil {
		t.Fatal(err)
	}
	if err := r.BindPeer(b, "10.0.0.1:5000"); !errors.Is(err, ErrPeerInUse) {
		t.Fatalf("peer collision err=%v", err)
	}
	if got := r.CountAccount("solo"); got != 2 {
		t.Fatalf("peer collision changed shared account count=%d", got)
	}
	if e, ok := r.GetByPeer("10.0.0.1:5000"); !ok || e.ID != a {
		t.Fatalf("peer owner changed after rejected collision: %+v ok=%v", e, ok)
	}
}

func TestAccountRegistryPeerCanMoveWithinSameSession(t *testing.T) {
	r, _ := NewAccountRegistry(1)
	var id LiveID
	id[0] = 7
	if err := r.Add("solo", id, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := r.BindPeer(id, "10.0.0.1:1111"); err != nil {
		t.Fatal(err)
	}
	if err := r.BindPeer(id, "10.0.0.1:2222"); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.GetByPeer("10.0.0.1:1111"); ok {
		t.Fatal("old peer route survived same-session rebind")
	}
	if e, ok := r.GetByPeer("10.0.0.1:2222"); !ok || e.ID != id {
		t.Fatalf("new peer route missing: %+v ok=%v", e, ok)
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
