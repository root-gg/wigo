#!/bin/bash

set -e

BUILDDIR=$(pwd)
PACKAGEROOT=/tmp/wigoBuild
REPOROOT=$(readlink -f ../..)
DEBMIRRORROOT=/var/www/mir.root.gg

# Test if we are in the right directory
if [ ! -e $REPOROOT/src/wigo.go ] ; then 
    echo "You must be in the build/deb directory of repository to build!"
    exit 0;
fi

# Create
if [ ! -e $PACKAGEROOT ] ; then
    rm -fr $PACKAGEROOT
fi

# Create package subdirs
mkdir -p $PACKAGEROOT
mkdir -p $PACKAGEROOT/etc/wigo/conf.d
mkdir -p $PACKAGEROOT/etc/cron.d
mkdir -p $PACKAGEROOT/etc/logrotate.d
mkdir -p $PACKAGEROOT/etc/init.d
mkdir -p $PACKAGEROOT/usr/local/wigo/lib
mkdir -p $PACKAGEROOT/usr/local/wigo/etc/conf.d
mkdir -p $PACKAGEROOT/usr/local/wigo/probes/examples
mkdir -p $PACKAGEROOT/usr/local/bin
mkdir -p $PACKAGEROOT/var/lib/wigo


# Replace version in code
cd $REPOROOT/src
VERSION=$(cat ../VERSION)

sed -i "s/##VERSION##/Wigo v$VERSION/" wigo/global.go
go build -o bin/wigo wigo.go || exit
go build -o bin/wigocli wigocli.go || exit
git checkout wigo/global.go


cp bin/wigo $PACKAGEROOT/usr/local/wigo/bin
cp bin/wigocli $PACKAGEROOT/usr/local/bin/wigocli

# Copy DEBIAN folder
cp -R $BUILDDIR/DEBIAN $PACKAGEROOT

# Copy lib
cp -R $REPOROOT/lib/* $PACKAGEROOT/usr/local/wigo/lib/

# Copy probes
cp $REPOROOT/probes/examples/* $PACKAGEROOT/usr/local/wigo/probes/examples

# Copy config && probes default config files
cp $REPOROOT/etc/wigo.conf $PACKAGEROOT/usr/local/wigo/etc/wigo.conf.sample
cp $REPOROOT/etc/conf.d/*.conf $PACKAGEROOT/usr/local/wigo/etc/conf.d

# Copy Init Script
cp $REPOROOT/etc/wigo.init $PACKAGEROOT/etc/init.d/wigo

# Copy cron.d
cp $REPOROOT/etc/wigo.cron $PACKAGEROOT/etc/cron.d/wigo

# Copy logrotate
cp $REPOROOT/etc/wigo.logrotate $PACKAGEROOT/etc/logrotate.d/wigo

# Copy public directory
cp -R $REPOROOT/public $PACKAGEROOT/usr/local/wigo

# Replace version
sed -i "s/^Version:.*/Version: $VERSION/" $BUILDDIR/DEBIAN/control
git checkout $BUILDDIR/DEBIAN/control

# Add to mir
dpkg-deb --build $PACKAGEROOT $PACKAGEROOT/wigo.deb
reprepro --ask-passphrase -b $DEBMIRRORROOT includedeb wheezy $PACKAGEROOT/wigo.deb
reprepro --ask-passphrase -b $DEBMIRRORROOT includedeb jessie $PACKAGEROOT/wigo.deb

# Remove folder
rm -fr $DEBMIRRORROOT
