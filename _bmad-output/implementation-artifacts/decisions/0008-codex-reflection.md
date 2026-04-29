# 0008 Codex Reflection

## 0. 对另一份 ADR 的差异预判

我没读另一份 `0008-error-protocol.md`，但基于这些 lesson 的写法，我预判 Claude 那份大概率会把重点放在“把 transient / terminal 二分正式化”以及“把 mapper / state machine / TerminalErrorView 这条链条整理成一致协议”。它大概率会比我更愿意把 round 9-11 提炼出的判则上升为正统结论。相应地，我怀疑它会漏掉两类东西：第一，`try?`、`message-only state`、`response.Error` 这种“信息压扁”才是更底层根因，二分判则只是事后补丁；第二，silent relogin 的存在本身可能就是复杂度放大器，而不是必须守护的能力。换句话说，它可能更像“把现有解做严密”，我更在意“现有解是不是该被缩减”。

## 1. 22 条 lesson 的根因

我认为根因至少有五层，不止“协议层缺 metadata”。

- **根因 1：错误语义在多个边界被主动压扁。**
  - `ErrorPresentation` 被压成 `String`
  - `try?` 把 throw 与 nil 压成同一路
  - unknown enum 被 `?? default` 吃掉
  - server middleware 自己写 envelope，绕过 canonical producer
  - 这说明系统更偏好“先跑起来”，再靠下游 mapper / review 补语义

- **根因 2：恢复责任没有被单点定义。**
  - 谁负责 retry
  - 谁负责 cold-start
  - 谁负责 terminal fallback
  - 谁负责 canonical error decision
  - 这些责任在 5.4 / 5.5 中不断跨层漂移

- **根因 3：把 UI mode 当成协议层修复器。**
  - 许多轮 fix 不是先修 error type，而是先修 AlertOverlay / RetryView / TerminalErrorView 的分发
  - 这会把本该在类型层解决的问题，下沉成 UI 行为补丁

- **根因 4：实现策略偏 patch-driven，不偏 invariant-driven。**
  - 先解决当前 review finding
  - 再在下一轮补上一轮遗漏
  - 这会自然诱发“修一条、破一条”

- **根因 5：代码没有真正承载规则，注释和 lesson 承载了规则。**
  - 这是最不舒服的一点
  - 现在很多正确性来自“有人记得 round 8 为什么推翻 round 7”，而不是来自更硬的接口

## 2. Story 5.5 11 轮 fix-review 的真因

我不认为单一归因够用。权重从高到低是：

1. **错误模型设计问题**
   - 最大头
   - 尤其是 lossy case 设计、message-only state、fallback 漂出主判则
2. **fix-review 流程问题**
   - review 是 finding-driven，修复也跟着 finding-driven
   - 缺少每轮后的“全路径语义回放”
3. **dev-story 第一遍质量问题**
   - 有，但不是主因
   - 第一版有坑很正常，真正异常的是 11 轮里坑的类型高度重复
4. **tool / skill 设计问题**
   - 有辅助责任
   - distill / lesson 机制强化了“把问题写清楚”，但没有强制“把规则压进类型系统”

如果只选一个，我选 **错误模型设计问题**。流程和工具是在放大这个问题，不是在制造这个问题。

## 3. “transient vs terminal”二分本身是否成立

我认为它**在当前 UI 层是成立的工作启发式**，但**不应被轻率升级为协议真理**。

如果它是错的，至少可能错在两种地方：

- **候选错误 1：真正的维度不是 transient / terminal，而是“恢复责任归属”。**
  - local store failure 该谁恢复
  - server 401 该谁恢复
  - reset 后无 token 该谁恢复
  - 这些问题的关键不是“暂时还是永久”，而是“应该走 silent relogin、cold-start、引导态还是直接 surface”

