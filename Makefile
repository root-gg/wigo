SHELL = /bin/bash

BASE_DIR:=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
RELEASE_VERSION:=$(shell cat $(BASE_DIR)/VERSION)
RELEASE_DIR="release/plik-$(RELEASE_VERSION)"
RELEASE_TARGETS=linux-amd64 linux-arm
GOHOSTOS=$(shell go env GOHOSTOS)
GOHOSTARCH=$(shell go env GOHOSTARCH)

DEBROOT=debs
DEBSRC=$(DEBROOT)/src

ifdef REPOROOT
else
    REPOROOT="/repo-root"
endif

build = go build

.PHONY: all clean deps lint race release releases build-dev run-dev

all: clean deps lint race release releases

race:
	@echo "Building with race detector (current OS)"
	@mkdir -p release
	@cd release; \
	export CGO_ENABLED=1; \
	GORACE="halt_on_error=1" go build -race -o current/wigo $(BASE_DIR)/src/wigo.go; \
	GORACE="halt_on_error=1" go build -race -o current/wigocli $(BASE_DIR)/src/wigocli.go; \
	go build -o current/generate_cert $(BASE_DIR)/src/generate_cert.go

deps:
	@echo "Installing dependencies"
	go mod download -x
	@echo "Installing frontend dependencies"
	cd src/public && npm install

build-frontend: deps
	@echo "Building frontend"
	cd src/public && npm run pretty && npm run build

releases: build-frontend
	@echo "Building Wigo releases"
	@mkdir -p release
	@cd release && for target in $(RELEASE_TARGETS) ; do \
		RELEASE_DIR=$(BASE_DIR)/release/$$target; \
		export CGO_ENABLED=0; \
		export GOOS=`echo $$target | cut -d "-" -f 1`; 	\
		export GOARCH=`echo $$target | cut -d "-" -f 2`; \
		if [ $$target = 'linux-arm' ]; then  \
			export GOARM='7'; \
			export CC=; \
		else \
			export GOARM=; \
			export CC=; \
	    fi ; \
		mkdir $$RELEASE_DIR; \
		echo "Building Wigo release for $$target to $$RELEASE_DIR"; \
		$(build) -tags "netgo osusergo" -ldflags "-s -w -X github.com/root-gg/wigo/src/wigo.Version=$(RELEASE_VERSION)" -o $$RELEASE_DIR/wigo $(BASE_DIR)/src/cmd/wigo/main.go; \
		$(build) -tags "netgo osusergo" -ldflags "-s -w -X github.com/root-gg/wigo/src/wigo.Version=$(RELEASE_VERSION)" -o $$RELEASE_DIR/wigocli $(BASE_DIR)/src/cmd/wigocli/main.go; \
		$(build) -o $$RELEASE_DIR/generate_cert $(BASE_DIR)/src/cmd/generate_cert/main.go; \
	done

release: build-frontend
	@echo "Building Wigo release for current OS"
	@mkdir -p release
	@cd release; \
	export CGO_ENABLED=0; \
	$(build) -tags "netgo osusergo" -ldflags "-s -w -X github.com/root-gg/wigo/src/wigo.Version=$(RELEASE_VERSION)" -o current/wigo $(BASE_DIR)/src/cmd/wigo/main.go;	\
	$(build) -tags "netgo osusergo" -ldflags "-s -w -X github.com/root-gg/wigo/src/wigo.Version=$(RELEASE_VERSION)" -o current/wigocli $(BASE_DIR)/src/cmd/wigocli/main.go; \
	$(build) -o current/generate_cert $(BASE_DIR)/src/cmd/generate_cert/main.go

debs: releases
	@echo "Building Wigo Debian packages"
	@mkdir -p $(DEBSRC)
	@mkdir -p $(DEBSRC)/etc/wigo/conf.d
	@mkdir -p $(DEBSRC)/etc/logrotate.d
	@mkdir -p $(DEBSRC)/lib/systemd/system/
	@mkdir -p $(DEBSRC)/usr/local/wigo/lib
	@mkdir -p $(DEBSRC)/usr/local/wigo/bin
	@mkdir -p $(DEBSRC)/usr/local/wigo/etc/conf.d
	@mkdir -p $(DEBSRC)/usr/local/wigo/probes/examples
	@mkdir -p $(DEBSRC)/usr/local/wigo/probes/60
	@mkdir -p $(DEBSRC)/usr/local/wigo/probes/120
	@mkdir -p $(DEBSRC)/usr/local/wigo/probes/300
	@mkdir -p $(DEBSRC)/usr/local/bin
	@mkdir -p $(DEBSRC)/var/lib/wigo
	@cp -R build/deb/DEBIAN $(DEBSRC)
	@cp -R lib/* $(DEBSRC)/usr/local/wigo/lib/
	@cp probes/examples/* $(DEBSRC)/usr/local/wigo/probes/examples
	@cp etc/wigo.conf $(DEBSRC)/usr/local/wigo/etc/wigo.conf.sample
	@cp etc/conf.d/*.conf $(DEBSRC)/usr/local/wigo/etc/conf.d
	@cp etc/wigo.systemd $(DEBSRC)/lib/systemd/system/wigo.service
	@cp etc/wigo.logrotate $(DEBSRC)/etc/logrotate.d/wigo
	@cp -R public $(DEBSRC)/usr/local/wigo
	@sed -i "s/##VERSION##/Wigo v$(RELEASE_VERSION)/" $(DEBSRC)/usr/local/wigo/public/index.html
	@for arch in amd64 armhf ; do \
		echo "Building Wigo Debian package for $$arch to $(DEBSRC)"; \
		cp -R build/deb/DEBIAN/control $(DEBSRC)/DEBIAN/control ; \
		sed -i "s/^Version:.*/Version: $(RELEASE_VERSION)/" $(DEBSRC)/DEBIAN/control ; \
		sed -i "s/^Architecture:.*/Architecture: $$arch/" $(DEBSRC)/DEBIAN/control ; \
		if [ $$arch = 'armhf' ]; then  \
			cp release/linux-arm/* $(DEBSRC)/usr/local/wigo/bin/ ; \
		else \
			cp release/linux-$$arch/* $(DEBSRC)/usr/local/wigo/bin/ ; \
		fi ; \
		fakeroot dpkg-deb -Z xz --build $(DEBSRC) $(DEBROOT)/wigo-$(RELEASE_VERSION)-$$arch.deb ; \
	done

