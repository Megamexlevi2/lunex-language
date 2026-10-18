SHELL := /usr/bin/env bash

VERSION := $(shell grep -o '"version"[[:space:]]*:[[:space:]]*"[^"]*"' internal/app/version.json | sed 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')

GO_MIN := 1.25.5
GO_VER := 1.25.14

UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)

ifeq ($(UNAME_S),Linux)
	ifneq ($(wildcard /proc/version),)
		IS_ANDROID := $(shell if { [ -n "$$ANDROID_ROOT" ] && [ -n "$$ANDROID_DATA" ]; } || { [ -n "$$PREFIX" ] && printf "%s" "$$PREFIX" | grep -qE "^/data/(data/com\.termux|user/[^/]+/com\.termux|user_de/[^/]+/com\.termux)/"; } || [ -x /system/bin/linker64 ] || [ -x /system/bin/linker ] || [ -x /apex/com.android.runtime/bin/linker64 ] || [ -x /apex/com.android.runtime/bin/linker ] || { [ -x /system/bin/getprop ] && [ -n "$$(/system/bin/getprop ro.build.version.sdk 2>/dev/null)" ]; } || { command -v getprop >/dev/null 2>&1 && [ -n "$$(getprop ro.build.version.sdk 2>/dev/null)" ]; } || [ "$$(uname -o 2>/dev/null || true)" = Android ]; then echo yes; elif [ -f /proc/version ] && grep -qiE "android|bionic" /proc/version 2>/dev/null; then echo yes; else echo no; fi)
	else
		IS_ANDROID := no
	endif
	ifeq ($(IS_ANDROID),yes)
		HOST_OS := android
	else
		HOST_OS := linux
	endif
else ifeq ($(UNAME_S),Darwin)
	HOST_OS := macos
else ifneq (,$(findstring MINGW,$(UNAME_S))$(findstring MSYS,$(UNAME_S))$(findstring CYGWIN,$(UNAME_S)))
	HOST_OS := windows
else ifeq ($(UNAME_S),FreeBSD)
	HOST_OS := freebsd
else
	HOST_OS := unknown
endif

ifeq ($(UNAME_M),x86_64)
	HOST_ARCH := amd64
else ifeq ($(UNAME_M),amd64)
	HOST_ARCH := amd64
else ifeq ($(UNAME_M),aarch64)
	HOST_ARCH := arm64
else ifeq ($(UNAME_M),arm64)
	HOST_ARCH := arm64
else ifeq ($(UNAME_M),riscv64)
	HOST_ARCH := riscv64
else
	HOST_ARCH := unknown
endif

OS ?= $(HOST_OS)
ARCH ?= $(HOST_ARCH)

ifeq ($(OS),macos)
	GOOS := darwin
else ifeq ($(OS),android)
	GOOS := android
else
	GOOS := $(OS)
endif

ifeq ($(HOST_OS),android)
	TOOLCHAIN_OS := linux
else ifeq ($(HOST_OS),macos)
	TOOLCHAIN_OS := darwin
else
	TOOLCHAIN_OS := $(HOST_OS)
endif

ifeq ($(ARCH),x86_64)
	GOARCH := amd64
else ifeq ($(ARCH),aarch64)
	GOARCH := arm64
else
	GOARCH := $(ARCH)
endif

ifeq ($(OS),windows)
	EXT := .exe
else
	EXT :=
endif

BIN := lunex$(EXT)
RELEASE_DIR := release

GOBIN := $(shell command -v go 2>/dev/null)

RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[1;33m
BLUE := \033[0;34m
CYAN := \033[0;36m
NC := \033[0m
BOLD := \033[1m

.PHONY: all build release test clean install-go banner help

all: build

banner:
	@printf '\n  Lunex lang  v$(VERSION)\n  Created by David Dev\n  GitHub: https://github.com/Megamexlevi2\n\n'

install-go:
	@if [ -n "$(GOBIN)" ] && [ "$$(printf '%s\n%s\n' "$(GO_MIN)" "$$(go version | awk '{print $$3}' | sed 's/go//')" | sort -V | head -n1)" = "$(GO_MIN)" ]; then \
		printf '$(GREEN)[ok]$(NC) Go '"$$(go version | awk '{print $$3}')"' is already installed\n'; \
		exit 0; \
	fi; \
	printf '$(YELLOW)[!]$(NC) Go $(GO_MIN)+ was not found on this system\n'; \
	if [ "$(HOST_OS)" = "android" ]; then \
		printf '$(CYAN)[info]$(NC) Detected environment: Android/Termux\n'; \
	elif [ "$(HOST_OS)" = "macos" ]; then \
		printf '$(CYAN)[info]$(NC) Detected environment: macOS\n'; \
	else \
		distro=""; \
		if [ -f /etc/os-release ]; then distro=$$(awk -F= '/^ID=/{gsub(/"/, "", $$2); print $$2; exit}' /etc/os-release); fi; \
		if [ -n "$$distro" ]; then \
			printf '$(CYAN)[info]$(NC) Detected environment: Linux ('"$$distro"')\n'; \
		else \
			printf '$(CYAN)[info]$(NC) Detected environment: Linux\n'; \
		fi; \
	fi; \
	pm=""; \
	if command -v pkg >/dev/null 2>&1 && [ -n "$$PREFIX" ]; then pm="pkg"; \
	elif command -v apt >/dev/null 2>&1; then pm="apt"; \
	elif command -v apt-get >/dev/null 2>&1; then pm="apt-get"; \
	elif command -v dnf >/dev/null 2>&1; then pm="dnf"; \
	elif command -v yum >/dev/null 2>&1; then pm="yum"; \
	elif command -v pacman >/dev/null 2>&1; then pm="pacman"; \
	elif command -v apk >/dev/null 2>&1; then pm="apk"; \
	elif command -v zypper >/dev/null 2>&1; then pm="zypper"; \
	elif command -v brew >/dev/null 2>&1; then pm="brew"; \
	fi; \
	use_sudo="0"; \
	if [ "$$(id -u 2>/dev/null || echo 1)" != "0" ] && command -v sudo >/dev/null 2>&1; then \
		if sudo -n true 2>/dev/null; then use_sudo="1"; \
		elif [ -t 0 ] && [ -t 1 ]; then use_sudo="1"; \
		fi; \
	fi; \
	installed="0"; \
	if [ -n "$$pm" ]; then \
		case "$$pm" in \
			pkg|apt|apt-get|dnf|yum) pkg_name="golang" ;; \
			*) pkg_name="go" ;; \
		esac; \
		prefix=""; \
		[ "$$use_sudo" = "1" ] && prefix="sudo "; \
		case "$$pm" in \
			pkg)     install_cmd="$${prefix}pkg install -y $$pkg_name" ;; \
			apt)     install_cmd="$${prefix}apt update && $${prefix}apt install -y $$pkg_name" ;; \
			apt-get) install_cmd="$${prefix}apt-get update && $${prefix}apt-get install -y $$pkg_name" ;; \
			dnf)     install_cmd="$${prefix}dnf install -y $$pkg_name" ;; \
			yum)     install_cmd="$${prefix}yum install -y $$pkg_name" ;; \
			pacman)  install_cmd="$${prefix}pacman -Sy --noconfirm $$pkg_name" ;; \
			apk)     install_cmd="$${prefix}apk add $$pkg_name" ;; \
			zypper)  install_cmd="$${prefix}zypper install -y $$pkg_name" ;; \
			brew)    install_cmd="brew install $$pkg_name" ;; \
		esac; \
		printf '$(CYAN)[info]$(NC) Found package manager: '"$$pm"'\n'; \
		if [ -t 0 ]; then \
			read -r -p "Install Go via '$$pm' now? ($$install_cmd) [y/N] " answer; \
			case "$$answer" in \
				y|Y|yes|Yes|YES) \
					printf '$(BLUE)==>$(NC) $(BOLD)Installing Go via '"$$pm"'$(NC)\n'; \
					if eval "$$install_cmd"; then \
						if command -v go >/dev/null 2>&1 && [ "$$(printf '%s\n%s\n' "$(GO_MIN)" "$$(go version | awk '{print $$3}' | sed 's/go//')" | sort -V | head -n1)" = "$(GO_MIN)" ]; then \
							printf '$(GREEN)[ok]$(NC) Go '"$$(go version | awk '{print $$3}')"' installed\n'; \
							installed="1"; \
						else \
							printf '$(YELLOW)[!]$(NC) Package manager installed a Go version older than $(GO_MIN) or go is not on PATH\n'; \
						fi; \
					else \
						printf '$(RED)[error]$(NC) Go installation via '"$$pm"' failed\n' >&2; \
						exit 1; \
					fi \
					;; \
			esac; \
		fi; \
	fi; \
	if [ "$$installed" != "1" ] && command -v curl >/dev/null 2>&1 && [ -t 0 ]; then \
		read -r -p "Download and install Go $(GO_MIN)+ directly from go.dev via curl? [y/N] " answer; \
		case "$$answer" in \
			y|Y|yes|Yes|YES) \
				printf '$(BLUE)==>$(NC) $(BOLD)Installing Go via curl$(NC)\n'; \
				tmp=$$(mktemp -d); \
				root="$(HOME)/.local/go"; \
				curl -fsSL "https://go.dev/dl/go$(GO_VER).$(TOOLCHAIN_OS)-$(GOARCH).tar.gz" -o "$$tmp/go.tar.gz"; \
				tar -C "$$tmp" -xzf "$$tmp/go.tar.gz"; \
				rm -rf "$$root"; mkdir -p "$(HOME)/.local" "$(HOME)/.local/bin"; \
				mv "$$tmp/go" "$$root"; \
				ln -sf "$$root/bin/go" "$(HOME)/.local/bin/go"; \
				rm -rf "$$tmp"; \
				printf '$(GREEN)[ok]$(NC) Go $(GO_VER) installed\n'; \
				installed="1"; \
				;; \
		esac; \
	fi; \
	if [ "$$installed" != "1" ]; then \
		printf '$(RED)[error]$(NC) Go $(GO_MIN)+ is required to build Lunex. Installation was declined or no installer is available.\n' >&2; \
		exit 1; \
	fi

