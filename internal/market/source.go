package market

// Source is the hedge-venue top of book.
// Stage 1 uses Static or BinanceREST; neither places orders.
type Source interface {
	Snapshot() (Book, error)
}

type Static struct {
	Book Book
}

func (s Static) Snapshot() (Book, error) {
	return s.Book, s.Book.Validate()
}

type BinanceREST struct {
	Symbol string
}

func (s BinanceREST) Snapshot() (Book, error) {
	return FetchBinanceBookTicker(s.Symbol)
}
