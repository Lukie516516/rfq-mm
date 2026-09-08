package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Status string

const (
	New        Status = "NEW"
	Sent       Status = "SENT"
	Uncertain  Status = "UNCERTAIN"
	Filled     Status = "FILLED"
	Partial    Status = "PARTIAL"
	Dead       Status = "DEAD"
)

type Record struct {
	ClientOrderID string    `json:"clientOrderId"`
	Symbol        string    `json:"symbol"`
	Side          string    `json:"side"`
	Qty           float64   `json:"qty"`
	Price         float64   `json:"price"`
	Status        Status    `json:"status"`
	OrderID       int64     `json:"orderId,omitempty"`
	ExecQty       float64   `json:"execQty,omitempty"`
	Note          string    `json:"note,omitempty"`
	Updated       time.Time `json:"updated"`
}

func (s Status) Open() bool {
	return s == New || s == Sent || s == Uncertain || s == Partial
}

type File struct {
	path string
	mu   sync.Mutex
	all  map[string]Record
}

func Open(path string) (*File, error) {
	f := &File{path: path, all: map[string]Record{}}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil
		}
		return nil, err
	}
	var list []Record
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, err
		}
	}
	for _, r := range list {
		f.all[r.ClientOrderID] = r
	}
	return f, nil
}

func (f *File) Put(r Record) error {
	if f == nil {
		return nil
	}
	if r.Updated.IsZero() {
		r.Updated = time.Now().UTC()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.all == nil {
		f.all = map[string]Record{}
	}
	f.all[r.ClientOrderID] = r
	return f.flush()
}

func (f *File) Get(cid string) (Record, bool) {
	if f == nil {
		return Record{}, false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.all[cid]
	return r, ok
}

func (f *File) All() []Record {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Record, 0, len(f.all))
	for _, r := range f.all {
		out = append(out, r)
	}
	return out
}

func (f *File) OpenOrders() []Record {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Record
	for _, r := range f.all {
		if r.Status.Open() {
			out = append(out, r)
		}
	}
	return out
}

func (f *File) flush() error {
	if f.path == "" {
		return nil
	}
	list := make([]Record, 0, len(f.all))
	for _, r := range f.all {
		list = append(list, r)
	}
	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return err
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, f.path)
}
