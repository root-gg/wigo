#!/bin/sh

###
# LOCATE ROOT
###

REPO_ROOT=$(readlink -f ../../../wigo 2>&1)

if [ -z "$REPO_ROOT" ] || [ ! -d $REPO_ROOT ]; then
    echo "Run build script from wigo/build/rpm directory"
    exit 1
fi

cd $REPO_ROOT

###
# REPLACE VERSION
###

VERSION=$(cat VERSION)
git checkout build/rpm/wigo.spec
git checkout src/wigo/global.go
git checkout public/index.html
sed -i "s/##VERSION##/$VERSION/"        build/rpm/wigo.spec
sed -i "s/##VERSION##/Wigo v$VERSION/"  src/wigo/global.go
sed -i "s/##VERSION##/$VERSION/"        public/index.html

###
# CREATE BUILD DIRECTORY
###

SRC_ROOT=$(mktemp -d)
tar czvf  $SRC_ROOT/wigo.tar.bz2 -C $REPO_ROOT wigo

###
# BUILD RPM PACKAGE
###

rpmbuild --define "_sourcedir $SRC_ROOT" -ba build/rpm/wigo.spec