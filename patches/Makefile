# Spectro Cloud patches for upstream kubev2v/forklift.
#
# Modelled on thirdparty-builder/images/mongodb-enterprise: the patches are
# applied to a pristine upstream checkout just before building, then reverted,
# so the source tree stays upstream and the fork delta lives in review-sized
# files rather than a permanently diverged branch.

UPSTREAM_TAG ?= v2.12.1
PATCH_DIR    ?= $(CURDIR)
REPO_ROOT    ?= $(CURDIR)/..
PATCHES      := $(shell grep -v '^\#' $(PATCH_DIR)/series | grep -v '^$$')

.PHONY: apply
apply: ## Apply every patch in series order
	@cd $(REPO_ROOT) && for p in $(PATCHES); do \
		echo "applying $$p"; \
		git apply --whitespace=nowarn $(PATCH_DIR)/$$p || { echo "FAILED: $$p"; exit 1; }; \
	done
	@echo "all patches applied on top of $(UPSTREAM_TAG)"

.PHONY: check
check: ## Dry-run: verify every patch still applies, change nothing
	@cd $(REPO_ROOT) && ok=0; done_=0; bad=0; \
	for p in $(PATCHES); do \
		if git apply --check $(PATCH_DIR)/$$p 2>/dev/null; then \
			echo "  applies    $$p"; ok=$$((ok+1)); \
		elif git apply --check -R $(PATCH_DIR)/$$p 2>/dev/null; then \
			echo "  already-in $$p"; done_=$$((done_+1)); \
		else \
			echo "  CONFLICT   $$p"; bad=$$((bad+1)); \
		fi; \
	done; \
	echo "applies: $$ok  already applied: $$done_  conflicting: $$bad"; \
	[ $$bad -eq 0 ]

.PHONY: reverse
reverse: ## Revert every patch (reverse series order)
	@cd $(REPO_ROOT) && for p in $$(echo "$(PATCHES)" | tr ' ' '\n' | tail -r); do \
		echo "reverting $$p"; \
		git apply -R --whitespace=nowarn $(PATCH_DIR)/$$p || true; \
	done

.PHONY: restore
restore: ## Discard all patch edits and return to a pristine tree
	@cd $(REPO_ROOT) && git checkout -- . && git clean -fd \
		pkg/apis/forklift/v1beta1/spectro_fork_test.go \
		pkg/virt-v2v/conversion/conversion_spectro.go 2>/dev/null || true
	@echo "tree restored to $(UPSTREAM_TAG)"

.PHONY: regenerate
regenerate: ## Rebuild patches from a branch: make regenerate BRANCH=<ref>
	@test -n "$(BRANCH)" || { echo "usage: make regenerate BRANCH=<ref>"; exit 1; }
	@cd $(REPO_ROOT) && rm -f $(PATCH_DIR)/*.patch && \
		git format-patch -q --no-signature --zero-commit \
			-o $(PATCH_DIR) $(UPSTREAM_TAG)..$(BRANCH)
	@echo "regenerated; update series to match"

.PHONY: verify
verify: ## Prove the patches reproduce a branch exactly: make verify BRANCH=<ref>
	@test -n "$(BRANCH)" || { echo "usage: make verify BRANCH=<ref>"; exit 1; }
	@tmp=$$(mktemp -d); cd $(REPO_ROOT) && \
		git worktree add -q --detach $$tmp $(UPSTREAM_TAG) && \
		for p in $(PATCHES); do git -C $$tmp apply $(PATCH_DIR)/$$p || exit 1; done && \
		git -C $$tmp add -A && \
		if [ -z "$$(git -C $$tmp diff --stat $(BRANCH) -- . ':(exclude)vendor')" ]; then \
			echo "IDENTICAL to $(BRANCH)"; else \
			echo "DIFFERS from $(BRANCH)"; git -C $$tmp diff --stat $(BRANCH) -- . ':(exclude)vendor'; fi; \
		git worktree remove --force $$tmp

.PHONY: help
help:
	@grep -E '^[a-z-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "};{printf "  %-12s %s\n",$$1,$$2}'
