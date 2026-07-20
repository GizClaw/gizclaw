# LoCoMo datasets

The bundled `locomo10_smoke.jsonl` is an adapted, noncommercial subset of the
SNAP Research LoCoMo benchmark from
<https://github.com/snap-research/locomo>. The original dataset is
`data/locomo10.json`; this repository keeps the official `locomo10` naming
prefix and adds `_smoke` so the subset cannot be mistaken for all 10
conversations.

The smoke subset contains `conv-30` sessions 1 through 3 (58 turns) and six QA
records whose evidence is entirely contained in those sessions. This preserves
real cross-session memory behavior while keeping the manual gate bounded.
Its conversation record requires at least one materialized fact per session so
a successful operation cannot silently drop an entire substantive session.

The source paper is *Evaluating Very Long-Term Conversational Memory of LLM
Agents* by Adyasha Maharana, Dong-Ho Lee, Sergey Tulyakov, Mohit Bansal,
Francesco Barbieri, and Yuwei Fang (ACL 2024).

This adapted dataset is distributed under the upstream
[CC BY-NC 4.0 license](https://creativecommons.org/licenses/by-nc/4.0/).
It is for noncommercial use only. The upstream license text is preserved in
`LICENSE.locomo.txt`; see `locomo10_smoke.manifest.json` for exact provenance
and changes.

The runner schema is newline-delimited JSON:

- one `conversation` record with ordered turns and stable evidence/session IDs;
- every turn carries its source speaker and normalized session timestamp;
- one `question` record per query with gold answers, evidence IDs, and tags.

The upstream session timestamps do not specify a timezone. The subset maps the
unchanged wall-clock date and time onto UTC solely as a deterministic Go
`ObservedAt` representation; the `Z` suffix does not claim a source timezone.

The dataset payload is stored with Git LFS. Run `git lfs pull` if the working
tree contains only an LFS pointer.
