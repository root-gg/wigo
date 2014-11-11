Wigo
=========

Wigo, aka "What Is Going On" is a light pull/push monitoring tool written in Golang.

Main features

  - Write probes in any language you want
  - Notifications when a probe status change (http,email)
  - Proxy mode for hosts behind NAT/Gateways
  - Graphing probes metrics to OpenTSDB instances


Version
----
0.51


Installation
--------------

On Debian Wheezy : 

##### Add repository and install package
```sh
echo "deb http://mir.root.gg wheezy main" > /etc/apt/sources.list.d/mir.root.gg.list
curl http://mir.root.gg/gg.key | apt-key add -
apt-get install wigo
```

##### Configuration
Edit /etc/wigo.conf

##### Default probes 

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

##### Get status

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

```
go get github.com/BurntSushi/toml
go get github.com/bodji/gopentsdb  
go get github.com/codegangsta/martini
go get github.com/fatih/color
go get github.com/howeyc/fsnotify
go get github.com/nu7hatch/gouuid
go get github.com/docopt/docopt-go

export GOPATH="$GOPATH:..../wigo/src"
go build src/wigo.go
go build src/wigocli.go
```


openssl req -x509 -nodes -days 3650 -newkey rsa:4096 -keyout etc/ssl/wigo.key -out etc/ssl/wigo.crt