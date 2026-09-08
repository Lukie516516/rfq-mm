package binance

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type Account struct {
	Free map[string]float64
}

type accountResp struct {
	Balances []struct {
		Asset string `json:"asset"`
		Free  string `json:"free"`
	} `json:"balances"`
}

func (c *Client) Account() (Account, error) {
	body, err := c.signed("GET", "/api/v3/account", url.Values{})
	if err != nil {
		return Account{}, err
	}
	var raw accountResp
	if err := json.Unmarshal(body, &raw); err != nil {
		return Account{}, err
	}
	out := Account{Free: map[string]float64{}}
	for _, b := range raw.Balances {
		v, _ := strconv.ParseFloat(b.Free, 64)
		if v != 0 {
			out.Free[strings.ToUpper(b.Asset)] = v
		}
	}
	c.logger().Info("account", "assets", len(out.Free))
	return out, nil
}

func (a Account) FreeOf(asset string) float64 {
	if a.Free == nil {
		return 0
	}
	return a.Free[strings.ToUpper(asset)]
}

func (a Account) CheckHedge(side string, qty, price float64) error {
	switch strings.ToUpper(side) {
	case "BUY":
		need := qty * price * 1.01
		if a.FreeOf("USDT")+1e-12 < need {
			return fmt.Errorf("insufficient USDT: free=%.8f need=%.8f", a.FreeOf("USDT"), need)
		}
	case "SELL":
		if a.FreeOf("BNB")+1e-12 < qty {
			return fmt.Errorf("insufficient BNB: free=%.8f need=%.8f", a.FreeOf("BNB"), qty)
		}
	}
	return nil
}
