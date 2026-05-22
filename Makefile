# Top-level Makefile for the demokit repo.
#
# The repo is a multi-module workspace (see go.work): the demokit
# module at the root and the standalone notebook module under
# ./notebook. `go test ./...` is module-scoped, so testall loops
# every module explicitly.

# Every go.mod in the repo, one module per line.
MODULES := . notebook notebook/examples/mathrepl notebook/examples/cmdshell

# Modules that get their own Go tags. Examples are intentionally
# excluded — they're consumer apps, not imported by anyone.
# Each entry is "<go.mod dir>:<tag prefix>"; "." → root tag with
# no prefix, "notebook" → notebook/vX.Y.Z, etc. Extend as new
# importable submodules ship.
TAG_MODULES := .:'' notebook:notebook

.PHONY: test testall build buildall race tidy fmt vet tag push-tags show-tags

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

# ----------------------------------------------------------------
# Release tagging
#
# Single VERSION variable creates one git tag per importable
# module at HEAD, lockstep. Submodule tags carry the module path
# as prefix (Go module-versioning convention) — notebook/vX.Y.Z
# is what consumers run `go get` against.
#
# Usage:
#   make tag VERSION=v0.0.23           # creates v0.0.23 + notebook/v0.0.23
#   make push-tags                     # pushes every local tag to origin
#   make show-tags                     # lists tags at HEAD + latest per module
#
# Examples (mathrepl, cmdshell) are deliberately untagged — they
# are consumer apps, not libraries. Add new importable modules to
# TAG_MODULES above to bring them into the cycle.

tag:
	@test -n "$(VERSION)" || (echo "VERSION=vX.Y.Z required" && exit 1)
	@set -e; for entry in $(TAG_MODULES); do \
		prefix=$${entry##*:}; \
		dir=$${entry%:*}; \
		tagname=$$([ -z "$$prefix" ] && echo $(VERSION) || echo "$$prefix/$(VERSION)"); \
		echo "==> tag $$tagname"; \
		git tag -a $$tagname -m "$$tagname"; \
	done
	@echo ""
	@echo "Local tags at HEAD:"
	@git tag --points-at HEAD
	@echo ""
	@echo "Push with: make push-tags"

# Push every local tag to origin. Safe to rerun (existing remote
# tags are skipped). Doesn't push branches — that's separate.
push-tags:
	git push origin --tags

# Diagnostic: latest tag per module + tags at HEAD. Useful before
# tagging a new release to confirm the version you're about to
# pick is actually the next one.
show-tags:
	@for entry in $(TAG_MODULES); do \
		prefix=$${entry##*:}; \
		dir=$${entry%:*}; \
		pattern=$$([ -z "$$prefix" ] && echo "v[0-9]*" || echo "$$prefix/v[0-9]*"); \
		latest=$$(git tag --list "$$pattern" --sort=-v:refname | head -1); \
		echo "$$dir: $$latest"; \
	done
	@echo ""
	@echo "Tags at HEAD:"
	@git tag --points-at HEAD | sed 's/^/  /' || echo "  (none)"
