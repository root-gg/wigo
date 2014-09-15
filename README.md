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
0.42


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
├── 10
├── 120
├── 300
└── 60
    ├── hardware_disks -> ../../probes-examples/hardware_disks
    ├── hardware_load_average -> ../../probes-examples/hardware_load_average
    └── hardware_memory -> ../../probes-examples/hardware_memory


```

##### Get status

```sh
telnet 0 4000
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




