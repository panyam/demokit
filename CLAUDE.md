# CLAUDE.md — demokit

Light router. Substantive content lives elsewhere:

- **What demokit is, how to use it** → [README.md](README.md)
- **Internal architecture, file map, gotchas** → [ARCHITECTURE.md](ARCHITECTURE.md)
- **A working branching demo** → [examples/graph/main.go](examples/graph/main.go)

## Must-know

- It's a **directed graph**, not a DAG — cycles are allowed. `Demo.MaxSteps` and `Demo.MaxVisits` are the termination guardrails. Don't remove either.
- Doc renderers (markdown, HTML, JSON) are **pure functions of `RenderContext{Demo, Trace, State}`** — they never invoke `Run()`. Maintain that invariant when adding new formats.
- Static `--doc md` and trace `--doc md --from` route to **different renderers** by design (rich visitor vs. per-entry layered). Don't unify them; see ARCHITECTURE.md for why.

```bash
go test ./...
go run ./examples/graph/                                  # interactive
go run ./examples/graph/ --doc md --from /tmp/run.json    # docs from a trace
```
