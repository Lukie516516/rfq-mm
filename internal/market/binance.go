package market

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type bookTickerResp struct {
	BidPrice string `json:"bidPrice"`
	AskPrice string `json:"askPrice"`
}

// FetchBinanceBookTicker pulls BNBUSDT best bid/ask.
// Stage 1 only uses this when the sim is started with -live.
func FetchBinanceBookTicker(symbol string) (Book, error) {
	url := "https://api.binance.com/api/v3/ticker/bookTicker?symbol=" + symbol
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return Book{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Book{}, fmt.Errorf("binance bookTicker status %d", resp.StatusCode)
	}
	var raw bookTickerResp
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return Book{}, err
	}
	bid, err := strconv.ParseFloat(raw.BidPrice, 64)
	if err != nil {
		return Book{}, err
	}
	ask, err := strconv.ParseFloat(raw.AskPrice, 64)
	if err != nil {
		return Book{}, err
	}
	book := Book{Bid: bid, Ask: ask, Source: "binance-rest", Ts: time.Now().UTC()}
	return book, book.Validate()
}
