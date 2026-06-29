# hrm — Live Whoop heart-rate & stress monitor
#
# buildvcs=false is set because the repo is not yet a git checkout; it is
# harmless once `git init` + first commit exist.

BIN        := hrm
PKG        := ./cmd/hrm
BUILD_DIR  := dist
GOFLAGS    := -buildvcs=false
PLIST      := Info.plist
# Pure-logic packages are safe to run under the race detector; the ble package
# links macOS CoreBluetooth (cgo) which aborts under -race, so it is excluded.
RACE_PKGS  := ./internal/stress/ ./internal/store/ ./internal/report/ ./internal/heartrate/

.DEFAULT_GOAL := help

.PHONY: build
build: ## Build the hrm binary into dist/
	@mkdir -p $(BUILD_DIR)
	go build $(GOFLAGS) -o $(BUILD_DIR)/$(BIN) $(PKG)
	@echo "built $(BUILD_DIR)/$(BIN)"
	@echo "macOS: your terminal must be granted Bluetooth permission — see 'make bluetooth-help'"

APP        := $(BUILD_DIR)/$(BIN).app

.PHONY: bundle
bundle: ## macOS: build a signed .app bundle (only useful if launched via `open`, for GUI distribution)
	@mkdir -p $(APP)/Contents/MacOS
	go build $(GOFLAGS) -o $(APP)/Contents/MacOS/$(BIN) $(PKG)
	@cp $(PLIST) $(APP)/Contents/Info.plist
	@codesign --force --sign - $(APP)
	@echo "built $(APP)"

.PHONY: bluetooth-help
bluetooth-help: ## Print how to fix the macOS 'abort trap: 6' Bluetooth-permission error
	@echo "macOS aborts a CLI's Bluetooth access unless the TERMINAL app it runs in"
	@echo "has been granted Bluetooth permission (the CLI inherits the terminal's grant)."
	@echo
	@echo "Fix:"
	@echo "  1. System Settings -> Privacy & Security -> Bluetooth -> enable your terminal"
	@echo "     (Warp / iTerm / Terminal). Then re-run 'hrm devices'."
	@echo "  2. If your terminal is NOT in that list (so you can't enable it), it was never"
	@echo "     able to register. Reset the Bluetooth privacy list to force a fresh prompt:"
	@echo "        tccutil reset Bluetooth"
	@echo "     then run 'hrm devices' again and click Allow on the prompt."
	@echo "  3. If your terminal still never prompts, run hrm from Apple's Terminal.app or"
	@echo "     iTerm2 once to grant + verify, then use any granted terminal."

.PHONY: test
test: ## Run the full test suite
	go test $(GOFLAGS) ./...

.PHONY: test-race
test-race: ## Run race-safe (non-cgo) packages under the race detector
	go test $(GOFLAGS) -race $(RACE_PKGS)

COVER_OUT := $(BUILD_DIR)/coverage.out

.PHONY: cover
cover: ## Run tests, writing a coverage profile and printing the total
	@mkdir -p $(BUILD_DIR)
	go test $(GOFLAGS) -coverprofile=$(COVER_OUT) ./...
	@echo
	@go tool cover -func=$(COVER_OUT) | tail -1

.PHONY: cover-html
cover-html: cover ## Open the coverage profile as an HTML report in the browser
	go tool cover -html=$(COVER_OUT)

.PHONY: vet
vet: ## Run go vet
	go vet $(GOFLAGS) ./...

.PHONY: fmt
fmt: ## Format all Go sources
	gofmt -w .

.PHONY: tidy
tidy: ## Tidy module dependencies
	go mod tidy

.PHONY: check
check: fmt vet test-race ## Format, vet, and run race tests

.PHONY: run
run: ## Run the live monitor dashboard (go run)
	go run $(GOFLAGS) $(PKG) monitor

.PHONY: cli-help
cli-help: build ## Build then print the CLI help menu
	@echo
	@$(BUILD_DIR)/$(BIN) --help

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR)

.PHONY: help
help: ## Show this help menu
	@echo "hrm — make targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "} {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'
