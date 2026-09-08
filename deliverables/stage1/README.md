# RFQ Market Making — Stage 1

本地 RFQ 做市闭环：成本加成报价、库存 skew、风控拒单、对冲失败残留、已实现/未实现 PnL。

标的 BNB/USDT。对冲目标是净库存为 0，模拟吃 Binance 对手价。阶段 2 才真下单。

## 依赖

- Go 1.25+（本仓库 `tools/go`，不改系统 PATH 也能跑）
- 默认不需要网络
- `-live` 才请求 Binance REST `bookTicker`

## 如何运行

```powershell
$env:GOROOT = "$PWD\tools\go"
$env:Path = "$PWD\tools\go\bin;$env:Path"

go test ./cmd/... ./internal/...
go run ./cmd/sim
```

或：

```powershell
.\run.ps1
.\run.ps1 -live
```

只跑一条场景：

```powershell
go run ./cmd/sim -scenario happy
go run ./cmd/sim -scenario stress
```

报告写到 `reports/stage1_<scenario>.txt` 和 `.json`。

## 关键参数

| 项 | 默认 | 含义 |
|---|---|---|
| `profit_pad` | `0.15` | 覆盖交易所半价差和双边手续费之后的利润垫 |
| `skew_per_bnb` | `0.05` | 每 1 BNB 净库存把 reservation 移动多少 |
| `max_order_qty` | `5` | 单笔上限 |
| `max_abs_inventory` | `2` | 净库存上限 |
| `-live` | `false` | 用真实行情，仍模拟成交 |
| `-scenario` | `all` | `happy` / `stress` / `all` |

默认 mock：`bid=600.00 / ask=600.10`。Happy path 两边各锁 `+0.15`。

## 目录

```
cmd/sim/             场景入口和报告
internal/market/     mock / REST bookTicker
internal/quote/      成本加成 + inventory skew
internal/risk/       数量和库存限制
internal/rfq/        taker → MM 成交
internal/hedge/      把净库存收到 0；可注入失败
internal/ledger/     均价、已实现、未实现
internal/engine/     串一轮：报价 → 风控 → 成交 → 对冲
DESIGN.md            决策和复盘问题
reports/             最近一次运行输出
```

设计说明见 [DESIGN.md](DESIGN.md)。
