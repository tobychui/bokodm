# bokodm Makefile
# Builds for Linux across most Go-supported CPU architectures.

BINARY  := bokodm
SRC_DIR := ./
OUT_DIR := dist

.PHONY: all clean help \
	amd64 386 \
	arm-6 arm-7 \
	arm64 \
	riscv64

all: amd64 386 arm-6 arm-7 arm64 riscv64

amd64:
	@mkdir -p $(OUT_DIR)
	GOOS=linux GOARCH=amd64 go build -o $(OUT_DIR)/$(BINARY)-linux-amd64 $(SRC_DIR)
	@echo "Built: $(OUT_DIR)/$(BINARY)-linux-amd64"

386:
	@mkdir -p $(OUT_DIR)
	GOOS=linux GOARCH=386 go build -o $(OUT_DIR)/$(BINARY)-linux-386 $(SRC_DIR)
	@echo "Built: $(OUT_DIR)/$(BINARY)-linux-386"

arm-6:
	@mkdir -p $(OUT_DIR)
	GOOS=linux GOARCH=arm GOARM=6 go build -o $(OUT_DIR)/$(BINARY)-linux-armv6 $(SRC_DIR)
	@echo "Built: $(OUT_DIR)/$(BINARY)-linux-armv6"

arm-7:
	@mkdir -p $(OUT_DIR)
	GOOS=linux GOARCH=arm GOARM=7 go build -o $(OUT_DIR)/$(BINARY)-linux-armv7 $(SRC_DIR)
	@echo "Built: $(OUT_DIR)/$(BINARY)-linux-armv7"

arm64:
	@mkdir -p $(OUT_DIR)
	GOOS=linux GOARCH=arm64 go build -o $(OUT_DIR)/$(BINARY)-linux-arm64 $(SRC_DIR)
	@echo "Built: $(OUT_DIR)/$(BINARY)-linux-arm64"

riscv64:
	@mkdir -p $(OUT_DIR)
	GOOS=linux GOARCH=riscv64 go build -o $(OUT_DIR)/$(BINARY)-linux-riscv64 $(SRC_DIR)
	@echo "Built: $(OUT_DIR)/$(BINARY)-linux-riscv64"

clean:
	rm -rf $(OUT_DIR)
	@echo "Cleaned $(OUT_DIR)/"

help:
	@echo "Usage (run from project root):"
	@echo "  make all      Build for all architectures"
	@echo "  make clean    Remove dist/"
	@echo "  make <arch>   Build for one target"
	@echo ""
	@echo "Targets: amd64  386  arm-6  arm-7  arm64  riscv64"
