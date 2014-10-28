Name:		wigo
Version:	##VERSION##
Release:	1%{?dist}
Summary:	WiGo is a monitoring tool that just works

License:	MIT
URL:		https://git.root.gg/bodji/wigo
Source0:	wigo.tar.bz2
Requires:   ntp-perl, perl-JSON, perl-Time-HiRes
AutoReqProv: no

%description
WiGo is a monitoring tool that just works



%prep
%setup -n wigo



%build
mkdir -p build
go build -o build/wigo src/wigo.go
go build -o build/wigocli src/wigocli.go



%install

# Create package subdirs
test -d %{buildroot} && rm -rf %{buildroot}
mkdir -p %{buildroot}
mkdir -p %{buildroot}/etc/wigo/conf.d
mkdir -p %{buildroot}/etc/cron.d
mkdir -p %{buildroot}/etc/logrotate.d
mkdir -p %{buildroot}/etc/init.d
mkdir -p %{buildroot}/usr/local/wigo/bin
mkdir -p %{buildroot}/usr/local/wigo/lib
mkdir -p %{buildroot}/usr/local/wigo/etc/conf.d
mkdir -p %{buildroot}/usr/local/wigo/probes/examples
mkdir -p %{buildroot}/usr/local/bin
mkdir -p %{buildroot}/var/lib/wigo

cp build/wigo %{buildroot}/usr/local/wigo/bin/wigo
cp build/wigocli %{buildroot}/usr/local/bin/wigocli

# Copy lib
cp -R lib/* %{buildroot}/usr/local/wigo/lib/

# Copy probes
cp probes/examples/* %{buildroot}/usr/local/wigo/probes/examples

# Copy config && probes default config files
cp etc/wigo.conf %{buildroot}/etc/wigo/wigo.conf
cp etc/wigo.conf %{buildroot}/usr/local/wigo/etc/wigo.conf.sample
cp etc/conf.d/*.conf %{buildroot}/usr/local/wigo/etc/conf.d

# Copy init script
cp build/rpm/wigo.init %{buildroot}/etc/init.d/wigo

# Copy cron.d
cp etc/wigo.cron %{buildroot}/etc/cron.d/wigo

# Copy logrotate
cp etc/wigo.logrotate %{buildroot}/etc/logrotate.d/wigo

# Copy public directory
cp -R public %{buildroot}/usr/local/wigo



%clean
rm -rf %{buildroot}



%files
%defattr(-,root,root,-)
/etc/init.d/wigo
/etc/wigo
%config(noreplace) /etc/wigo/wigo.conf
%config(noreplace) /etc/wigo/conf.d
/etc/logrotate.d/wigo
/etc/cron.d/wigo
/usr/local/wigo
/usr/local/bin/wigocli

%post
WIGOPATH="/usr/local/wigo"
EXAMPLEPROBES60=( hardware_load_average hardware_disks hardware_memory ifstat supervisord check_mdadm check_process haproxy lm-sensors iostat check_uptime)
EXAMPLEPROBES300=( smart check_ntp packages-apt )

# Enabling default probes on 60 directory
echo "Enabling default probes.."

mkdir -p $WIGOPATH/probes/60
cd $WIGOPATH/probes/60
for probeName in ${EXAMPLEPROBES60[@]}; do
    if [ ! -e $probeName ] ; then
        echo " - Enabling $probeName every 60 seconds"
        ln -s ../examples/$probeName .
    else
        echo " - Probe $probeName already exists. Doing nothing.."
    fi
done

mkdir -p $WIGOPATH/probes/300
cd $WIGOPATH/probes/300
for probeName in ${EXAMPLEPROBES300[@]}; do
    if [ ! -e $probeName ] ; then
        echo " - Enabling $probeName every 300 seconds"
        ln -s ../examples/$probeName .
    else
        echo " - Probe $probeName already exists. Doing nothing.."
    fi
done

mkdir -p /var/lib/wigo
mkdir -p /var/log/wigo

# Start it
/etc/init.d/wigo restart



%changelog
