# RFQ Market Making

两阶段分开交付。改阶段 2 不会覆盖阶段 1。

| 阶段 | 入口 | 文档 | 冻结副本 / 报告 |
|---|---|---|---|
| 1 本地模拟 | `.\run.ps1` → `cmd/sim` | [DESIGN.md](DESIGN.md) | [deliverables/stage1](deliverables/stage1)、`reports/stage1_*` |
| 2 Binance 对冲 | `.\run_stage2.ps1` → `cmd/hedge` | [DESIGN_STAGE2.md](DESIGN_STAGE2.md) | `reports/stage2_*` |
| Case 2 代币化美股 | 无代码 | [CASE2.md](CASE2.md) | 策略 + 落地路径 |

下面先写阶段 1。阶段 2 在文末。

# Stage 1

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
go run ./cmd/sim -scenario guards
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
| `-scenario` | `all` | `happy` / `stress` / `guards` / `all` |
| `-slip-ticks` | `0` | 对冲往不利方向走 n tick；0 时仍锁 +0.15 |
| `-vol-pad` | `0` | 半价差额外波动垫 |
| `-ttl` | `0` | 报价存活时间；0 不过期 |

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
internal/money/      账本内部 1e8 整数
internal/store/      出网前订单日志
internal/metrics/    值班拒单/滑点/PnL
DESIGN.md            决策和复盘问题
reports/             最近一次运行输出（含 desk_*.json）
```

设计说明见 [DESIGN.md](DESIGN.md)。冻结输出见 [deliverables/stage1](deliverables/stage1)。

# Stage 2

把 `hedge.Executor` 换成 Binance Spot Testnet。阶段 1 的 `cmd/sim` 和 `Immediate` / `FailOnce` 不动。

```powershell
.\run_stage2.ps1
.\run_stage2.ps1 -Mode fail
.\run_stage2.ps1 -Mode dry-run
```

真下单（需要 testnet key，默认不开）：

```powershell
copy .env.example .env
# 填入 BINANCE_API_KEY / BINANCE_API_SECRET
.\run_stage2.ps1 -Mode order -Send
```

| 模式 | 要密钥 | 出网下单 | 作用 |
|---|---|---|---|
| `public` | 否 | 否 | 拉 bookTicker / filters，演示精度对齐 |
| `fail` | 否 | 否 | 制造 LOT_SIZE / MIN_NOTIONAL 失败 |
| `dry-run` | 否 | 否 | 用最小合法数量走完整 RFQ→hedge，不下单 |
| `order -send` | 是 | 是 | Testnet 最小量市价对冲 |

说明见 [DESIGN_STAGE2.md](DESIGN_STAGE2.md)。已发生的 Testnet 成交和 `-1013` 拒单都写在那份文档里。值班手册：[OPERATIONS.md](OPERATIONS.md)。

# Case 2

代币化美股 7×24 做市，只要思路，不写代码。文档：[CASE2.md](CASE2.md)。