publish-debs:
	@echo "Publishing Wigo Debian packages to repo"
	@for arch in amd64 armhf ; do \
		for release in stretch buster bullseye bookworm trixie; do \
		  	echo "Adding package with arch $$arch and release $$release to repo $(REPOROOT)" ; \
			reprepro --ask-passphrase -b $(REPOROOT) includedeb $$release $(DEBROOT)/wigo-$(RELEASE_VERSION)-$$arch.deb ; \
		done \
	done

lint: fmt
	@FAIL=0 ;echo -n " - go fmt :" ; OUT=`gofmt -l . | grep -v ^vendor` ; \
	if [[ -z "$$OUT" ]]; then echo " OK" ; else echo " FAIL"; echo "$$OUT"; FAIL=1 ; fi ;\
	echo -n " - go vet :" ; OUT=`go vet ./...` ; \
	if [[ -z "$$OUT" ]]; then echo " OK" ; else echo " FAIL"; echo "$$OUT"; FAIL=1 ; fi ;\
	test $$FAIL -eq 0

fmt:
	@gofmt -w -s $(shell find . -type f -name '*.go' -not -path "./vendor/*" )

clean:
	@echo "Cleaning all files"
	@rm -rf release
	@rm -rf $(DEBROOT)
	@rm -rf dev
	@rm -rf public

build-dev: deps
	@echo "Building Wigo for development"
	@mkdir -p release/current
	@go build -o release/current/wigo $(BASE_DIR)/src/cmd/wigo/main.go
	@go build -o release/current/wigocli $(BASE_DIR)/src/cmd/wigocli/main.go
	@go build -o release/current/generate_cert $(BASE_DIR)/src/cmd/generate_cert/main.go


run-dev: build-dev
	@echo "Starting Wigo development server"
	mkdir -p $(BASE_DIR)/dev; \
	mkdir -p $(BASE_DIR)/dev/probes/; \
	if [ ! -d $(BASE_DIR)/dev/probes/60 ]; then mkdir -p $(BASE_DIR)/dev/probes/60; fi; \
	for probe in hardware_load_average hardware_disks hardware_memory ifstat supervisord needrestart check_mdadm check_process haproxy lm-sensors iostat check_uptime smart check_ntp packages-apt; do \
		if [ ! -e $(BASE_DIR)/dev/probes/60/$$probe ]; then \
			ln -s $(BASE_DIR)/probes/examples/$$probe $(BASE_DIR)/dev/probes/60/$$probe; \
		fi; \
	done; \
	if [ ! -d $(BASE_DIR)/dev/probes/300 ]; then mkdir -p $(BASE_DIR)/dev/probes/300; fi; \
	for probe in smart check_ntp packages-apt; do \
		if [ ! -e $(BASE_DIR)/dev/probes/300/$$probe ]; then \
			ln -s $(BASE_DIR)/probes/examples/$$probe $(BASE_DIR)/dev/probes/300/$$probe; \
		fi; \
	done; \
	if [ ! -e $(BASE_DIR)/dev/lib ]; then \
		ln -s $(BASE_DIR)/lib $(BASE_DIR)/dev/lib; \
	fi; \
	if [ ! -e $(BASE_DIR)/dev/wigo.crt ]; then \
		(cd $(BASE_DIR)/dev && $(BASE_DIR)/release/current/generate_cert -ca=true -duration=87600h0m0s -host "127.0.0.1" --rsa-bits=4096;) \
	fi; \
	# Try without sudo first, use sudo only if needed
	$(BASE_DIR)/release/current/wigo --config $(BASE_DIR)/etc/wigo-dev.conf & \
	WIGO_PID=$$!; \
	sleep 1; \
	if ! kill -0 $$WIGO_PID 2>/dev/null; then \
		sudo $(BASE_DIR)/release/current/wigo --config $(BASE_DIR)/etc/wigo-dev.conf & \
		WIGO_PID=$$!; \
	fi; \
	sleep 1; \
	cd $(BASE_DIR)/src/public && npm run pretty && npm run watch; \
	EXIT_CODE=$$?; \
	# Cleanup: kill process and wait for it to terminate
	if [ -n "$$WIGO_PID" ]; then kill $$WIGO_PID 2>/dev/null; wait $$WIGO_PID 2>/dev/null; fi; \
	exit $$EXIT_CODE
