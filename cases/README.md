# cases/ —— 语言无关的契约数据

这个目录放**不属于任何一种语言**的契约数据。它要入库，因为 Go 与 Rust 的测试读**同一份**
（见 `clip-sync/ARCHITECTURE.md` §5.4 与 §10.3）—— 复制一份到两边必然漂。

## protocol/ —— 从 Go 导出的 JSON fixture

**谁生成**：`cloud-clip/lib/protocol_fixture_test.go`（Go），用 `encoding/json`
直接序列化真实结构体。所以它就是**服务端真实吐出来的字节**，不是手抄的。

**谁消费**：`rust/crates/protocol/tests/go_fixtures.rs`（Rust），
读进来 → 反序列化 → 再序列化 → 与原文件深比较。

**为什么值得**：这套契约的权威是 `docs/api.md`，但字段名与 `omitempty` 的**实际效果**
只有 Go 的 `encoding/json` 说了算。「读代码觉得一致」和「真的一致」是两件事。

```bash
# 校验（改了 type.go 的字段名/omitempty 之后这个会红 —— 那是有意的，它是一次契约变更）
cd cloud-clip && go test ./lib -run TestProtocolFixtures

# 确认改动是有意的之后，重新生成
cd cloud-clip && UPDATE_FIXTURES=1 go test ./lib -run TestProtocolFixtures

# Rust 侧验收
cd rust && cargo test -p clip9-protocol
```

⚠️ **不要手工编辑这里的 JSON** —— 它是导出产物。要改就改 Go 的结构体，然后重新生成。

⚠️ 比的是 JSON **语义**（`serde_json::Value` 深比较），不是字节。两处刻意的字节差异：
Go 默认开 HTML 转义（`<` → `\u003c`），serde_json 不转义；以及 key 顺序。
两者都不是契约 —— 任何 JSON 解析器读出来都一样。

## share/ —— 从 Go 导出的分享令牌 fixture

**谁生成**：`cloud-clip/lib/share_fixture_test.go`（Go）。每组用例是「固定的配置 + 固定的
claims」，它把**派生出的签名密钥**（`keyHex`）和**签出来的 token** 一起导出。

**谁消费**：`rust/crates/core/tests/share_tokens.rs`（Rust）—— 逐字节复现密钥与 token，
并把 token 解析回同一份 claims。

**为什么值得**：分享令牌的形状（`base64url(json).base64url(hmac)`）、claims 的
`omitempty` 效果、**密钥派生时房间的升序** —— 这三样读代码都看不出来，
而任何一处对不上都会导致「Go 签的链接在 Rust 上全变废链」，而且要等用户点开旧链接才发现。

```bash
# 校验（红了先别改 fixture —— 它意味着所有已发出的分享链接会一起失效）
cd cloud-clip && go test ./lib -run TestShareTokenFixtures

# 确认改动是有意的之后，重新生成
cd cloud-clip && UPDATE_FIXTURES=1 go test ./lib -run TestShareTokenFixtures

# Rust 侧验收
cd rust && cargo test -p clip9-core
```

⚠️ 活的互验在 `rust/tools/compare-with-go.mjs` 里（起两个真实实例，各签一张，
看对方认不认）—— fixture 钉的是静态的那一半。

## actions.json —— 动作行为用例（还没建）

按 §5.4 的计划，动作测试用例会抽成**语言无关的 JSON**，Go 与 Rust 都读它 = 双跑验证：

```json
{ "action": "text.replace", "input": "axb",
  "params": { "mode": "text", "find": ".", "with": "-" }, "expect": "axb" }
```

它防的是「预览区替换了、定时任务没替换」那类错 —— 现在的契约测试**只验 id，验不了行为**。
等 `rust/crates/actions` 开始实现时再建。
