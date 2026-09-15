# Performance verification

Lumeleaf treats latency and memory as product behavior. `budgets.json` contains gates, not marketing results.

Run the short local gate with `make bench-check`. Run the repeatable microbenchmark set with `make bench-full`; output stays in `/tmp`. Compare two runs with `benchstat` and investigate statistically significant regressions above 10%.

Record the Go version, commit, OS, CPU, memory, renderer, scale, and filesystem beside any published result. Java analysis is built into Lumeleaf and is included in the core reader measurements.
