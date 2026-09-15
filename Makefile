GO ?= go
GOCACHE ?= /tmp/lumeleaf-gocache
export GOCACHE

.PHONY: all build build-headless run verify test race visual smoke-headless smoke-linux bench-check bench-full fuzz-smoke security clean

all: build-headless

build:
	$(GO) build -buildvcs=false -trimpath -tags desktop -o /tmp/lumeleaf ./cmd/lumeleaf

build-headless:
	$(GO) build -buildvcs=false -trimpath -o /tmp/lumeleaf ./cmd/lumeleaf

run:
	$(GO) run -buildvcs=false -tags desktop ./cmd/lumeleaf $(FILE)

test:
	$(GO) test -shuffle=on -count=1 ./...

race:
	$(GO) test -race -shuffle=on -count=1 ./...

verify:
	$(GO) mod verify
	test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './.git/*'))"
	$(GO) vet ./...
	$(GO) test -shuffle=on -count=1 ./...
	$(GO) test -race -shuffle=on -count=1 ./...
	$(GO) test -tags headless -count=1 ./internal/ui/...
	$(GO) build -buildvcs=false -trimpath -o /tmp/lumeleaf-verify ./cmd/lumeleaf

visual:
	mkdir -p /tmp/lumeleaf-visual-check
	$(GO) run -buildvcs=false ./cmd/lumeleaf --render-suite --output-dir /tmp/lumeleaf-visual-check

smoke-headless:
	$(GO) run -buildvcs=false ./cmd/lumeleaf --smoke-script testdata/smoke/basic.json --report /tmp/lumeleaf-smoke.json
	test -s /tmp/lumeleaf-smoke.json

smoke-linux:
	command -v weston >/dev/null || { echo 'weston is required; install the Gio Linux prerequisites from README.md' >&2; exit 2; }
	./scripts/smoke-linux.sh

bench-check:
	$(GO) test -run '^$$' -bench 'Benchmark(VisibleLines|QuickOpenRank|ByteRuneUTF16|TreeSitterJavaInitial)' -benchtime=100ms ./internal/...
	$(GO) run -buildvcs=false ./cmd/lumeleaf bench --check --scenario core --output /tmp/lumeleaf-bench-check.json

bench-full:
	$(GO) test -run '^$$' -bench . -benchmem -count=10 ./internal/... > /tmp/lumeleaf-bench-current.txt
	$(GO) run -buildvcs=false ./cmd/lumeleaf bench --scenario full --output /tmp/lumeleaf-bench-full.json

fuzz-smoke:
	$(GO) test ./internal/document -run '^$$' -fuzz FuzzAtomicEdits -fuzztime=2s
	$(GO) test ./internal/encoding -run '^$$' -fuzz FuzzDecode -fuzztime=2s
	$(GO) test ./internal/largefile -run '^$$' -fuzz FuzzSparseIndex -fuzztime=2s
	$(GO) test ./internal/gitreview -run '^$$' -fuzz FuzzGitPorcelainParser -fuzztime=2s
	$(GO) test ./internal/java -run '^$$' -fuzz FuzzLSPFraming -fuzztime=2s

security:
	$(GO) vet ./...
	$(GO) run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
	$(GO) list -m -json all >/tmp/lumeleaf-modules.json
	$(GO) version -m /tmp/lumeleaf-verify

clean:
	$(GO) clean
