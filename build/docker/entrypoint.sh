#!/bin/sh
set -e

# Seeding on first start, and only on first start.
#
# The probes directory is the source of truth for which probes run and how
# often, and it is a volume. Re-linking the defaults on every start would undo
# every choice an operator made through the interface -- the same mistake the
# debian postinst used to make on every upgrade.
#
# Presence of the marker is what says "this volume has been set up", not the
# presence of the links themselves : a volume where somebody disabled all the
# default probes is set up, and must not be re-seeded.

WIGOPATH=/usr/local/wigo
SEEDED="$WIGOPATH/probes/.seeded"

DEFAULT_60="hardware_load_average hardware_disks hardware_memory ifstat check_uptime"
DEFAULT_300="check_ntp"

mkdir -p /etc/wigo/conf.d /var/lib/wigo

if [ ! -f /etc/wigo/wigo.conf ] ; then
    echo "No configuration file, installing the sample one"
    cp "$WIGOPATH/etc/wigo.conf.sample" /etc/wigo/wigo.conf
fi

for config in "$WIGOPATH"/etc/conf.d/*.conf ; do
    name=$(basename "$config")
    if [ ! -f "/etc/wigo/conf.d/$name" ] ; then
        cp "$config" "/etc/wigo/conf.d/$name"
    fi
done

# A named volume is seeded from the image, a bind mount onto an empty directory
# is not. Either way the probes have to be there, or wigo monitors nothing.
mkdir -p "$WIGOPATH/probes/examples"
cp -n /usr/local/share/wigo/probes-examples/* "$WIGOPATH/probes/examples/" 2>/dev/null || true

if [ ! -f "$SEEDED" ] ; then
    echo "First start, enabling the default probes"

    for interval in 60 300 ; do
        mkdir -p "$WIGOPATH/probes/$interval"
    done

    # In a subshell : wigo resolves its public directory and its probe library
    # relative to the working directory, so this must not leave it moved.
    seed() {
        interval=$1
        shift

        cd "$WIGOPATH/probes/$interval" || return 1
        for probe in "$@" ; do
            if [ -e "../examples/$probe" ] && [ ! -e "$probe" ] ; then
                echo " - Enabling $probe every $interval seconds"
                ln -s "../examples/$probe" .
            fi
        done
    }

    ( seed 60 $DEFAULT_60 )
    ( seed 300 $DEFAULT_300 )

    touch "$SEEDED"
fi

echo "Starting in $(pwd)"

exec "$@"
