GOBUILD=go build -trimpath

ifeq ("$(shell go env GOARCH || true)", "arm64")
	# this makes it work better on M1 machines
	GODEBUG=asyncpreemptoff=1
endif

ALL_SPIES = "ebpfspy,rbspy,pyspy,dotnetspy,debugspy"
ifeq ("$(shell go env GOOS || true)", "linux")
	ENABLED_SPIES ?= "ebpfspy,rbspy,pyspy,phpspy,dotnetspy"
else
	ENABLED_SPIES ?= "rbspy,pyspy"
endif

ifeq ("$(shell go env GOOS || true)", "linux")
	THIRD_PARTY_DEPENDENCIES ?= "build-rust-dependencies build-phpspy-dependencies"
else
	THIRD_PARTY_DEPENDENCIES ?= "build-rust-dependencies"
endif

ARCHIVE_EXT ?= "a"

EMBEDDED_ASSETS ?= ""
EMBEDDED_ASSETS_DEPS ?= "assets-release"
EXTRA_LDFLAGS ?= ""

ifndef $(GOPATH)
	GOPATH=$(shell go env GOPATH || true)
	export GOPATH
endif

.PHONY: all
all: build

.PHONY: build
build:
	$(GOBUILD) -tags $(ENABLED_SPIES) -ldflags "$(EXTRA_LDFLAGS) $(shell scripts/generate-build-flags.sh $(EMBEDDED_ASSETS))" -o ./bin/pyroscope ./cmd/pyroscope

# see for context: https://github.com/golang/go/issues/42136#issuecomment-717684147
# .PHONY: ensure-modern-ld-version
# ifeq ("$(shell go env GOOS || true)", "linux")
# ensure-modern-ld-version:

# else
# ensure-modern-ld-version:
# 	@echo "Not on Linux, so not checking for modern ld version."
# endif



.PHONY: build-rbspy-static-library
build-rbspy-static-library:
	mkdir -p ./out
	$(GOBUILD) -tags nogospy,rbspy -buildmode=c-archive -ldflags "-compressdwarf=false $(EXTRA_LDFLAGS) $(shell scripts/generate-build-flags.sh $(EMBEDDED_ASSETS))" -o "./out/libpyroscope.rbspy.$(ARCHIVE_EXT)" ./pkg/agent/clib

.PHONY: build-pyspy-static-library
build-pyspy-static-library:
	mkdir -p ./out
	$(GOBUILD) -tags nogospy,pyspy -buildmode=c-archive -ldflags "-compressdwarf=false $(EXTRA_LDFLAGS) $(shell scripts/generate-build-flags.sh $(EMBEDDED_ASSETS))" -o "./out/libpyroscope.pyspy.$(ARCHIVE_EXT)" ./pkg/agent/clib

.PHONY: build-phpspy-static-library
build-phpspy-static-library:
	mkdir -p ./out
	$(GOBUILD) -tags nogospy,phpspy -buildmode=c-archive -ldflags "-compressdwarf=false $(EXTRA_LDFLAGS) $(shell scripts/generate-build-flags.sh $(EMBEDDED_ASSETS))" -o "./out/libpyroscope.phpspy.$(ARCHIVE_EXT)" ./pkg/agent/clib

.PHONY: build-release
build-release: embedded-assets
	EMBEDDED_ASSETS=true $(MAKE) build

.PHONY: build-rust-dependencies
build-rust-dependencies:
# relocation-model=pic
	cd third_party/rustdeps && RUSTFLAGS="-C target-feature=+crt-static" cargo build --release

.PHONY: build-phpspy-dependencies
build-phpspy-dependencies:
	cd third_party && git clone https://github.com/pyroscope-io/phpspy.git
	cd phpspy && USE_ZEND=1 make

.PHONY: build-third-party-dependencies
build-third-party-dependencies: $(shell echo $(THIRD_PARTY_DEPENDENCIES))

.PHONY: test
test:
	go test -race -tags debugspy ./...

.PHONY: server
server:
	bin/pyroscope server --log-level debug --badger-log-level error --storage-path tmp/pyroscope-storage

