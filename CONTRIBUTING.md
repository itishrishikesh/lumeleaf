# Contributing to Lumeleaf

Lumeleaf prioritizes measured responsiveness, correctness, accessibility, and a small dependency surface.

1. Discuss large behavior or architecture changes before implementing them.
2. Keep I/O and parsing off the UI thread; bound queues and make work cancellable.
3. Add tests for behavior and benchmarks for performance-sensitive paths.
4. Run `make verify` and `make bench-check` before submitting a change.
5. Do not weaken Unicode handling, safe saves, trust boundaries, or accessibility to meet a benchmark.

By contributing, you agree that your contribution is licensed under MIT.
