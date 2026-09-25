# Project Wiki Implementation Plan

> **For agentic workers:** Documentation-only execution plan for the current session.

**Goal:** Create an OKF-conformant `WIKI/` knowledge bundle that explains the current fast-router workspace, build/test workflow, architecture, implementation status, and known gaps.

**Architecture:** Keep the Wiki isolated under `WIKI/` with a root `index.md` and `log.md`, then split project knowledge by overview, workspace map, architecture, build/test, developer workflow, module guides, existing-doc index, and known gaps. Treat source code and `SOP/build.md` as implementation truth; mark aspirational or incomplete capabilities explicitly.

**Tech Stack:** Markdown + YAML frontmatter under OKF v0.1; Go 1.25; Zig 0.14+; Python POC; shell build scripts.

## Global Constraints

- Every non-reserved Wiki Markdown file has parseable YAML frontmatter with non-empty `type`.
- `WIKI/index.md` and `WIKI/log.md` remain reserved OKF files without frontmatter.
- No source code or runtime behavior changes.
- No `git push`; only local files may be created or modified.
- Distinguish implemented, mocked, planned, and historical capabilities.

### Task 1: Inventory source of truth

**Files:** Read `AGENTS.md`, `OKF-SPEC.md`, `SOP/build.md`, Go/Zig/Python source, tests, scripts, and existing docs.

- [x] Identify workspace areas and implementation boundaries.
- [x] Record build commands and verification targets.
- [x] Record known gaps and status discrepancies.

### Task 2: Generate Wiki bundle

**Files:** Create `WIKI/index.md`, `WIKI/log.md`, and focused concept documents under `WIKI/`.

- [x] Add OKF frontmatter to every concept document.
- [x] Link the documents from `WIKI/index.md`.
- [x] Include source/method/findings/limitations/conclusion in the project overview.

### Task 3: Validate

- [x] Parse/check every Wiki frontmatter block.
- [x] Check index links point to existing targets.
- [x] Run `go test ./...` and `go build ./cmd/fast-router`.
- [x] Inspect `git diff --check` and working-tree scope.
- [ ] `gofmt -l cmd router` is not clean; existing Go files are outside this documentation-only change and were not reformatted.
