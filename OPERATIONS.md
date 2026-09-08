# 运行手册

把它当交易桌，不当演示脚本。密钥不进 git。阶段 1 冻结副本在 `deliverables/stage1/`。

## 日常怎么跑

```powershell
.\run.ps1                  # 阶段 1：happy / stress / guards
.\run_stage2.ps1           # 阶段 2：public，不下单
.\run_stage2.ps1 -Mode dry-run
.\run_stage2.ps1 -Mode order -Send   # 仅 Testnet，且 .env 已填
```

Kill switch：在仓库根放一个文件 `KILL`，然后把 `engine.Config.KillSwitch` 指过去。文件在就拒报，不改账。

## 出事前先看什么

1. `reports/journal_stage1.jsonl` / `journal_stage2.jsonl` — 每一轮决策
2. 最新 `reports/stage2_*.txt` — 行情、filters、订单原文
3. 账本：`bnb` 是否还在漂，`realized` 是否穿了日亏损

对冲失败后不要继续对称报价。库存 skew 会自动偏，但连续失败应靠 `MaxUnhedged` 停手。

## 错误怎么分类

| 现象 | 是否重试 | 动作 |
|---|---|---|
| 网络超时、5xx | 先查单 | 同一 `clientOrderId` 查询；查到就入账，查不到再发 |
| `-1013` LOT_SIZE / NOTIONAL | 否 | 修精度，禁止原样重发 |
| `-2015` 签名 / 权限 | 否 | 停，查 key |
| `-2010` 余额 | 否 | 停一边，补库存 |
| 行情过期、盘口过宽 | 不下单 | 拒报 |

主网默认禁止下单。要动真钱必须同时：`BINANCE_BASE_URL` 指向主网，且 `ALLOW_PROD=1`。作业和日常调试都不要开。

## 对账

一笔对冲只用一个 `clientOrderId`。发送超时后先 `GET /api/v3/order`，禁止换新单号重发。

本地先把订单写成 `NEW` / `SENT`（`reports/orders.json`），再出网。重启时 `RecoverPending` 只查这些未完结单号。状态：`NEW → SENT → UNCERTAIN|PARTIAL|FILLED|DEAD`。

```
fill_px = cummulativeQuoteQty / executedQty
slippage = fill_px - theory_px
fee_usdt = USDT 手续费 + BNB 手续费 * mid
```

账本只记 `executedQty`、`fill_px`、折算后的 `fee_usdt`。部分成交的剩余库存留在账上，下一轮继续朝 0 收。下单前会查可用 USDT/BNB。

真下单后会把交易所钱包和**本会话**的 `hedge-binance` 成交对一下。RFQ 成交不在钱包上，不参与对账。

## 值班数字

`reports/desk_stage1.json` / `desk_stage2.json`：拒单率、对冲失败率、未对冲秒数、均滑点、价差/手续费/库存 PnL。

默认关闭、验收数字不变的开关：

| 开关 | 默认 | 作用 |
|---|---|---|
| `-slip-ticks` | 0 | 对冲价往不利方向走 n 个 tick |
| `-vol-pad` | 0 | 半价差再加一层波动垫 |
| `-ttl` / `-hit-delay` | 0 | 报价过期；模拟报价到成交的间隔 |

## 已知生产缺口

- 没有独立的日切和多进程锁。单进程、单标的。
- 钱包对账用 free 余额，冻结资金和非本会话成交会表现为 drift，要人工看。
