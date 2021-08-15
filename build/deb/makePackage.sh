#!/bin/bash

set -e

BUILDDIR=$(pwd)
PACKAGEROOT=/tmp/wigoBuild
REPOROOT=$(readlink -f ../..)
DEBMIRRORROOT=/var/www/mir.root.gg
GOCROSSCOMPILEFILE=/root/golang-crosscompile/crosscompile.bash

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
#mkdir -p $PACKAGEROOT/etc/cron.d
mkdir -p $PACKAGEROOT/etc/logrotate.d
mkdir -p $PACKAGEROOT/etc/init.d
mkdir -p $PACKAGEROOT/usr/local/wigo/lib
mkdir -p $PACKAGEROOT/usr/local/wigo/bin
mkdir -p $PACKAGEROOT/usr/local/wigo/etc/conf.d
mkdir -p $PACKAGEROOT/usr/local/wigo/probes/examples
mkdir -p $PACKAGEROOT/usr/local/wigo/probes/60
mkdir -p $PACKAGEROOT/usr/local/wigo/probes/120
mkdir -p $PACKAGEROOT/usr/local/wigo/probes/300
mkdir -p $PACKAGEROOT/usr/local/bin
mkdir -p $PACKAGEROOT/var/lib/wigo


# Replace version in code
cd $REPOROOT/src
VERSION=$(cat ../VERSION)

sed -i "s/##VERSION##/Wigo v$VERSION/" wigo/global.go
echo "Compiling wigo && wigocli"
echo " - Building amd64 versions..."
go build -o bin/wigo wigo.go || exit
go build -o bin/wigocli wigocli.go || exit
go build -o bin/generate_cert generate_cert.go || exit

if [ -e $GOCROSSCOMPILEFILE ] ; then
    source $GOCROSSCOMPILEFILE

    #if `hash go-linux-arm` ; then
    #    echo " - Building ARM versions..."
    #    go-linux-arm build -o bin/wigo_arm wigo.go || exit
    #    go-linux-arm build -o bin/wigocli_arm wigocli.go || exit
    #    go-linux-arm build -o bin/generate_cert generate_cert.go || exit
    #fi

    #if `hash go-linux-386` ; then
    #    echo " - Building i386 versions..."
    #    go-linux-386 build -o bin/wigo_386 wigo.go || exit
    #    go-linux-386 build -o bin/wigocli_386 wigocli.go || exit
    #    go-linux-386 build -o bin/generate_cert generate_cert.go || exit
    #fi
fi

git checkout wigo/global.go


echo "Copying files to package temporary directory"
cp bin/wigo $PACKAGEROOT/usr/local/wigo/bin
cp bin/wigocli $PACKAGEROOT/usr/local/bin/wigocli
cp bin/generate_cert $PACKAGEROOT/usr/local/wigo/bin/generate_cert

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
# cp $REPOROOT/etc/wigo.cron $PACKAGEROOT/etc/cron.d/wigo

# Copy logrotate
cp $REPOROOT/etc/wigo.logrotate $PACKAGEROOT/etc/logrotate.d/wigo

# Copy public directory
cp -R $REPOROOT/public $PACKAGEROOT/usr/local/wigo

# Replace version
sed -i "s/^Version:.*/Version: $VERSION/" $PACKAGEROOT/DEBIAN/control

# Add to mir
if [[ $1 == "dev" ]] ; then
    sed -i "s/^Package:.*/Package: wigo-dev/" $PACKAGEROOT/DEBIAN/control
fi

echo "Building deb packages."
echo " - Building amd64 deb..."
dpkg-deb --build $PACKAGEROOT /tmp/wigo.deb

#if [ -e $REPOROOT/src/bin/wigo_arm ] ; then
#    echo " - Building arm deb..."
#    sed -i "s/^Architecture:.*/Architecture: armhf/" $PACKAGEROOT/DEBIAN/control
#    cp $REPOROOT/src/bin/wigo_arm $PACKAGEROOT/usr/local/wigo/bin/wigo
#    cp $REPOROOT/src/bin/wigocli_arm $PACKAGEROOT/usr/local/bin/wigocli
#    dpkg-deb --build $PACKAGEROOT /tmp/wigo_arm.deb
#fi


#if [ -e $REPOROOT/src/bin/wigo_386 ] ; then
#    echo " - Building i386 deb..."
#    sed -i "s/^Architecture:.*/Architecture: i386/" $PACKAGEROOT/DEBIAN/control
#    cp $REPOROOT/src/bin/wigo_386 $PACKAGEROOT/usr/local/wigo/bin/wigo
#    cp $REPOROOT/src/bin/wigocli_386 $PACKAGEROOT/usr/local/bin/wigocli
#    dpkg-deb --build $PACKAGEROOT /tmp/wigo_386.deb
#fi

reprepro --ask-passphrase -b $DEBMIRRORROOT includedeb stretch /tmp/wigo.deb
reprepro --ask-passphrase -b $DEBMIRRORROOT includedeb buster /tmp/wigo.deb
reprepro --ask-passphrase -b $DEBMIRRORROOT includedeb bullseye /tmp/wigo.deb

#if [ -e /tmp/wigo_arm.deb ] ; then
#    reprepro --ask-passphrase -b $DEBMIRRORROOT includedeb stretch /tmp/wigo_arm.deb
#fi

#if [ -e /tmp/wigo_386.deb ] ; then
#    reprepro --ask-passphrase -b $DEBMIRRORROOT includedeb stretch /tmp/wigo_386.deb
#fi
