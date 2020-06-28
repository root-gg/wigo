Wigo
=========

Wigo, aka "What Is Going On" is a light pull/push monitoring tool written in Golang.

Main features
  - Write probes in any language you want
  - Notifications when a probe status change (http,email)
  - Proxy mode for hosts behind NAT/Gateways
  - Graphing probes metrics to OpenTSDB instances

[Install Wigo Monitoring from Chrome Web Store](https://chrome.google.com/webstore/detail/wigo-monitoring/eaoeankffafdpnhgnamdlifgaknjdcog)

#### Screenshots:

##### Web UI Main view:
![Main view](http://pix.toile-libre.org/upload/original/1452263131.png)

##### Web UI Group view:
![Group view](http://pix.toile-libre.org/upload/original/1452263323.png)

##### Web UI Host view:
![Host view](http://pix.toile-libre.org/upload/original/1452263238.png)


### Installation

> Warning : Breaking change since version 0.73.19 :
> A change in the wigo push protocol can cause crashes if your wigo push server version is < 0.73.22.
> You should update both wigo push clients and server to (at least) 0.73.22.

##### Debian 9 & 10 :
```sh
apt-get install lsb-release
wget -O- http://deb.carsso.com/deb.carsso.com.key | apt-key add -
echo "deb http://deb.carsso.com $(lsb_release --codename --short) main" > /etc/apt/sources.list.d/deb.carsso.com.list
apt-get update
apt-get install wigo
```
_Debian Jessie packages are not available anymore, it's time to upgrade :)_

##### Centos 6 & 7 :
```sh
echo -e "[carsso]\nname=Carsso\nbaseurl=http://rpm.carsso.com\ngpgcheck=0" > /etc/yum.repos.d/carsso.repo
yum clean all
yum install wigo yum-plugin-security
```

### Configuration / Setup

- Wigo configuration is in `/etc/wigo/wigo.conf`
- Probes configurations are in `/etc/wigo/conf.d/`

##### PULL
The master fetch remote clients over the http api.
Communications should be secured using firewall rules, TLS and http basic authentication.

##### PUSH
Clients send their data to a remote master over a persistent tcp connection.
Communications should be secured using firewall rules and TLS.

##### TLS
Wigo needs a key pair to enable HTTPS api.
You can generate a self signed key pair by running this command:
```
/usr/local/wigo/bin/generate_cert -ca=false -duration=87600h0m0s -host "hostnames,ips,..." --rsa-bits=4096
```

Wigo needs a CA key pair to secure PUSH communications.
You can generate a CA self signed key pair by running this command on the push server :
```
/usr/local/wigo/bin/generate_cert -ca=true -duration=87600h0m0s -host "hostnames,ips,..." --rsa-bits=4096
```


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

### Building from sources

##### Environment
To build from sources, you need `golang` and `gcc` installed.
The `$GOPATH` environment variable should also be set on your system.

##### Dependencies
```sh
make deps
```

##### Building for your system
```sh
make clean release
```

##### Building for all supported archs (amd64 & armhf)
You will need `arm-linux-gnueabihf-gcc`
```sh
make clean releases
```

##### Build debian packages
You will need `reprepro`
```sh
make debs
```

##### Build rpm packages
You will need rpm-building tools
```sh
cd build/rpm/
./makePackege.sh
```

### Usage

##### Wigo web interface

`http://[your-ip]:4000/` (by default)


##### Wigo CLI

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


##### Write your own probes !

Probes are binaries, written in any language you want, that output a json string with at least Status param :
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

##### Status codes :
```
    100         OK
    101 to 199  INFO
    200 to 299  WARN
    300 to 499  CRIT
    500+        ERROR
```

