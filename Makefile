SHELL = /bin/bash

RELEASE_DIR="release/plik-$(RELEASE_VERSION)"
BASE_DIR:=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
RELEASE_VERSION=`cat $(BASE_DIR)/VERSION`
RELEASE_TARGETS=linux-amd64 linux-arm
GOHOSTOS=$(shell go env GOHOSTOS)
GOHOSTARCH=$(shell go env GOHOSTARCH)

DEBROOT=debs
RPMROOT=rpms

ifdef REPOROOT
else
	REPOROOT="/repo-root"
endif

race_detector = GORACE="halt_on_error=1" go build -race
ifdef ENABLE_RACE_DETECTOR
	build = $(race_detector)
else
	build = go build
endif
test: build = $(race_detector)

all: clean release

releases:
	@echo "Building Wigo releases"
	@mkdir -p release
	@cd release && for target in $(RELEASE_TARGETS) ; do \
		RELEASE_DIR=$(BASE_DIR)/release/$$target; \
		export CGO_ENABLED=1; \
		export GOOS=`echo $$target | cut -d "-" -f 1`; 	\
		export GOARCH=`echo $$target | cut -d "-" -f 2`; \
		if [ $$target = 'linux-arm' ]; then  \
			export GOARM='7'; \
			export CC='arm-linux-gnueabihf-gcc'; \
		else \
			export GOARM=; \
			export CC=; \
	    fi ; \
		mkdir $$RELEASE_DIR; \
		echo "Building Wigo release for $$target to $$RELEASE_DIR"; \
		$(build) -ldflags "-X wigo.Version=$(RELEASE_VERSION)" -o $$RELEASE_DIR/wigo $(BASE_DIR)/src/wigo.go; \
		$(build) -ldflags "-X wigo.Version=$(RELEASE_VERSION)" -o $$RELEASE_DIR/wigocli $(BASE_DIR)/src/wigocli.go; \
		$(build) -o $$RELEASE_DIR/generate_cert $(BASE_DIR)/src/generate_cert.go; \
	done

release:
	@echo "Building Wigo release for current OS"
	@mkdir -p release
	@cd release; \
	$(build) -ldflags "-X wigo.Version=$(RELEASE_VERSION)" -o current/wigo $(BASE_DIR)/src/wigo.go;	\
	$(build) -ldflags "-X wigo.Version=$(RELEASE_VERSION)" -o current/wigocli $(BASE_DIR)/src/wigocli.go; \
	$(build) -o current/generate_cert $(BASE_DIR)/src/generate_cert.go

debs:
	@echo "Building Wigo Debian packages"
	@mkdir -p $(DEBROOT)
	@mkdir -p $(DEBROOT)/etc/wigo/conf.d
	@mkdir -p $(DEBROOT)/etc/logrotate.d
	@mkdir -p $(DEBROOT)/etc/init.d
	@mkdir -p $(DEBROOT)/usr/local/wigo/lib
	@mkdir -p $(DEBROOT)/usr/local/wigo/bin
	@mkdir -p $(DEBROOT)/usr/local/wigo/etc/conf.d
	@mkdir -p $(DEBROOT)/usr/local/wigo/probes/examples
	@mkdir -p $(DEBROOT)/usr/local/wigo/probes/60
	@mkdir -p $(DEBROOT)/usr/local/wigo/probes/120
	@mkdir -p $(DEBROOT)/usr/local/wigo/probes/300
	@mkdir -p $(DEBROOT)/usr/local/bin
	@mkdir -p $(DEBROOT)/var/lib/wigo
	@cp -R build/deb/DEBIAN $(DEBROOT)
	@cp -R lib/* $(DEBROOT)/usr/local/wigo/lib/
	@cp probes/examples/* $(DEBROOT)/usr/local/wigo/probes/examples
	@cp etc/wigo.conf $(DEBROOT)/usr/local/wigo/etc/wigo.conf.sample
	@cp etc/conf.d/*.conf $(DEBROOT)/usr/local/wigo/etc/conf.d
	@cp etc/wigo.init $(DEBROOT)/etc/init.d/wigo && chmod +x $(DEBROOT)/etc/init.d/wigo
	@cp etc/wigo.logrotate $(DEBROOT)/etc/logrotate.d/wigo
	@cp -R public $(DEBROOT)/usr/local/wigo
	@sed -i "s/##VERSION##/Wigo v$(RELEASE_VERSION)/" $(DEBROOT)/usr/local/wigo/public/index.html
	@for arch in amd64 armhf ; do \
		echo "Building Wigo Debian package for $$arch to $(DEBROOT)"; \
		cp -R build/deb/DEBIAN/control $(DEBROOT)/DEBIAN/control ; \
		sed -i "s/^Version:.*/Version: $(RELEASE_VERSION)/" $(DEBROOT)/DEBIAN/control ; \
		sed -i "s/^Architecture:.*/Architecture: $$arch/" $(DEBROOT)/DEBIAN/control ; \
		if [ $$arch = 'armhf' ]; then  \
			cp release/linux-arm/* $(DEBROOT)/usr/local/wigo/bin/ ; \
		else \
			cp release/linux-$$arch/* $(DEBROOT)/usr/local/wigo/bin/ ; \
		fi ; \
		dpkg-deb --build $(DEBROOT) $(DEBROOT)/wigo-$(RELEASE_VERSION)-$$arch.deb ; \
	done

publish-debs:
	@echo "Publishing Wigo Debian packages to repo"
	@for arch in amd64 armhf ; do \
		for release in stretch buster bullseye; do \
		  	echo "Adding package with arch $$arch and release $$release to repo $(REPOROOT)" ; \
			reprepro --ask-passphrase -b $(REPOROOT) includedeb $$release $(DEBROOT)/wigo-$(RELEASE_VERSION)-$$arch.deb ; \
		done \
	done

lint:
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

deps:
	@echo "Installing dependencies"
	go mod download -x
