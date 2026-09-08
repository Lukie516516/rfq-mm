package binance

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type OrderVenue interface {
	PlaceMarketID(symbol, side string, qty, step float64, clientOrderID string) (OrderAck, error)
	QueryOrder(symbol, clientOrderID string) (OrderAck, error)
}

func (c *Client) QueryOrder(symbol, clientOrderID string) (OrderAck, error) {
	q := url.Values{}
	q.Set("symbol", symbol)
	q.Set("origClientOrderId", clientOrderID)
	c.logger().Info("order_query", "symbol", symbol, "clientOrderId", clientOrderID)
	body, err := c.signed("GET", "/api/v3/order", q)
	if err != nil {
		return OrderAck{}, err
	}
	var ack OrderAck
	if err := json.Unmarshal(body, &ack); err != nil {
		return OrderAck{}, fmt.Errorf("decode query ack: %w body=%s", err, body)
	}
	c.logger().Info("order_query_hit", "orderId", ack.OrderID, "status", ack.Status, "executedQty", ack.ExecutedQty)
	return ack, nil
}

// SubmitMarket places once under clientOrderID. An uncertain send is queried
// before any second place, so a timeout cannot open a duplicate hedge.
func SubmitMarket(v OrderVenue, symbol, side string, qty, step float64, clientOrderID string, tries int, sleep func(time.Duration)) (OrderAck, error) {
	if tries <= 0 {
		tries = 3
	}
	if sleep == nil {
		sleep = time.Sleep
	}
	var last error
	for i := 0; i < tries; i++ {
		ack, err := v.PlaceMarketID(symbol, side, qty, step, clientOrderID)
		if err == nil {
			return ack, nil
		}
		last = err
		if DuplicateOrder(err) {
			if qack, qerr := v.QueryOrder(symbol, clientOrderID); qerr == nil {
				return qack, nil
			}
			return OrderAck{}, err
		}
		if Uncertain(err) {
			if qack, qerr := v.QueryOrder(symbol, clientOrderID); qerr == nil {
				return qack, nil
			} else if !NotFound(qerr) && !Retryable(qerr) {
				return OrderAck{}, fmt.Errorf("uncertain send and query failed: place=%v query=%w", err, qerr)
			}
			last = fmt.Errorf("uncertain send, order not visible yet: %w", err)
		} else if !Retryable(err) {
			return OrderAck{}, err
		}
		if i+1 < tries {
			sleep(time.Duration(i+1) * 200 * time.Millisecond)
		}
	}
	if qack, qerr := v.QueryOrder(symbol, clientOrderID); qerr == nil {
		return qack, nil
	}
	return OrderAck{}, fmt.Errorf("submit retries exhausted: %w", last)
}

func (c *Client) PlaceAndReconcile(symbol, side string, qty, step float64, clientOrderID string) (OrderAck, error) {
	return SubmitMarket(c, symbol, side, qty, step, clientOrderID, 3, time.Sleep)
}

func Uncertain(err error) bool {
	if err == nil {
		return false
	}
	var api *APIError
	if errors.As(err, &api) && api.Status >= 500 {
		return true
	}
	if timestampError(err) {
		return false
	}
	return Retryable(err)
}

func timestampError(err error) bool {
	var api *APIError
	return errors.As(err, &api) && api.Code == -1021
}

func NotFound(err error) bool {
	var api *APIError
	return errors.As(err, &api) && api.Code == -2013
}

func DuplicateOrder(err error) bool {
	if err == nil {
		return false
	}
	var api *APIError
	if errors.As(err, &api) {
		if api.Code == -2010 || api.Code == -4131 {
			return strings.Contains(strings.ToLower(api.Msg), "duplicate") || api.Code == -4131
		}
		return strings.Contains(strings.ToLower(api.Msg), "duplicate")
	}
	return strings.Contains(strings.ToLower(err.Error()), "duplicate")
}
