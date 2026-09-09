ROOT := $(abspath $(CURDIR)/..)
HUGO ?= hugo
DOC_SOURCE := $(ROOT)/docs
CONTENT_DOCS := $(CURDIR)/content/docs
DOC_FILES := compatibility.md keybindings.md parity.md sparse-checkout.md worktrees.md

.PHONY: sync build serve clean

sync:
	cd "$(ROOT)" && go run ./internal/keymap/cmd/keymapdoc -check
	mkdir -p "$(CONTENT_DOCS)" "$(CURDIR)/static/images"
	for file in $(DOC_FILES); do \
		if [ "$$file" = "keybindings.md" ]; then \
			sed 's#](../internal/#](https://github.com/richardrh/lazymagit/blob/master/internal/#g' "$(DOC_SOURCE)/$$file" > "$(CONTENT_DOCS)/$$file"; \
		else \
			cp "$(DOC_SOURCE)/$$file" "$(CONTENT_DOCS)/$$file"; \
		fi; \
	done
	cp "$(DOC_SOURCE)"/images/* "$(CURDIR)/static/images/"

build: sync
	$(HUGO) --source "$(CURDIR)" --gc --minify --panicOnWarning

serve: sync
	$(HUGO) server --source "$(CURDIR)" --buildDrafts --disableFastRender

clean:
	rm -rf "$(CURDIR)/public" "$(CURDIR)/resources" "$(CURDIR)/static/images"
	rm -f $(addprefix $(CONTENT_DOCS)/,$(DOC_FILES))
