# PR 与 Commit 格式

Review-ready PR 和 commit history 必须使用一致、可扫描的标题格式。格式只描述交付物，不记录临时工作过程。

## PR 标题

PR 标题统一使用：

```text
prefix: subject
```

标题必须能够自然组成下面这句话，并且准确描述完整 diff：

```text
This PR changes {prefix} to {subject}.
```

规则：

- `prefix` 表示被改变的主要 ownership 或 contract，使用小写字母、数字、连字符或 `/`，不能包含空格、空 segment 或首尾 `/`。
- 冒号后保留一个空格。
- `subject` 使用小写开头的原形动词短语，必须能够直接接在 `to` 后面，并描述合并后的最终结果。
- 不使用句号结尾，不把 Issue/PR 编号写进标题，不保留 `WIP`、`draft`、`fixup`、`address review` 或 `misc changes` 等过程描述。
- 一个 PR 只有一个主要 prefix。跨模块改动使用拥有 source contract 的模块；仓库级改动使用 `repo`。

示例：

```text
apps/mobile: add app-wide localization support
api: add source-qualified workflow RPC
guides/reviewing: document PR and commit format
repo: close deterministic validation gaps
```

这些标题分别可以读作：

```text
This PR changes apps/mobile to add app-wide localization support.
This PR changes api to add source-qualified workflow RPC.
This PR changes guides/reviewing to document PR and commit format.
This PR changes repo to close deterministic validation gaps.
```

## Commit 标题

每个 review-ready commit 使用与 PR 相同的格式：

```text
prefix: subject
```

PR 标题中关于 `prefix`、`subject`、大小写、ownership 和最终结果的全部规则原样适用于 commit。Commit 标题必须准确描述该 commit 的 diff，不能为了匹配 PR 标题，把多个无关 ownership 或独立目标塞入同一 commit。

Commit 标题使用相同的句子测试：

```text
This commit changes {prefix} to {subject}.
```

## Commit 内容

- 一个 commit 只拥有一个可独立解释的目的；无关修复、格式化、生成产物或依赖变化应拆开。
- 标题已经完整表达原因和结果时，可以不写 body。
- 需要 body 时，标题后空一行，先解释为什么需要改变，再说明不可从 diff 直接得出的行为、边界或兼容影响。
- 较大 commit 可以使用 `Summary`、`Impact` 和 `Validation` 等小节；验证命令必须是真实执行的命令，不能把未运行的检查写成通过。
- Source Schema 和由它产生的 committed generated outputs 应在同一原子 change 中，除非 Issue 明确要求分阶段交付。
- 临时 `fixup!`、`squash!`、`WIP`、merge residue 和只写“address review comments”的 commit 不得出现在 review-ready history 中。
- 不写 secret、credential、私人路径、机器状态、日志转储或与变更无关的工具署名。

示例：

```text
guides/reviewing: document PR and commit format

Define the title contract used by review-ready PRs and commits so reviewers
can reject ambiguous or process-only history before merge.
```

## Review 检查

PR 进入 review-ready 或 merge 状态前确认：

- PR 标题符合 `prefix: subject`，并准确覆盖最终 diff；
- 每个保留的 commit 标题和边界符合本规范；
- review 修改已经整理进有意义的 commit，不保留临时过程 commit；
- squash merge 使用的最终标题仍符合本规范，GitHub 自动附加的 PR 编号不属于标题输入。
