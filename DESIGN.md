# DESIGN — Stage 1 RFQ Simulator

阶段 1 只做本地闭环：行情 → 报价 → 风控 → 模拟成交 → 模拟对冲 → 更新仓位 → 拆开已实现/未实现 PnL。不接下单，不碰链上。

## 选定方案

现货对冲，目标库存为 0。模拟在 Binance 吃对手价。

不选永续：阶段 1 要证明方向和记账，funding / basis / 保证金会把主线搅乱。永续作为阶段 2 之后的备选写在文末。

对冲对象是 **RFQ 之后的净库存**，不是机械反转上一笔成交。对冲失败留下库存后，下一笔 RFQ 可能已经把净敞口打平；这时再反转上一笔，等于重新开仓。

## 模块

```
market.Source  →  quote  →  risk  →  rfq fill  →  hedge.Executor  →  ledger
```

`hedge.Executor` 和 `market.Source` 是阶段 2 的替换点。阶段 1 用 `Immediate` / `FailOnce` 和 mock（或 REST bookTicker）。

## 报价逻辑

```
venue_half   = (binance_ask - binance_bid) / 2
half_spread  = venue_half + 2*fee_per_unit + profit_pad
reservation  = mid - skew_per_bnb * inventory
rfq_bid      = reservation - half_spread
rfq_ask      = reservation + half_spread
```

`2*fee`：RFQ 和 hedge 各付一次。`profit_pad` 默认 0.15。验收行情 `600.00 / 600.10` 时 `half=0.20`，报价 `599.85 / 600.25`，和手算一致。

库存 skew：多头把 reservation 下移，更愿意卖、更不愿意买；空头相反。对冲失败后，下一笔报价必须看见这个偏斜。这不是装饰，是作业第 4 问的代码答案。

报价本身不改账本。

## Hedge 逻辑

记账只用 MM 自己的方向。

| Taker | MM 在 RFQ 上 | RFQ 后库存 | 风险 | 目标为 0 时的 hedge |
|---|---|---|---|---|
| buy BNB | sell @ RFQ ask | BNB 下降 | 空头，怕涨 | 若仍空：买 @ ask |
| sell BNB | buy @ RFQ bid | BNB 上升 | 多头，怕跌 | 若仍多：卖 @ bid |

已经为 0 则不下对冲单。`FailOnce` 用来制造「成交了但对冲超时」，库存留在账上，后续 skew / 风控生效。

## 仓位更新

```
MM buy : usdt -= price*qty + fee ; 库存 +qty
MM sell: usdt += price*qty - fee ; 库存 -qty
```

加仓用加权平均价。减仓按均价实现盈亏：

- 平多：`(exit - avg) * qty`
- 平空：`(avg - exit) * qty`
- 手续费从已实现里扣

允许现金和币暂时为负。这是风险账，不是钱包。

## PnL

```
unrealized = bnb * (mid - avg)     // 空头时 bnb 为负，公式仍然成立
equity     = usdt + bnb * mid
```

全量对冲且 fee=0 时：

- Taker buy：`rfq_ask - binance_ask` → `+0.15`
- Taker sell：`binance_bid - rfq_bid` → `+0.15`

对冲完成前不要只看现金。未对冲的 1 BNB 空头，现金很多，权益里已经扣了回补成本。

## 风控（阶段 1 有意做薄）

- 单笔数量上限
- 净库存绝对值上限
- 碰到上限：多头不再买、空头不再卖；拒单不改账本

延迟用 `-slip-ticks` 仿真（默认 0，验收数字不变），不是订单簿路径。部分成交：阶段 1 仍是整笔假成交；阶段 2 按交易所 `executedQty` 入账。没有做：换新单号重报、链上 gas 实扣。

## 阶段复盘问题

### 1. Taker buy / taker sell 分别对应 MM 承担什么方向的风险？

Taker buy：我们卖出 BNB，变成空头。价格上涨则补货更贵。

Taker sell：我们买入 BNB，变成多头。价格下跌则库存贬值。

### 2. Hedge 为什么是这个方向？

对冲的是净 delta，不是跟用户同向，也不是猜涨跌。缺币就去 Binance 买，多币就去卖。方向反了是在加仓。

### 3. Spread 收入与 hedge 成本的关系是什么？

RFQ 价差是毛利。Hedge 在 Binance 是 taker，必须吃对手价，再加两边手续费、滑点、延迟。`half_spread` 必须先覆盖 `venue_half + 2*fee`，剩下才是 `profit_pad`。验收里 pad=0.15，venue_half=0.05，所以每边锁 0.15。价差不够覆盖交易所价差，对冲完就是亏的。

### 4. Inventory 为什么会影响报价？

对冲不是瞬时、也不是必成。失败或延迟时，账上已经有方向。多头下移买卖价，用价格把库存卖回去；空头上移。`scenario=stress` 里第一笔对冲失败后，第二笔 ask 会低于 600.25。这就是 skew 在干活。

## 已知边界

- 账本内部是 `int64`（1e8）。对外 `Book.BNB` / `USDT` 仍是 float，验收数字不变。
- 默认 mock 行情，保证数字可复现。`-live` 只拉 bookTicker。
- 对冲失败是注入的，不是交易所返回。阶段 2 要用真实拒单替换 `FailOnce`。
- 延迟成本用 `-slip-ticks` 仿真，默认 0，happy path 仍是 +0.15。
- 没有选永续对冲。省现货库存，但引入 funding 和 basis；现阶段不值得。

## 怎么跑给评审看

```powershell
.\run.ps1
```

看 `reports/stage1_all.txt`。Happy path 证明方向和 0.15；stress 证明失败、skew、拒单。

## 阶段 2（独立交付）

阶段 1 以上内容保持有效，入口仍是 `cmd/sim`。阶段 2 不改这套模拟账，只新增 `cmd/hedge` 和 [DESIGN_STAGE2.md](DESIGN_STAGE2.md)。

## 生产级补充（阶段 1 引擎里，默认关闭）

作业验收数字不变。下面这些开关默认是 0 / 空，happy path 不会碰到：

- 行情过期：`MaxBookAge`，book 必须带时间戳
- 盘口过宽：`MaxVenueSpread`，异常价差时拒报
- 日亏损：`MaxDailyLoss`，已实现亏穿就停
- 连续对冲失败：`MaxUnhedged`
- 熔断文件：`KillSwitch` 路径存在即拒报
- 审计：每轮写 JSONL，包含决策链
- 报价 TTL / 波动垫：`QuoteTTL`、`VolPad`，默认关
- 对冲滑点：`SlipTicks` + `TickSize`，默认 0
- 值班指标：拒单率、对冲失败、未对冲秒数、滑点、PnL 拆分

`.\run.ps1` 的 `guards` 场景会把前三条打出来。冻结的 `deliverables/stage1` 仍是原来的两场景快照。