.PHONY: agent
agent:
	bin/pyroscope agent

.PHONY: install-web-dependencies
install-web-dependencies:
	yarn install

.PHONY: assets
assets: install-web-dependencies
	$(shell yarn bin webpack) --config scripts/webpack/webpack.dev.js

.PHONY: assets-watch
assets-watch: install-web-dependencies
	$(shell yarn bin webpack) --config scripts/webpack/webpack.dev.js --watch

.PHONY: assets
assets-release: install-web-dependencies
	rm -rf webapp/public
	NODE_ENV=production $(shell yarn bin webpack) --config scripts/webpack/webpack.prod.js

.PHONY: embedded-assets
embedded-assets: install-dev-tools $(shell echo $(EMBEDDED_ASSETS_DEPS))
	go run "$(shell scripts/pinned-tool.sh github.com/markbates/pkger)/cmd/pkger" -o pkg/server

.PHONY: lint
lint:
	go run "$(shell scripts/pinned-tool.sh github.com/mgechev/revive)" -config revive.toml -exclude ./pkg/agent/pprof/... -exclude ./vendor/... -formatter stylish ./...

.PHONY: lint-summary
lint-summary:
	$(MAKE) lint | grep 'https://revive.run' | sed 's/[ ()0-9,]*//' | sort

.PHONY: ensure-logrus-not-used
ensure-logrus-not-used:
	@! go run "$(shell scripts/pinned-tool.sh github.com/kisielk/godepgraph)" -nostdlib -s ./pkg/agent/profiler/ | grep ' -> "github.com/sirupsen/logrus' \
		|| (echo "\n^ ERROR: make sure ./pkg/agent/profiler/ does not depend on logrus. We don't want users' logs to be tainted. Talk to @petethepig if have questions\n" &1>2; exit 1)

	@! go run "$(shell scripts/pinned-tool.sh github.com/kisielk/godepgraph)" -nostdlib -s ./pkg/agent/clib/ | grep ' -> "github.com/sirupsen/logrus' \
	|| (echo "\n^ ERROR: make sure ./pkg/agent/clib/ does not depend on logrus. We don't want users' logs to be tainted. Talk to @petethepig if have questions\n" &1>2; exit 1)

.PHONY: clib-deps
clib-deps:
	go run "$(shell scripts/pinned-tool.sh github.com/kisielk/godepgraph)" -tags nogospy ./pkg/agent/clib/ | dot -Tsvg -o ./tmp/clib-deps.svg

.PHONY: unused
unused:
	staticcheck -f stylish -tags $(ALL_SPIES) -unused.whole-program ./...

.PHONY: install-dev-tools
install-dev-tools:
	cat tools/tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -tI {} go install {}

.PHONY: dev
dev:
	goreman -exit-on-error -f scripts/dev-procfile start

.PHONY: godoc
godoc:
	sleep 5 && open http://localhost:8090/pkg/github.com/pyroscope-io/pyroscope/ &
	godoc -http :8090

.PHONY: go-deps-graph
go-deps-graph:
	sh scripts/dependency-graph.sh
	open -a "/Applications/Google Chrome.app" tmp/go-deps-graph.svg

.PHONY: clean
clean:
	rm -rf tmp/pyroscope-storage

.PHONY: update-contributors
update-contributors:
	$(shell yarn bin contributor-faces) \
		-e pyroscopebot \
		.

.PHONY: update-changelog
update-changelog:
	$(shell yarn bin conventional-changelog) -i CHANGELOG.md -s
	sed -i '/Updates the list of contributors in README/d' CHANGELOG.md
	sed -i '/Update README.md/d' CHANGELOG.md

.PHONY: update-protobuf
update-protobuf:
	go install google.golang.org/protobuf/cmd/protoc-gen-go
	protoc --go_out=. pkg/convert/profile.proto

.PHONY: docker-dev
docker-dev:
	docker build . --tag pyroscope/pyroscope:dev

.PHONY: windows-dev
windows-dev:
	docker build --platform linux/amd64 -f Dockerfile.windows --output type=local,dest=out .
