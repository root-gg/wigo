#!/bin/bash

set -e

export GOPATH=/root/go:/opt/wigo/repository/

PACKAGEROOT=/opt/wigo/packages/wigo
REPOROOT=/opt/wigo/repository

cd $REPOROOT/src

# Replace version in code
VERSION=$(cat ../VERSION)

sed -i "s/##VERSION##/Wigo v$VERSION/" wigo/global.go
go build -o bin/wigo wigo.go || exit
go build -o bin/wigocli wigocli.go || exit
git checkout wigo/global.go


cp bin/wigo $PACKAGEROOT/usr/local/wigo/bin
cp bin/wigocli $PACKAGEROOT/usr/local/bin/wigocli

# Copy lib
mkdir -p $PACKAGEROOT/usr/local/wigo/lib
mkdir -p $PACKAGEROOT/var/lib/wigo

cp -R $REPOROOT/lib/* $PACKAGEROOT/usr/local/wigo/lib/

# Copy probes
mkdir -p $PACKAGEROOT/usr/local/wigo/probes/examples
cp $REPOROOT/probes/examples/* $PACKAGEROOT/usr/local/wigo/probes/examples

# Copy config && probes default config files
mkdir -p $PACKAGEROOT/etc/wigo/conf.d
mkdir -p $PACKAGEROOT/usr/local/wigo/etc/conf.d
cp $REPOROOT/etc/wigo.conf $PACKAGEROOT/usr/local/wigo/etc/wigo.conf.sample
cp $REPOROOT/etc/conf.d/*.conf $PACKAGEROOT/usr/local/wigo/etc/conf.d

# Copy Init Script
mkdir -p $PACKAGEROOT/etc/init.d
cp $REPOROOT/etc/wigo.init $PACKAGEROOT/etc/init.d/wigo

# Copy cron.d
mkdir -p $PACKAGEROOT/etc/cron.d
cp $REPOROOT/etc/wigo.cron $PACKAGEROOT/etc/cron.d/wigo

# Copy logrotate
mkdir -p $PACKAGEROOT/etc/logrotate.d
cp $REPOROOT/etc/wigo.logrotate $PACKAGEROOT/etc/logrotate.d/wigo

# Copy public directory
if [ -e "$PACKAGEROOT/usr/local/wigo/public" ] ; then
    rm -fr $PACKAGEROOT/usr/local/wigo/public
fi
cp -R $REPOROOT/public $PACKAGEROOT/usr/local/wigo


# Build
cd /opt/wigo/packages/

# Replace version
sed -i "s/^Version:.*/Version: $VERSION/" wigo/DEBIAN/control

dpkg-deb --build wigo

# Add to mir
reprepro --ask-passphrase -b /var/www/mir.root.gg includedeb wheezy /opt/wigo/packages/wigo.deb
reprepro --ask-passphrase -b /var/www/mir.root.gg includedeb jessie /opt/wigo/packages/wigo.deb
