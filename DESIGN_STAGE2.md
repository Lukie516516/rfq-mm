# DESIGN — Stage 2 Binance hedge

阶段 1 交付物保持独立：`cmd/sim`、`DESIGN.md`、`reports/stage1_*` 不改行为。阶段 2 只替换 `hedge.Executor` 的实现，并增加 `cmd/hedge`。

## 选定方案

- 行情：REST `bookTicker` + REST `exchangeInfo`
- 下单：Binance Spot **Testnet** `POST /api/v3/order`，默认 `MARKET`
- 默认 `dry-run`，只有 `-send` 才会出网下单
- 生产主网默认禁止下单。公钥行情在 testnet 不可达时，才回退到 `api.binance.com` 拉盘口和 filters

不选 WebSocket：对冲发生在 RFQ 成交之后，是一次事件，不是持续挂单。REST 足够证明 filters、签名、拒单和日志。报价延迟以后若要压到几十毫秒，再上 WS。

## 精度

理论数量不能直接下单。必须先读：

- `PRICE_FILTER.tickSize`
- `LOT_SIZE.stepSize / minQty / maxQty`
- `NOTIONAL` 或 `MIN_NOTIONAL`

数量向下对齐到 `stepSize`，再检查最小名义金额。最小可对冲数量取 `max(minQty, ceil(minNotional/price))`。

`internal/binance` 负责这段。`hedge.Live` 在出网前先 Align，非法单不会当成“成功对冲”写进账本。

## 已发生的真实请求

Testnet 成功对冲（`reports/stage2_order.txt`）：

- 订单 `4155714`，`MARKET BUY`，`FILLED`
- `0.007` BNB @ `762.39`，成交额 `5.33673` USDT
- 数量来自 `max(minQty, ceil(minNotional/ask))`，不是拍脑袋的 1 BNB

Testnet 主动失败（`reports/stage2_fail.txt`）：

- 故意发送 `quantity=1.0000001`
- 交易所返回 `HTTP 400 code -1013 Filter failure: LOT_SIZE`
- 本地 filters 在出网前也能拦下同一笔；这次是为了留下交易所原文

阶段 1 的 `FailOnce` 仍然只存在于模拟器里，用来讲库存 skew。

## 日志

每一步都打：

- `market_data`：bookTicker / exchangeInfo
- `hedge_plan`：理论数量 → 对齐后数量
- `order_request`：签名前的 query（不含 secret）
- `exchange_response` 或 `http_error`：原始 JSON

报告落到 `reports/stage2_*.txt`。

## 阶段复盘问题

### 1. 为什么不能直接按理论数量下单？

交易所按离散网格撮合。`1.0007` BNB 对 `stepSize=0.001` 非法；`0.001` BNB 在 BNB≈600 时名义金额约 0.6 USDT，过不了 `minNotional=5`。不对齐直接 `-1013`。理论仓位是连续的，交易所订单是离散的。

### 2. 哪些交易规则会导致下单失败？

- `LOT_SIZE`：数量不是 step 的倍数，或小于 minQty
- `PRICE_FILTER`：限价单价格不是 tick 的倍数（市价单主要看数量）
- `MIN_NOTIONAL` / `NOTIONAL`：价格 × 数量太小
- 余额不足、权限不够、签名错误、`recvWindow` 过期、交易对停牌、`-2010` 余额、`-1021` 时间戳
- 测试网和主网 filters 可能不一致，所以每次启动都重新拉 `exchangeInfo`

### 3. 实际 hedge price 为什么会偏离理论价格？

理论价用的是下单前的最优一档。市价单吃的是当时订单簿，可能更深、可能已经变了。还有手续费、部分成交、买卖价差。账本必须用成交回报的均价 `cummulativeQuoteQty / executedQty`，不能再用 bookTicker 的 ask/bid。

## 密钥

环境变量：

```
BINANCE_API_KEY
BINANCE_API_SECRET
BINANCE_BASE_URL   # 默认 https://testnet.binance.vision
```

不要把密钥写进仓库。没有密钥时，`public` / `dry-run` / 本地 `fail` 仍然可以交。

## 生产级补充

- `.env` 由 `internal/dotenv` 加载，已有环境变量优先，密钥不进日志
- 主网下单默认拒绝，必须 `ALLOW_PROD=1`
- 每笔对冲带 `newClientOrderId`，回报里记下 `orderId` / 滑点
- 一笔对冲一个 `clientOrderId`。超时先查单，查到就入账，禁止换单号重发
- `-1013` 立刻失败；`-1021` 可重试但不算“可能已经成交”
- 账本用成交均价、`executedQty`、折成 USDT 的手续费
- 下单前检查可用余额
- 出网前把订单持久化成 `NEW`/`SENT`（`reports/orders.json`）。重启只查未完结 `clientOrderId`
- 订单状态：`NEW → SENT → UNCERTAIN | PARTIAL | FILLED | DEAD`
- 真下单后核对交易所钱包和本会话 `hedge-binance` 成交
- 运行手册见 [OPERATIONS.md](OPERATIONS.md)
