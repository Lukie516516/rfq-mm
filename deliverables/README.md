# Frozen deliverables

阶段 1 和阶段 2 分开存放。后面改阶段 2，不要覆盖这里的阶段 1 副本。

## 阶段 1（已冻结）

可运行入口仍是仓库根目录的 `cmd/sim` 和 `.\run.ps1`，行为不应再改。

本目录是评审用快照：

- `stage1/DESIGN.md` — 报价、对冲映射、仓位、PnL、四个复盘问题
- `stage1/README.md` — 阶段 1 运行说明
- `stage1/reports/stage1_all.txt` — happy path + stress 完整输出
- `stage1/reports/stage1_all.json` — 同一份结构化结果

根目录 `reports/stage1_*` 若被重新跑模拟器覆盖，以这里为准。

## 阶段 2

入口：`cmd/hedge`、`.\run_stage2.ps1`  
文档：`DESIGN_STAGE2.md`  
报告：`reports/stage2_*.txt`（不会写成 `stage1_*`）
