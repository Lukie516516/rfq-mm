package store

import (
	"path/filepath"
	"testing"
)

func TestPutAndReopen(t *testing.T) {
	p := filepath.Join(t.TempDir(), "orders.json")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(Record{ClientOrderID: "hedge-1", Status: Sent, Symbol: "BNBUSDT"}); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	open := s2.OpenOrders()
	if len(open) != 1 || open[0].ClientOrderID != "hedge-1" {
		t.Fatalf("%+v", open)
	}
	if err := s2.Put(Record{ClientOrderID: "hedge-1", Status: Filled}); err != nil {
		t.Fatal(err)
	}
	s3, _ := Open(p)
	if n := len(s3.OpenOrders()); n != 0 {
		t.Fatalf("filled should not be open, n=%d", n)
	}
	if rec, ok := s3.Get("hedge-1"); !ok || rec.Status != Filled {
		t.Fatalf("get %+v ok=%v", rec, ok)
	}
}