- **候选错误 2：真正的维度不是错误本身，而是“当前产品状态允许什么退出路径”。**
  - bootstrap 阶段没有上一屏
  - in-app feature error 有上一屏
  - 同一个错误在 bootstrap 与普通 feature flow 上，允许的 UI mode 可能不同
  - 这时 transient / terminal 就不是协议维度，而是路由维度

- **候选错误 3：它把“retry 可能有用”和“用户值得被提供 retry 入口”混成一件事。**
  - 有些错误严格说存在 transient 子集，但从产品角度仍不值得让用户反复试
  - 反过来，有些错误理论上 terminal，但给用户一次手动 retry 也比静态死路更好
  - 所以它同时掺了技术概率判断和 UX 宽容判断

我的结论：这套二分现在可以用，但要把它标成 **policy heuristic**，不要标成 **protocol ontology**。

## 4. silent relogin 是否值得保留

站在“砍掉 silent relogin”的立场，我认为这东西在 MVP 阶段很可能是过度工程化。为了掩盖一次 401，系统现在引入了 decorator、coordinator、single-flight、generation snapshot、stale caller dedup、失败清缓存、本地态与 server 态拆分、reset 语义保护等一整套复杂机制。它确实严谨，但严谨地服务了一个不高频、且完全可以被更粗粒度流程吸收的问题：用户 token 失效后重新建身份。MVP 更合理的策略可能是承认 401 是会话断裂，直接把用户送回启动恢复流或 guest login 重建流，而不是试图把“无感恢复”做到并发安全和多层语义完备。现在的复杂度不像在买核心业务能力，更像在买“少弹一次错误 UI”的体面；这笔复杂度成本不划算。

## 5. lesson 沉淀机制本身的问题

我的判断偏负面。

- **它确实能防 regress。**
  - 因为很多坑太具体了，不写就忘
  - 比如 generation snapshot、try? bridging、queue onRetry loss，这些都是高价值细节

- **但它也明显在滑向“代码注释病”的外部化版本。**
  - 88 条 lesson / 4 天 / 22 条每天
  - 单条 2000-4000 tokens
  - 这已经不是“经验库”，而是在用大量文字托住一套还没被结构化消化的设计

- **最大问题不是多，而是症状重复。**
  - 很多 lesson 本质上都在说：不要压扁语义；不要 patch 一层忘一层；不要让 fallback 漂出判则
  - 这说明 lesson 在重复驯化代码，而不是代码在吸收 lesson

所以我认为 lesson 机制是**有效但危险**。它短期有用，长期如果不把高频 lesson 反向沉淀成类型、接口、测试模板、lint 规则、代码生成模板，它就会变成“把架构债写成文档”的高级形式。

## 6. Claude 未必会发现的盲点

我猜另一份 ADR 最可能漏掉的一点是：

- **`AppErrorMapper.swift` 现在已经不是 mapper，而是制度补丁汇编。**
  - 如果一份 ADR 只是继续强化它的 centrality，会把问题解决成“让 mapper 更权威”
  - 但我认为更大的问题是：为什么这么多规则只能活在 mapper 注释里
  - 换句话说，真正的盲点不是“分类表还不够完整”，而是“分类表为什么必须这么长”

这是我最想提醒的一点：别把 `AppErrorMapper` 的膨胀误认成架构收敛。

## 7. 我对当前架构的独立反思

- server 侧已经有接近成熟的 error protocol：单一生产者、显式 canonical key、多消费者只读。
- iOS 侧还没有同等级的协议，只是拼出了一条可工作的恢复链。
- `transient / terminal` 在 iOS 侧确实帮助止血，但它更像**战地分类法**，不像**总设计语言**。
- silent relogin 的实现质量并不差，甚至相当细。但实现质量高，不等于需求本身值得。
- 最大的未来风险不是再出一个 1009 / `.unauthorized` 归类 bug，而是团队继续相信“只要把 lesson 写细，系统就稳了”。不会。系统稳要靠更少的语义压扁点。