build: banner install-go
	@printf '$(BLUE)==>$(NC) $(BOLD)Building Lunex for $(GOOS)/$(GOARCH)$(NC)\n'
	@GONOSUMDB='*' GOFLAGS='-mod=mod' GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 \
		go build $(if $(filter android,$(OS)),-buildmode=pie,) $(if $(filter freebsd,$(OS)),-gcflags=github.com/go-webgpu/goffi/internal/fakecgo=-std,) -trimpath -tags netgo,osusergo -ldflags="-s -w" -o "$(BIN)" ./cmd/lunex
	@test -f "$(BIN)" || (printf '$(RED)[error]$(NC) Build completed without producing $(BIN)\n' >&2; exit 1)
	@if [ "$(OS)" = "android" ] && command -v readelf >/dev/null 2>&1; then \
		interp="$$(readelf -lW "$(BIN)" 2>/dev/null | sed -n 's/.*Requesting program interpreter: \([^]]*\).*/\1/p' | head -n1 | tr -d '\r' | sed 's/[[:space:]]*$$//')"; \
		case "$(GOARCH):$$interp" in arm64:/system/bin/linker64|arm64:/apex/com.android.runtime/bin/linker64|amd64:/system/bin/linker|amd64:/apex/com.android.runtime/bin/linker) ;; *) printf '$(RED)[error]$(NC) Android binary has an invalid ELF interpreter: %s\n' "$${interp:-missing}" >&2; exit 1 ;; esac; \
	fi
	@printf '$(GREEN)[ok]$(NC) $(BIN) built: '"$$(du -sh "$(BIN)" | cut -f1)"'\n'
	@printf '\n$(GREEN)[ok]$(NC) Lunex v$(VERSION) built successfully\n'
	@printf '$(CYAN)[info]$(NC) Run:  ./$(BIN) run <file.lx>\n'

release: banner install-go
	@printf '$(BLUE)==>$(NC) $(BOLD)Starting multi-platform release build$(NC)\n'
	@mkdir -p $(RELEASE_DIR)
	@for target in linux:amd64 linux:arm64 windows:amd64 windows:arm64 darwin:amd64 darwin:arm64 android:arm64 freebsd:amd64; do \
		t_os=$${target%%:*}; t_arch=$${target##*:}; \
		ext=""; \
		[ "$$t_os" = "windows" ] && ext=".exe"; \
		name="lunex-$$t_os-$$t_arch$$ext"; \
		printf '$(CYAN)[info]$(NC) Building '"$$t_os/$$t_arch"'\n'; \
		GONOSUMDB='*' GOFLAGS='-mod=mod' GOOS=$$t_os GOARCH=$$t_arch CGO_ENABLED=0 \
			go build $(if $(filter android,$$t_os),-buildmode=pie,) $(if $(filter freebsd,$$t_os),-gcflags=github.com/go-webgpu/goffi/internal/fakecgo=-std,) -trimpath -tags netgo,osusergo -ldflags="-s -w" -o "$(RELEASE_DIR)/$$name" ./cmd/lunex || exit 1; \
		printf '$(GREEN)[ok]$(NC) Done: $(RELEASE_DIR)/'"$$name ($$(du -sh "$(RELEASE_DIR)/$$name" | cut -f1))"'\n'; \
	done
	@printf '$(GREEN)[ok]$(NC) All targets built successfully in ./$(RELEASE_DIR)/\n'

test: build
	@printf '$(BLUE)==>$(NC) $(BOLD)Running Lunex integration tests$(NC)\n'
	@bash tests/run_all.sh ./$(BIN)
	@printf '$(GREEN)[ok]$(NC) All tests passed\n'

clean:
	@printf '$(BLUE)==>$(NC) $(BOLD)Cleaning build artifacts$(NC)\n'
	@rm -rf lunex lunex.exe lunex.target lunex.exe.target $(RELEASE_DIR)
	@printf '$(GREEN)[ok]$(NC) Cleaned\n'

help:
	@echo "Targets:"
	@echo "  build    - build for current platform (default)"
	@echo "  release  - build for all platforms"
	@echo "  test     - build + run tests"
	@echo "  clean    - remove build artifacts"
	@echo ""
	@echo "Override OS/ARCH: make build OS=linux ARCH=arm64"