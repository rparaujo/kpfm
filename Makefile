CONFIG_DIR=$(HOME)/.config
FILE_PATH=$(CONFIG_DIR)/.kpfm

go_mod:
	@go mod tidy
	@go mod vendor
	@echo "Updated go.mod"

build: go_mod
	@go build -o kpfm main.go
	@echo "Built kpfm"

run: build
	@APP_MODE=release ./kpfm
	@echo "Running kpfm in release mode"

debug: go_mod
	@go build -o kpfm -gcflags="all=-N -l" main.go
	@APP_MODE=debug ./kpfm
	@echo "Running kpfm in debug mode"

clean:
	@rm -rf kpfm
	@echo "Cleaned up the build files"

install: build
	@cp kpfm /usr/local/bin/kpfm
	@mkdir -p $(CONFIG_DIR)
	@touch $(FILE_PATH)
	@echo "\nDon't forget to add /usr/local/bin to your PATH"