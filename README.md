Wigo
=========

Wigo, aka "What Is Going On" is a light pull/push monitoring tool written in Golang.

Main features

  - Write probes in any language you want
  - Notifications when a probe status change (http,email)
  - Proxy mode for hosts behind NAT/Gateways
  - Graphing probes metrics to OpenTSDB instances

Screenshots:

Web UI Main view:
![Main view](http://pix.toile-libre.org/upload/original/1452263131.png)

Web UI Group view:
![Group view](http://pix.toile-libre.org/upload/original/1452263323.png)

Web UI Host view:
![Host view](http://pix.toile-libre.org/upload/original/1452263238.png)


Installation
--------------

### Installation :

#### Debian Buster :
```sh
echo "deb http://deb.carsso.com buster main" > /etc/apt/sources.list.d/deb.carsso.com.list
wget -O- http://deb.carsso.com/deb.carsso.com.key | apt-key add -
apt-get update
apt-get install wigo

#### Debian Stretch :
```sh
echo "deb http://deb.carsso.com stretch main" > /etc/apt/sources.list.d/deb.carsso.com.list
wget -O- http://deb.carsso.com/deb.carsso.com.key | apt-key add -
apt-get update
apt-get install wigo

#### Debian Jessie :
```sh
echo "deb http://deb.carsso.com jessie main" > /etc/apt/sources.list.d/deb.carsso.com.list
wget -O- http://deb.carsso.com/deb.carsso.com.key | apt-key add -
apt-get update
apt-get install wigo

#### Centos 6 & 7 :
```sh
echo -e "[carsso]\nname=Carsso\nbaseurl=http://rpm.carsso.com\ngpgcheck=0" > /etc/yum.repos.d/carsso.repo
yum clean all
yum install wigo yum-plugin-security
```

### Configuration

Wigo configuration is in /etc/wigo/wigo.conf

Probes configurations are in /etc/wigo/conf.d/

### Default probes 

The directory name is the interval of check in seconds

```sh
/usr/local/wigo/probes
├── 120
├── 300
│   ├── apt-security -> ../../probes-examples/apt-security
│   └── smart -> ../../probes-examples/smart
└── 60
    ├── check_mdadm -> ../../probes-examples/check_mdadm
    ├── check_processes
    ├── hardware_disks -> ../../probes-examples/hardware_disks
    ├── hardware_load_average -> ../../probes-examples/hardware_load_average
    ├── hardware_memory -> ../../probes-examples/hardware_memory
    ├── ifstat -> ../../probes-examples/ifstat
    └── supervisord

```

Get status
-----------

### Wigo web interface

`http://[your-ip]:4000/` (by default)


#### Wigo CLI


```sh
# wigocli
Wigo v0.51.5 running on backbone.root.gg 
Local Status    : 100
Global Status   : 250

Remote Wigos : 

    1.2.3.4:4000 ( ns2 ) - Wigo v0.51.5: 
        apt-security              : 100  No security packages availables
        hardware_disks            : 250  Highest occupation percentage is 93% in partition /dev/md0
        hardware_load_average     : 100  0.09 0.04 0.05
        hardware_memory           : 100  Current memory usage is 19.32%
        ifstat                    : 100  tap0 0.00/0.01 mbps , eth1 0.00/0.00 mbps , eth0 0.01/0.01 mbps , 
        smart                     : 100  /dev/sdc : PASSED /dev/sdb : PASSED 

```


Write your own probes !
-------------------------
Probes are binaires, written in any language you want, that output a json string with at least Status param :
```sh

$ ./hardware_load_average
{
   "Detail" : "",
   "Version" : "0.11",
   "Message" : "0.38 0.26 0.24",
   "Status" : 100,
   "Metrics" : [
      {
         "Value" : 0.38,
         "Tags" : {
            "load" : "load5"
         }
      },
      {
         "Value" : 0.26,
         "Tags" : {
            "load" : "load10"
         }
      },
      {
         "Value" : 0.24,
         "Tags" : {
            "load" : "load15"
         }
      }
   ]
}
```

Status codes :
--------------

```
    100         OK
    101 to 199  INFO
    200 to 299  WARN
    300 to 499  CRIT
    500+        ERROR
```

Building from sources
---------------------

### Environment
```sh
export GOPATH="$GOPATH:..../wigo/src"
```

### Dependencies
```sh
go get github.com/docopt/docopt-go
go get github.com/root-gg/gopentsdb
go get github.com/fatih/color
go get github.com/nu7hatch/gouuid
go get github.com/BurntSushi/toml
go get github.com/codegangsta/martini
go get github.com/docopt/docopt-go
go get github.com/fatih/color
go get github.com/lann/squirrel
go get github.com/mattn/go-sqlite3
go get github.com/nu7hatch/gouuid
go get github.com/codegangsta/martini
go get github.com/codegangsta/martini-contrib/auth
go get github.com/codegangsta/martini-contrib/gzip
go get github.com/codegangsta/martini-contrib/secure
go get github.com/howeyc/fsnotify
```
or (the ugly way)
```sh
go install -v src/wigo.go 2>&1 | grep "cannot find package" | grep -Po '"github.com.*"' | sed 's/"//g' | while read line; do go get "$line"; done
go install -v src/wigocli.go 2>&1 | grep "cannot find package" | grep -Po '"github.com.*"' | sed 's/"//g' | while read line; do go get "$line"; done
```

### Building
```sh
go build src/wigo.go
go build src/wigocli.go
```

### Build DEB (deb-building tools required)

```sh
cd build/deb/
./makePackege.sh
```

### Build RPM (rpm-building tools required)

```sh
cd build/rpm/
./makePackege.sh
```

PULL
----

The master fetch remote clients over the http api.
Communications should be secured using firewall rules, TLS and http basic authentication.

PUSH
----

Clients send their data to a remote master over a persistent tcp connection.
Communications should be secured using firewall rules and TLS.

TLS
---

Wigo needs a key pair to enable HTTPS api.
One can generate a self signed key pair by running this command :
```
/usr/local/wigo/bin/generate_cert -ca=false -duration=87600h0m0s -host "hostnames,ips,..." --rsa-bits=4096
```

Wigo needs a CA key pair to secure PUSH communications.
One can generate a CA self signed key pair by running this command :
```
/usr/local/wigo/bin/generate_cert -ca=true -duration=87600h0m0s -host "hostnames,ips,..." --rsa-bits=4096
```
