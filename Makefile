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
	REPOROOT="/repo-root/"
endif

race_detector = GORACE="halt_on_error=1" go build -race
ifdef ENABLE_RACE_DETECTOR
	build = $(race_detector)
else
	build = go build
endif
test: build = $(race_detector)

all: clean release

release: deps
	@mkdir -p release
	@cp $(BASE_DIR)/src/wigo/global.go $(BASE_DIR)/src/wigo/global.go.bkp
	@cd release && for target in $(RELEASE_TARGETS) ; do \
		RELEASE_DIR=$(BASE_DIR)/release/$$target; \
		sed -i "s/##VERSION##/Wigo v$(RELEASE_VERSION)/" $(BASE_DIR)/src/wigo/global.go;\
		export CGO_ENABLED=1; \
		export GOPATH=`echo "$(GOPATH):$(BASE_DIR)"`; \
		echo $(GOPATH); \
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
		echo "Building Wigo for $$target to $$RELEASE_DIR"; \
		$(build) -o $$RELEASE_DIR/wigo $(BASE_DIR)/src/wigo.go;	\
		$(build) -o $$RELEASE_DIR/wigocli $(BASE_DIR)/src/wigocli.go;	\
		$(build) -o $$RELEASE_DIR/generate_cert $(BASE_DIR)/src/generate_cert.go;	\
		cp $(BASE_DIR)/src/wigo/global.go.bkp $(BASE_DIR)/src/wigo/global.go; \
	done
	@rm $(BASE_DIR)/src/wigo/global.go.bkp

debs: release
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

publish-debs: debs
	@for arch in amd64 armhf ; do \
		for release in stretch buster ; do \
		  	echo "Adding package with arch $$arch and release $$release to repo" ; \
			reprepro --ask-passphrase -b $(DEBMIRRORROOT) includedeb $$release $(DEBROOT)/wigo-$(RELEASE_VERSION)-$$arch.deb ; \
		done \
	done

lint:
	@FAIL=0 ;echo -n " - go fmt :" ; OUT=`gofmt -l . | grep -v ^vendor` ; \
	if [[ -z "$$OUT" ]]; then echo " OK" ; else echo " FAIL"; echo "$$OUT"; FAIL=1 ; fi ;\
	echo -n " - go vet :" ; OUT=`go vet ./...` ; \
	if [[ -z "$$OUT" ]]; then echo " OK" ; else echo " FAIL"; echo "$$OUT"; FAIL=1 ; fi ;\
	echo -n " - go lint :" ; OUT=`golint ./... | grep -v ^vendor` ; \
	if [[ -z "$$OUT" ]]; then echo " OK" ; else echo " FAIL"; echo "$$OUT"; FAIL=1 ; fi ;\
	test $$FAIL -eq 0

clean:
	@rm -rf release
	@rm -rf $(DEBROOT)

deps:
	@go get -d ./...
