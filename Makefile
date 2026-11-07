# Builds ScottiCAD on Linux (GNU make). Needs Go 1.25 or newer, or any Go
# from 1.21 on with internet access (it fetches 1.25 itself); no C
# compiler. On Windows, build.ps1 and release.ps1 do the same.
#
#   make                 the program for this machine: ./scotticad
#   make test            build, then run the Go and Python tests
#   make linux-amd64     build/linux-amd64/scotticad
#   make linux-arm64     build/linux-arm64/scotticad
#   make windows         build/windows-amd64/scotticad.exe
#   make cross           all three, for release (Go cross-compiles the ones
#                        that aren't for this machine)
#   make release         the release packages for the version in VERSION, in dist/:
#                          scotticad-v<version>-windows-amd64.zip
#                          scotticad-v<version>-linux-amd64.tar.gz
#                          scotticad-v<version>-linux-arm64.tar.gz
#                        and their SHA-256s for the release notes
#   make release-test    the same, after running the tests
#   make examples        run every examples/star script and save each design
#                        in examples/scotticad (run by hand; needs an X
#                        display, as the Python tests do)
#   make install         install ./scotticad for this user, as the tarball's install.sh does
#   make uninstall       remove it again
#   make clean           remove everything the targets above built
#
# The Python tests drive the real program, which needs an X display: they run
# under xvfb-run when it is installed (package xvfb), else on $DISPLAY (their
# windows are hidden either way). They use the virtual environment in VENV,
# made with pytest on first use.

GO ?= go
VENV ?= .venv
PYTHON ?= python3

# one module: a go.work in a folder above mustn't take over the build
export GOWORK := off
# pure Go: the X11 and OpenGL libraries are opened at run time
export CGO_ENABLED := 0

# -trimpath keeps the build machine's folders out of the executable
BUILD := $(GO) build -trimpath
LDFLAGS := -s -w
PACK := $(GO) run ./tools/tarball

VERSION := $(strip $(shell cat VERSION))
# file times inside the packages are the last commit's (or a fixed date
# outside a checkout), so the same commit always gives the same packages
EPOCH := $(shell git log -1 --format=%ct 2>/dev/null || echo 1767225600)
PKG := scotticad-v$(VERSION)

DOCS := README.md RELEASE-NOTES.md LICENSE NOTICE THIRD_PARTY_NOTICES.md
LINUX_FILES := packaging/linux/install.sh packaging/linux/scotticad.desktop \
	packaging/linux/scotticad.svg packaging/linux/scotticad-mime.xml

# Go tracks what needs rebuilding, so every build target always runs it.
.PHONY: all scotticad test linux-amd64 linux-arm64 windows cross release release-test examples install uninstall clean

all: scotticad

scotticad:
	$(BUILD) -ldflags '$(LDFLAGS)' -o scotticad ./cmd/scotticad

test: scotticad
	$(GO) test ./pkg/...
	test -x $(VENV)/bin/python || { $(PYTHON) -m venv $(VENV) && $(VENV)/bin/python -m pip install --quiet --disable-pip-version-check pytest; }
	@if command -v xvfb-run >/dev/null; then \
		xvfb-run -a -s "-screen 0 1920x1080x24" $(VENV)/bin/python -m pytest python/tests -q; \
	elif [ -n "$$DISPLAY" ]; then \
		$(VENV)/bin/python -m pytest python/tests -q; \
	else \
		echo "the Python tests need an X display: install xvfb (sudo apt install xvfb) or run them in a desktop session" >&2; \
		exit 1; \
	fi

linux-amd64 linux-arm64:
	GOOS=linux GOARCH=$(@:linux-%=%) $(BUILD) -ldflags '$(LDFLAGS)' -o build/$@/scotticad ./cmd/scotticad

windows:
	GOOS=windows GOARCH=amd64 $(BUILD) -ldflags '-H windowsgui $(LDFLAGS)' -o build/windows-amd64/scotticad.exe ./cmd/scotticad

cross: linux-amd64 linux-arm64 windows

release: cross
	@echo "$(VERSION)" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$$' || \
		{ echo "VERSION does not hold a version number: '$(VERSION)'" >&2; exit 1; }
	mkdir -p dist
	$(PACK) -o dist/$(PKG)-windows-amd64.zip -mtime $(EPOCH) build/windows-amd64/scotticad.exe $(DOCS)
	$(PACK) -o dist/$(PKG)-linux-amd64.tar.gz -folder $(PKG)-linux-amd64 -mtime $(EPOCH) \
		-exec scotticad,install.sh build/linux-amd64/scotticad $(LINUX_FILES) $(DOCS)
	$(PACK) -o dist/$(PKG)-linux-arm64.tar.gz -folder $(PKG)-linux-arm64 -mtime $(EPOCH) \
		-exec scotticad,install.sh build/linux-arm64/scotticad $(LINUX_FILES) $(DOCS)
	@echo; echo "Tag: v$(VERSION)"
	@cd dist && sha256sum $(PKG)-windows-amd64.zip $(PKG)-linux-amd64.tar.gz $(PKG)-linux-arm64.tar.gz

release-test:
	$(MAKE) test
	$(MAKE) release

examples: scotticad
	@if command -v xvfb-run >/dev/null; then \
		xvfb-run -a -s "-screen 0 1920x1080x24" $(PYTHON) examples/build.py; \
	elif [ -n "$$DISPLAY" ]; then \
		$(PYTHON) examples/build.py; \
	else \
		echo "make examples needs an X display: install xvfb (sudo apt install xvfb) or run it in a desktop session" >&2; \
		exit 1; \
	fi

# install.sh installs from its own folder, as laid out in the tarball
install: scotticad
	rm -rf build/install
	mkdir -p build/install
	cp scotticad $(LINUX_FILES) build/install/
	bash build/install/install.sh
	rm -rf build/install

uninstall:
	bash packaging/linux/install.sh --uninstall

clean:
	rm -rf scotticad build dist
