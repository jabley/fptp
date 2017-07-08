# This repo's root import path (under GOPATH).
PKG := github.com/jabley/fptp

# Which architecture to build - see $(ALL_ARCH) for options.
ARCH ?= amd64

###
### These variables should not need tweaking.
###

SRC_DIRS := . # directories which hold app source (not vendored)

ALL_ARCH := amd64 arm arm64 ppc64le

BUILD_IMAGE ?= golang:1.8-alpine

all: test

test: build-dirs
	@docker run                                                            \
	    -ti                                                                \
	    -u $$(id -u):$$(id -g)                                             \
	    -v $$(pwd)/.go:/go                                                 \
	    -v $$(pwd):/go/src/$(PKG)                                          \
	    -v $$(pwd)/bin/$(ARCH):/go/bin                                     \
	    -v $$(pwd)/.go/std/$(ARCH):/usr/local/go/pkg/linux_$(ARCH)_static  \
	    -w /go/src/$(PKG)                                                  \
	    $(BUILD_IMAGE)                                                     \
	    /bin/sh -c "                                                       \
	        ./build/test.sh $(SRC_DIRS)                                    \
	    "

build-dirs:
	@mkdir -p bin/$(ARCH)
	@mkdir -p .go/src/$(PKG) .go/pkg .go/bin .go/std/$(ARCH)

clean: bin-clean

bin-clean:
	rm -rf .go bin

lint:
	golint ./...
	go vet ./...
