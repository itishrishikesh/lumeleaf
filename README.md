# Lumeleaf

**A fast, native, view-first workspace for understanding agent-written Java and Markdown.**

![Lumeleaf in the sepia theme](assets/screenshot.png)

Lumeleaf is an early, local-first reader for the part of software development that AI agents make more important: seeing exactly what changed, following structure, and deciding whether the result is right. It deliberately avoids an extension host, embedded browser, terminal, debugger, and built-in AI chat.

> **Development disclosure:** This project was generated with OpenAI Codex 5.6 Sol using high reasoning effort. It is primarily intended for the author's personal use. Review and test the code before relying on it.

## Current status

Lumeleaf is an **experimental 0.1 vertical slice**, not yet a replacement for a full IDE.

Implemented today:

- Native Gio window for Linux and macOS, plus a deterministic headless renderer.
- Direct Java and Markdown file opening with dark, light, and sepia palettes.
- Tree-sitter Java parsing, visible-range syntax spans, folds, and local symbols.
- Native Markdown document model with GFM tasks, tables, fenced code, and safe resource rules—no WebView.
- Revisioned Unicode document model, undo/redo, atomic conflict-aware saves, and UTF-16 position mapping.
- Memory-mapped, sparse-indexed large-file reader.
- Incremental workspace discovery, Quick Open ranking, streaming search through `rg` or a Go fallback, and disk reconciliation.
- Read-only Git status/diff parsing and local review state foundations.
- Lazy, trust-gated JDT LS protocol client foundations.
- Reproducible visual fixtures, race-tested packages, fuzz targets, and benchmark gates.

The native window currently provides the reading surface; several underlying services are not yet wired to every interactive control. See [Known limitations](#known-limitations).

## Why view-first?

Agent workflows produce changes faster than people can responsibly inspect them. Lumeleaf optimizes for opening first, reading without layout churn, moving through changed hunks, and noticing external edits. Language servers and repository analysis are optional background layers; they never block the first readable frame.

## Build

Requirements: Go 1.25+, Git, a C compiler, and Gio's platform libraries.

Fedora:

```bash
sudo dnf install gcc pkg-config wayland-devel libX11-devel libxkbcommon-x11-devel mesa-libGLES-devel mesa-libEGL-devel libXcursor-devel vulkan-headers mesa-dri-drivers
make build
/tmp/lumeleaf README.md
```

macOS:

```bash
xcode-select --install
make build
/tmp/lumeleaf README.md
```

For core development or servers without graphics headers:

```bash
make build-headless
/tmp/lumeleaf --render-fixture testdata/java/Reader.java --theme sepia --output /tmp/lumeleaf.png
```

## Keyboard direction

The command registry defines the intended stable shortcuts while UI wiring is completed:

| Action | Shortcut |
|---|---|
| Open file | `Ctrl/⌘ O` |
| Quick Open | `Ctrl/⌘ P` |
| Search repository | `Ctrl/⌘ Shift F` |
| Toggle Markdown reading | `Ctrl/⌘ Shift M` |
| Next / previous change | `F7` / `Shift F7` |
| Go to definition | `F12` |

## Privacy and trust

Source stays on your machine. Lumeleaf has no telemetry, account, cloud dependency, or bundled AI service. Opening an untrusted workspace never launches a language server, build wrapper, Git hook, formatter, remote image, or external link. JDT LS is an optional local process and is measured separately from the core reader.

## Performance

Run `make bench-check` for short gates and `make bench-full` for repeatable microbenchmarks. The methodology and absolute budgets live in [benchmarks/README.md](benchmarks/README.md) and [benchmarks/budgets.json](benchmarks/budgets.json). The project does not claim numbers that have not been measured on a named machine.

## Verification

```bash
make verify
make visual
make smoke-headless
make bench-check
```

`make smoke-linux` additionally exercises the Gio path under a headless Weston compositor. CI builds and tests Linux and macOS.

## Known limitations

- The editor surface is intentionally minimal and some workspace, Git review, and Markdown actions are currently exercised through package APIs/tests rather than complete GUI panels.
- JDT LS must already be installed; Lumeleaf does not download a JRE or language server.
- Large-file mode is read-only.
- macOS artifacts are unsigned and not notarized.
- Screen-reader semantics have a tested platform-neutral model, but native assistive-technology testing is incomplete.
- AppImage packaging and session-snapshot compression are planned after the vertical slice stabilizes.

## Project policy

Contributions are welcome under [CONTRIBUTING.md](CONTRIBUTING.md). Please report vulnerabilities according to [SECURITY.md](SECURITY.md). Participation is governed by [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

Lumeleaf is available under the [MIT License](LICENSE). Third-party licenses remain with their respective authors; see [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
