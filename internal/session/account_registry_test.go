package session

import (
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
			errs <- r.Add("solo", id, time.Unix(int64(i+1), 0))
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
	if r.CountAccount("solo") != 2 {
		t.Fatal("username incorrectly collapsed two live sessions")
	}
	if !r.Remove(a) || r.CountAccount("solo") != 1 {
		t.Fatal("removing one session affected shared-account identity")
	}
}
