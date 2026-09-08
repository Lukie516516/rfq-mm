package types

// Side is always the market maker's own buy/sell of BNB.
type Side string

const (
	SideBuy  Side = "buy"
	SideSell Side = "sell"
)

// TakerAction is the user's action on the RFQ.
type TakerAction string

const (
	TakerBuy  TakerAction = "taker_buy"
	TakerSell TakerAction = "taker_sell"
)
