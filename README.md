Wigo
=========

Wigo, aka "What Is Going On" is a light pull/push monitoring tool written in Golang.

Main features

  - Write probes in any language you want
  - Notifications when a probe status change (http,email)
  - Proxy mode for hosts behind NAT/Gateways



Version
----
0.41


Installation
--------------

On Debian Wheezy : 

##### Add repository and install package
```sh
echo "deb http://mir.root.gg wheezy main" > /etc/apt/sources.list.d/mir.root.gg.list
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
{
   "Detail" : "0.01 0.03 0.05 2/126 22686",
   "Value" : "0.01",
   "Version" : "0.10",
   "Message" : "",
   "Status" : 100,
   "Metrics" : {
      "load10" : 0.03,
      "load15" : 0.05,
      "load5" : 0.01
   }
}
```

The metrics are a hashmap[string] float, that will be pushed to OpenTSDB to graph your probe values (soon)



