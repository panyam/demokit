# Top-level Makefile for the demokit repo.
#
# The repo is a multi-module workspace (see go.work): the demokit
# module at the root and the standalone notebook module under
# ./notebook. `go test ./...` is module-scoped, so testall loops
# every module explicitly.

# Every go.mod in the repo, one module per line.
MODULES := . notebook notebook/examples/mathrepl

.PHONY: test testall build buildall race tidy fmt vet

# Test the root (demokit) module only.
test:
	go test ./...

# Test every module in the workspace.
testall:
	@set -e; for m in $(MODULES); do \
		echo "==> test $$m"; \
		(cd $$m && go test ./...); \
	done

# Test every module with the race detector.
race:
	@set -e; for m in $(MODULES); do \
		echo "==> test -race $$m"; \
		(cd $$m && go test -race ./...); \
	done

# Build the root (demokit) module only.
build:
	go build ./...

# Build every module in the workspace.
buildall:
	@set -e; for m in $(MODULES); do \
		echo "==> build $$m"; \
		(cd $$m && go build ./...); \
	done

# Tidy every module's go.mod.
tidy:
	@set -e; for m in $(MODULES); do \
		echo "==> tidy $$m"; \
		(cd $$m && go mod tidy); \
	done

# gofmt every module.
fmt:
	@set -e; for m in $(MODULES); do \
		echo "==> fmt $$m"; \
		gofmt -w $$m; \
	done

# go vet every module.
vet:
	@set -e; for m in $(MODULES); do \
		echo "==> vet $$m"; \
		(cd $$m && go vet ./...); \
	done
