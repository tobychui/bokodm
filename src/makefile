SHELL := /bin/sh

BINARY := bokodmd
DIST_DIR := dist
PLATFORMS := \
	linux_amd64 \
	linux_arm64 \
	darwin_amd64 \
	darwin_arm64 \
	windows_amd64 \
	windows_arm64

.DEFAULT_GOAL := all

.PHONY: all clean package $(PLATFORMS)

all: $(PLATFORMS)

clean:
	rm -rf $(DIST_DIR)

ifeq ($(GOOS),windows)
BIN_EXT := .exe
else
BIN_EXT :=
endif

PACKAGE_NAME := $(BINARY)_$(GOOS)_$(GOARCH)
PACKAGE_DIR := $(DIST_DIR)/$(PACKAGE_NAME)
ARCHIVE := $(DIST_DIR)/$(PACKAGE_NAME).tar.gz
OUTPUT_BIN := $(PACKAGE_DIR)/$(BINARY)$(BIN_EXT)

package:
	@test -n "$(GOOS)" || { echo "GOOS is required"; exit 1; }
	@test -n "$(GOARCH)" || { echo "GOARCH is required"; exit 1; }
	rm -rf $(PACKAGE_DIR)
	mkdir -p $(PACKAGE_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(OUTPUT_BIN) .
	cp -R web $(PACKAGE_DIR)/web
	tar -C $(DIST_DIR) -czf $(ARCHIVE) $(PACKAGE_NAME)
	@echo "Created $(ARCHIVE)"

define PLATFORM_template
$(1):
	$(MAKE) package GOOS=$(2) GOARCH=$(3)
endef

$(eval $(call PLATFORM_template,linux_amd64,linux,amd64))
$(eval $(call PLATFORM_template,linux_arm64,linux,arm64))
$(eval $(call PLATFORM_template,darwin_amd64,darwin,amd64))
$(eval $(call PLATFORM_template,darwin_arm64,darwin,arm64))
$(eval $(call PLATFORM_template,windows_amd64,windows,amd64))
$(eval $(call PLATFORM_template,windows_arm64,windows,arm64))
