package binance

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"rfq-mm/internal/market"
)

const (
	TestnetBase = "https://testnet.binance.vision"
	ProdBase    = "https://api.binance.com"
)

type Client struct {
	BaseURL string
	APIKey  string
	Secret  string
	HTTP    *http.Client
	Log     *slog.Logger
}

type OrderAck struct {
	Symbol             string `json:"symbol"`
	OrderID            int64  `json:"orderId"`
	Status             string `json:"status"`
	Side               string `json:"side"`
	Type               string `json:"type"`
	ExecutedQty        string `json:"executedQty"`
	CummulativeQuoteQty string `json:"cummulativeQuoteQty"`
	Fills              []struct {
		Price           string `json:"price"`
		Qty             string `json:"qty"`
		Commission      string `json:"commission"`
		CommissionAsset string `json:"commissionAsset"`
	} `json:"fills"`
}

type APIError struct {
	Status int
	Code   int    `json:"code"`
	Msg    string `json:"msg"`
	Body   string
}

func (e *APIError) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("binance http %d code %d: %s", e.Status, e.Code, e.Msg)
	}
	return fmt.Sprintf("binance http %d: %s", e.Status, e.Body)
}

func NewFromEnv() *Client {
	base := os.Getenv("BINANCE_BASE_URL")
	if base == "" {
		base = TestnetBase
	}
	return &Client{
		BaseURL: strings.TrimRight(base, "/"),
		APIKey:  os.Getenv("BINANCE_API_KEY"),
		Secret:  os.Getenv("BINANCE_API_SECRET"),
		HTTP:    &http.Client{Timeout: 10 * time.Second},
		Log:     slog.Default(),
	}
}

func (c *Client) logger() *slog.Logger {
	if c.Log != nil {
		return c.Log
	}
	return slog.Default()
}

func (c *Client) BookTicker(symbol string) (market.Book, error) {
	body, err := c.public("GET", "/api/v3/ticker/bookTicker", url.Values{"symbol": {symbol}})
	if err != nil {
		return market.Book{}, err
	}
	var raw struct {
		BidPrice string `json:"bidPrice"`
		AskPrice string `json:"askPrice"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return market.Book{}, err
	}
	bid, err := strconv.ParseFloat(raw.BidPrice, 64)
	if err != nil {
		return market.Book{}, err
	}
	ask, err := strconv.ParseFloat(raw.AskPrice, 64)
	if err != nil {
		return market.Book{}, err
	}
	book := market.Book{Bid: bid, Ask: ask, Source: c.BaseURL, Ts: time.Now().UTC()}
	c.logger().Info("market_data", "endpoint", "bookTicker", "symbol", symbol, "bid", book.Bid, "ask", book.Ask, "raw", string(body))
	return book, book.Validate()
}

func (c *Client) ExchangeFilters(symbol string) (Filters, error) {
	body, err := c.public("GET", "/api/v3/exchangeInfo", url.Values{"symbol": {symbol}})
	if err != nil {
		return Filters{}, err
	}
	c.logger().Info("market_data", "endpoint", "exchangeInfo", "symbol", symbol, "bytes", len(body))
	return ParseExchangeInfo(body, symbol)
}

func (c *Client) PlaceMarket(symbol, side string, qty float64, step float64) (OrderAck, error) {
	return c.PlaceMarketID(symbol, side, qty, step, NewClientOrderID("rfq"))
}

func (c *Client) PlaceMarketID(symbol, side string, qty float64, step float64, clientOrderID string) (OrderAck, error) {
	q := url.Values{}
	q.Set("symbol", symbol)
	q.Set("side", strings.ToUpper(side))
	q.Set("type", "MARKET")
	q.Set("quantity", FormatStep(qty, step))
	q.Set("newOrderRespType", "FULL")
	if clientOrderID != "" {
		q.Set("newClientOrderId", clientOrderID)
	}
	c.logger().Info("order_request", "symbol", symbol, "side", side, "qty", FormatStep(qty, step), "clientOrderId", clientOrderID)
	body, err := c.signed("POST", "/api/v3/order", q)
	if err != nil {
		return OrderAck{}, err
	}
	var ack OrderAck
	if err := json.Unmarshal(body, &ack); err != nil {
		return OrderAck{}, fmt.Errorf("decode order ack: %w body=%s", err, body)
	}
	c.logger().Info("exchange_response",
		"orderId", ack.OrderID,
		"status", ack.Status,
		"executedQty", ack.ExecutedQty,
		"cummulativeQuoteQty", ack.CummulativeQuoteQty,
		"raw", string(body),
	)
	return ack, nil
}

func (c *Client) public(method, path string, q url.Values) ([]byte, error) {
	u := c.BaseURL + path
	if encoded := q.Encode(); encoded != "" {
		u += "?" + encoded
	}
	c.logger().Info("http_request", "method", method, "url", u)
	req, err := http.NewRequest(method, u, nil)
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

func (c *Client) signed(method, path string, q url.Values) ([]byte, error) {
	if c.APIKey == "" || c.Secret == "" {
		return nil, fmt.Errorf("BINANCE_API_KEY / BINANCE_API_SECRET not set")
	}
	q.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	q.Set("recvWindow", "5000")
	payload := q.Encode()
	sig := Sign(c.Secret, payload)
	u := c.BaseURL + path + "?" + payload + "&signature=" + sig
	c.logger().Info("order_request", "method", method, "path", path, "query", payload)
	req, err := http.NewRequest(method, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-MBX-APIKEY", c.APIKey)
	return c.do(req)
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		apiErr := &APIError{Status: resp.StatusCode, Body: string(body)}
		_ = json.Unmarshal(body, apiErr)
		c.logger().Error("http_error", "status", resp.StatusCode, "body", string(body))
		return nil, apiErr
	}
	return body, nil
}

func Sign(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func (a OrderAck) AvgPrice() (float64, error) {
	quote, err := strconv.ParseFloat(a.CummulativeQuoteQty, 64)
	if err != nil {
		return 0, err
	}
	qty, err := strconv.ParseFloat(a.ExecutedQty, 64)
	if err != nil {
		return 0, err
	}
	if qty == 0 {
		return 0, fmt.Errorf("order %d executed qty is 0 status=%s", a.OrderID, a.Status)
	}
	return quote / qty, nil
}

func (a OrderAck) Fee() float64 {
	return a.FeeUSDT(0)
}

// FeeUSDT converts commission into USDT. BNB fees use markBNB; unknown assets are skipped.
func (a OrderAck) FeeUSDT(markBNB float64) float64 {
	var total float64
	for _, f := range a.Fills {
		v, _ := strconv.ParseFloat(f.Commission, 64)
		switch strings.ToUpper(f.CommissionAsset) {
		case "USDT":
			total += v
		case "BNB":
			total += v * markBNB
		}
	}
	return total
}
