# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/) and the project uses SemVer
(pre-1.0: minor-looking features ship under `v0.0.x` patch tags). Releases
before v0.0.31 were tag-only; this file starts at v0.0.31.

## v0.0.31 - 2026-06-29

Markdown-native authoring: content lives in a sidecar `demo.md`, behavior stays
in Go, and a new CLI scaffolds and migrates walkthroughs. Additive — existing
inline demos are unaffected. Full notes: [docs/releases/v0.0.31.md](docs/releases/v0.0.31.md).

### Added

- Verbatim blocks in sidecar markdown via `verbatim="..."` fence attributes, single and multi-variant. (PR 72)
- `harness` package: `harness.Run(demo)` / `harness.SetupRenderer(demo)` wire the renderer/mode and `--doc bundle`/`--serve` in one call. (PR 73)
- `harness.WireRecipe` / `harness.SplitLines` authoring helpers. (PR 73)
- `--doc sidecar` renderer (`Demo.Sidecar()`): emit a demo's content as sidecar markdown, the inverse of `FromMarkdown`. (PR 74)
- `demokit` CLI: `init` and `new` scaffolding (`--kind=narrated|live|branching`) and `extract` for migrating Go walkthroughs. (PR 74)

### Changed

- `FilterArgs` strips demokit's complete flag set (`--record`, `--replay`, `--out`, `--serve`, `--input-timeout` added), drift-guarded against `RegisterFlags`. Explicit `BoolFlag("--serve")` still wins, so existing usage is unaffected. (PR 73)
- `examples/graph` and `examples/dungeon` use `harness.Run` (and gain `BorderHorizontalOnly` output). (PR 73)
