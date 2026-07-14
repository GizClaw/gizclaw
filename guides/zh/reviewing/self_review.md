# 开发后自我审查

自我审查发生在实现完成后、提交或请求远程 Review 前。它不是快速浏览 diff，而是一次 `review → fix → verify → re-review` 闭环。

## 1. 重新读取需求

回到 Issue、设计文档或用户请求，逐项确认：

- 实现覆盖了哪些 acceptance criteria；
- 哪些内容明确不在本次 scope；
- 是否在开发过程中改变了 API、目录或行为假设；
- 文档是否已经反映最终实现。

如果实现与需求发生偏离，应先修正实现或更新得到认可的需求，不能让 PR reviewer 猜测最终目标。

## 2. 建立改动地图

按 changed folder 和 ownership 分组，而不是从第一个 diff 一路读到最后：

```text
Schema / Contract
Generated surfaces
Go services and packages
SDK and applications
Tests and fixtures
Guides and workflows
```

标出每个 source contract 的全部 consumer，确保生成代码和调用方没有遗漏。

## 3. 逐模块检查与修复

对每组改动使用[审查项目](./review_items)和对应[编码规范](../coding-styles/)。发现问题后直接修复，并增加能够证明问题不会回归的测试。

优先检查：

- 正确性、失败路径和边界值；
- ownership、取消、关闭和 partial cleanup；
- public API 与 package boundary；
- Contract 与生成文件一致性；
- 不可信输入与跨语言转换。

## 4. 验证

执行最能证明改动正确的命令，而不是只运行最容易通过的命令。

- Go 行为变化默认运行 `go test ./...`。
- 并发改动根据风险增加 `go test -race`。
- Schema 变化先重新生成，再运行所有受影响 SDK 和调用方测试。
- JavaScript、Dart/Flutter、C 和 Wails 使用各 package 已定义的 build/test。
- 文档至少运行 `git diff --check` 和对应站点 build。

记录命令、结果以及没有运行的验证和原因。

## 5. Fresh review

修复完成后重新从完整 diff 开始审查，不能只看刚修过的行。重复这一过程，直到新一轮不再产生新的 blocking finding。

结束前确认：

- 每个 changed folder 都已经审查；
- 每个 Contract consumer 都已经检查；
- 测试覆盖与改动风险匹配；
- diff 中没有临时文件、调试代码和无关修改；
- 验证结果对应当前最终代码，而不是较早版本。
