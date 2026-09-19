package faketcp

import "testing"

func TestCarrierFragmentStatsBoundary(t *testing.T) {
	f, err := NewCarrierFragmenter(1500)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{1460, 1461, 3000} {
		if _, err := f.Fragment(make([]byte, n)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.Fragment(nil); err == nil {
		t.Fatal("empty datagram accepted")
	}
	s := f.Stats()
	if s.PayloadBudget != 1460 || s.Datagrams != 3 || s.FragmentedDatagrams != 2 || s.Frames != 6 || s.InputBytes != 5921 {
		t.Fatalf("unexpected boundary accounting: %+v", s)
	}
}
