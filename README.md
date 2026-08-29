# Wigo

**Wigo** (What Is Going On) is a lightweight pull/push monitoring tool written in Go.

## Features

- **Probes in any language** — Write probes as binaries in the language of your choice
- **Notifications** — HTTP, email, and Apprise alerts when probe or host status changes
- **Proxy mode** — Monitor hosts behind NAT/gateways via PUSH mode
- **Metrics** — Send probe metrics to OpenTSDB

## Screenshots

Taken against a fabricated fleet — every hostname, group and measurement below
is invented.

### Main view

Every group, and every host under it with its probes.

![Main view](docs/screenshots/main.png)

### Group view

One group, and every probe of every host in it with what it last said.

![Group view](docs/screenshots/group.png)

### Host view

One host: its labels, whether it is silenced, its status over time, and each
probe with its detail and its graphs.

![Host view](docs/screenshots/host.png)

### Wall

For a screen on a wall: worst first, and its own "updated Ns ago" turning amber
when the page stops being refreshed — a wall display that quietly froze is
worse than a blank one.

![Wall](docs/screenshots/wall.png)

---

## Monitoring modes: PULL and PUSH

Wigo can collect data from other Wigo instances in two ways.

### PULL mode

The **master** (central Wigo) periodically **fetches** data from remote Wigos over HTTP/HTTPS.

- You configure a list of remote hosts in `[RemoteWigos]` (see [Configuration](#configuration)).
- The master polls each remote at `CheckInterval` seconds by requesting `GET /api` (full JSON state).
- Remotes must expose the HTTP API (default port 4000). Use firewall rules, TLS, and optional HTTP Basic Auth to secure access.
- **Use case:** You can open the master’s network to outbound connections; remotes are just normal Wigo instances with the HTTP API enabled.

### PUSH mode

**Clients** (e.g. hosts behind NAT) **send** their state to a central Wigo over a persistent TCP connection.

- You enable `[PushServer]` on the central Wigo and `[PushClient]` on each client.
- Clients connect to the server (default port 4001), authenticate with a signed UUID, then periodically push their state via RPC (binary gob over TCP).
- TLS is strongly recommended. The server uses a **CA** certificate to sign client UUIDs; clients use the server certificate to verify the server.
- **Use case:** Hosts that cannot be reached from the master (NAT, no public IP). They initiate the connection and push their probes and status.

You can combine both: a master can have some remotes via PULL (HTTP) and others via PUSH (TCP).

---

## Status codes

Every probe returns a numeric **Status**. Wigo uses it for aggregation and notifications.

| Code      | Label  | Meaning |
|-----------|--------|---------|
| **100**   | OK     | All good |
| **101–199** | INFO  | Informational (e.g. minor notice) |
| **200–299** | WARN  | Warning, attention needed |
| **300–499** | CRIT  | Critical, should be fixed |
| **500+**  | ERROR  | Error or failure (e.g. probe timeout, exec failure) |

- **Host status** is the **maximum** of its probe statuses (worst probe wins).
- A host that has not reported in time is marked **DOWN** (status 999).
- **Notifications** are sent when a probe’s status **changes** and crosses the threshold defined by `MinLevelToSend` (e.g. only WARN and above). You can also get notified when a host goes UP or DOWN.

---

## Installation

Packages for **Debian 12 (Bookworm)** and **Debian 13 (Trixie)** are available
from the project repository. The line below picks the suite matching the
machine it runs on, so it only works on one of those two:

```sh
apt-get install lsb-release
sudo mkdir -p /etc/apt/keyrings
wget -qO- http://deb.carsso.com/deb.carsso.com.key | gpg --dearmor | sudo tee /etc/apt/keyrings/deb.carsso.com.gpg > /dev/null
echo "deb [signed-by=/etc/apt/keyrings/deb.carsso.com.gpg] http://deb.carsso.com $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/deb.carsso.com.list
apt-get update
apt-get install wigo
```

Alternatively, you can build the packages from source (see [Building from source](#building-from-source)) or download the latest release from the [releases page](https://github.com/root-gg/wigo/releases).

**Optional:** [Install the Wigo Monitoring browser extension](https://github.com/carsso/wigo-browser-extension) for Chrome or Firefox.

---

## Configuration

 - Main config file: **`/etc/wigo/wigo.conf`**.
 - Probes config: **`/etc/wigo/conf.d/`**.

The main config file is split into sections. Below is what each section does and the main options.

### `[Global]`

| Option | Description |
|--------|-------------|
| `Hostname` | This machine’s hostname (default: system hostname). |
| `Group` | Logical group (e.g. `webserver`, `loadbalancer`) for the UI and OpenTSDB tags. |
| `LogFile` | Log path (e.g. `/var/log/wigo.log`). |
| `ProbesDirectory` | Where probe executables live (e.g. `/usr/local/wigo/probes`). |
| `ProbesConfigDirectory` | Directory for per-probe config (e.g. `/etc/wigo/conf.d`). |
| `UuidFile` | Path where the instance UUID is stored. |
| `Database` | SQLite DB to store data (logs, status, etc.) (e.g. `/var/lib/wigo/wigo.db`). |
| `AliveTimeout` | Seconds without update before a remote is considered DOWN (e.g. `60`). |
| `Debug` | Enable debug logging. |

### `[Http]`

HTTP API (used by the web UI and by PULL mode).

| Option | Description |
|--------|-------------|
| `Enabled` | Turn the HTTP server on/off. |
| `Address`, `Port` | Listen address (e.g. `0.0.0.0`, `4000`). |
| `SslEnabled` | Use HTTPS. |
| `SslCert`, `SslKey` | Paths to the server certificate and private key. |
| `Login`, `Password` | Optional HTTP Basic Auth for the API. |
| `AllowWriteActions` | Allow the API to change this host (enable, disable and repitch its probes). **Default `false`** — anyone reaching the dashboard could otherwise switch monitoring off. |

### `[PushServer]`

Central server for PUSH mode (clients connect here).

| Option | Description |
|--------|-------------|
| `Enabled` | Enable the push server. |
| `Address`, `Port` | Listen address (e.g. `0.0.0.0`, `4001`). |
| `SslEnabled` | Use TLS (recommended). |
| `SslCert`, `SslKey` | **CA** certificate and private key (used to sign client UUIDs). |
| `AllowedClientsFile` | File listing allowed client UUIDs (one per line). |
| `AutoAcceptClients` | If true, new clients are accepted without manual approval. |

### `[PushClient]`

Client side for PUSH mode (this instance pushes to a central Wigo).

| Option | Description |
|--------|-------------|
| `Enabled` | Enable the push client. |
| `Address`, `Port` | Push server host and port. |
| `SslEnabled` | Use TLS (recommended). |
| `SslCert` | Path to the **server’s** certificate (e.g. `/var/lib/wigo/master.crt`) so the client can verify the server. |
| `UuidSig` | Path where the server’s signature of this client’s UUID is stored. |
| `PushInterval` | Seconds between state pushes (e.g. `10`). |
| `AllowRemoteControl` | Allow the push server to act on this client (enable, disable and repitch its probes). **Default `false`**, and separate from `[Http] AllowWriteActions` on purpose: opening the local API never hands the machine over to its master as well. |

### `[RemoteWigos]`

List of remotes for **PULL** mode. The master polls these via HTTP.

| Option | Description |
|--------|-------------|
| `CheckInterval` | Seconds between polls per remote (e.g. `10`). Do not set below about half of `AliveTimeout`. |
| `SslEnabled` | Use HTTPS when polling. |
| `Login`, `Password` | HTTP Basic Auth for remote APIs. |
| `List` | Simple list: `["host1", "host2:4000", ...]`. Port optional (default: master’s HTTP port). |

For per-host options (custom port, TLS, auth, depth), use the advanced form in the config file (see comments in `etc/wigo.conf`).

### `[OpenTSDB]`

Optional: send probe metrics to OpenTSDB.

| Option | Description |
|--------|-------------|
| `Enabled` | Enable OpenTSDB output. |
| `Address` | OpenTSDB host(s). |
| `MetricPrefix` | Prefix for metric names (e.g. `wigo`). |
| `Deduplication`, `BufferSize` | Tuning for metric submission. |

### `[Notifications]`

Alerts when probe or host status changes.

| Option | Description |
|--------|-------------|
| `MinLevelToSend` | Minimum status to trigger a notification (e.g. `250` = WARN and above). |
| `OnProbeChange` | Notify on probe status change. |
| `OnWigoChange` | Notify when a host goes UP/DOWN. |
| `HttpEnabled`, `HttpUrl` | HTTP POST callback with notification payload. |
| `EmailEnabled`, `EmailSmtpServer`, `EmailRecipients`, … | SMTP email alerts. |
| `AppriseEnabled`, `ApprisePath`, `AppriseUrls` | Alerts via [Apprise](https://github.com/caronc/apprise). `AppriseUrls` receives **all** notifications. |
| `[[Notifications.AppriseTargets]]` | Apprise urls restricted to some groups, hosts and/or labels (see below). |

#### Filtering Apprise notifications

`AppriseUrls` is the catch-all list: every notification is sent to it. To route
notifications to different urls depending on the host or the group, declare one
or more `AppriseTargets`:

```toml
[Notifications]
AppriseEnabled = 1
ApprisePath    = "/usr/local/bin/apprise"
AppriseUrls    = []                                 # catch-all, keep it empty to only use targets

# Only the "databases" group and the host db-master.domain.tld
[[Notifications.AppriseTargets]]
Name   = "dba team"
Urls   = ["mailto://user:pass@domain.tld"]
Groups = ["databases"]
Hosts  = ["db-master.domain.tld"]

# Two groups
[[Notifications.AppriseTargets]]
Name   = "web team"
Urls   = ["slack://token/#alerts"]
Groups = ["frontend", "backend"]

# Every production database, wherever it lives
[[Notifications.AppriseTargets]]
Name   = "dba on call"
Urls   = ["slack://token/#dba"]
Labels = ["env=prod,role=db"]
```

| Field | Description |
|-------|-------------|
| `Name` | Optional label, shown in the logs when a notification is sent. Defaults to the position of the target in the file (`#1`, `#2`, …). |
| `Urls` | Apprise urls notified when the target matches. |
| `Groups` | Groups to notify. Matches the `Group` of the host that raised the notification. |
| `Hosts` | Hostnames to notify. |
| `Labels` | Label selectors, each `env=prod` or `env=prod,role=db`. See [Labels](#labels). |

`Groups`, `Hosts` and `Labels` are OR'ed: a notification matches if its group is listed, **or** its host is listed, **or** it matches one of the selectors. Matching on groups and hosts is case insensitive and `"*"` matches everything. A target with none of the three never matches — use `AppriseUrls` to notify every host. Urls matched by several targets are deduplicated, so a notification is never sent twice to the same url.

**`Labels` is what `Groups` cannot express.** A host has one group, so a target interested in every database in par1 has to name them one by one and remember to come back when one is added. Within one selector every label must match; across selectors any one is enough — "the prod databases" is one entry, "prod or db" is two.

The host's labels are looked up when the notification is sent, not carried on it: labels follow the host's config, and a copy taken when the problem started would route the recovery by what was true an hour ago.

A selector that cannot be read is **named at startup and routes nothing**, rather than quietly widening the target to everything:

```
Apprise target "dba on call" has an unusable label selector "nonsense" and will
not route on it : label selector "nonsense" is not key=value
```

Because of the TOML syntax, `[[Notifications.AppriseTargets]]` blocks must be
written **after** the other `[Notifications]` keys.

---

## TLS

### HTTPS API (PULL and web UI)

Use a **server** certificate and key so the HTTP API is served over HTTPS.

Generate a self-signed certificate (replace hostnames/IPs as needed):

```sh
/usr/local/wigo/bin/generate_cert -ca=false -duration=876000h0m0s -host "hostname,ip1,ip2" --rsa-bits=4096
```

Then set in `[Http]`: `SslEnabled = true`, and point `SslCert` / `SslKey` to the generated files.

You may want to order a certificate from a trusted CA like [Let's Encrypt](https://letsencrypt.org/) instead of using a self-signed certificate.

### PUSH mode (server and clients)

The **push server** uses a **CA** certificate to sign client UUIDs. Clients use the **server’s** certificate to verify the server.

On the **push server**, generate a CA key pair:

```sh
/usr/local/wigo/bin/generate_cert -ca=true -duration=876000h0m0s -host "hostnames,ips,..." --rsa-bits=4096
```

Configure `[PushServer]` with these cert/key paths. Copy the **certificate** (not the key) to each client and set `[PushClient].SslCert` to that path. On first connect, the client can optionally fetch the server cert via RPC and then reconnect with verification enabled.

---

## Probes

Probes live under `ProbesDirectory` (e.g. `/usr/local/wigo/probes`).

Subdirectory name is the check interval in **seconds** (60, 120, 300) (e.g. `/usr/local/wigo/probes/60`). They are executed automatically every time the interval is reached.

Probes in subdirectories are symlinks to the actual probe executable located in the examples subdirectory (e.g. `/usr/local/wigo/probes/60/check_mdadm` -> `/usr/local/wigo/probes/examples/check_mdadm`).

`examples/` is not a check interval and is never executed: it holds the probe executables themselves, and the interval directories link to them.

**A probe no interval directory links to is disabled.** Being disabled is not a place a probe is put, it is the absence of any schedule, so there is no directory of disabled probes. Wigo ships around thirty probes and the package enables half of them, which means most disabled probes were never turned off by anyone — they were never turned on. Either way the check is not happening, which is what matters.

**This directory is the source of truth** for which probes run and how often. Changing a probe's interval means moving its symlink to another interval directory, creating it if needed — `probes/900/` is as valid as `probes/60/`. Nothing else records that state, so a database that disagreed with the directory can never silently stop the monitoring.

**A probe is scheduled once, and the smallest interval wins.** Linked into two interval directories, it used to be *run* twice: one scheduler per directory, at two different rates, each overwriting the other's result — and, for the probes that keep state between runs in `/tmp/<probe>.wigo`, each corrupting the deltas the other computes. Now exactly one directory owns it.

The smallest is the one that runs because between the two possible mistakes there is no contest: running something more often than someone asked costs a little CPU, running it less often than they asked is a gap in the monitoring that nobody sees.

The extra links are **not deleted** — links somebody put there by hand are theirs — but they do nothing, and a link that does nothing is exactly the state somebody comes back to in six months wondering why the interval is not what the directory says. So each one is named once at startup:

```
Probe check_ntp is linked in 60, 300. Only 60 runs it, the smallest interval wins ;
the other links do nothing and can be removed.
```

The Debian package only seeds the default symlinks on a **fresh install**, so upgrading never re-enables a probe you disabled nor recreates one you moved.

Probes config files are located in `ProbesConfigDirectory` (e.g. `/etc/wigo/conf.d`).

**A config file is an opinion about some settings, not a replacement for all of them.** It is merged over the probe's defaults, so a file naming one key keeps every other default. It used to replace them outright, which meant adding a single line silently dropped the rest and left the probe comparing values against nothing.

#### check_ssl_cert

A certificate expires on a date known months in advance and takes the site down anyway. There is nothing to detect and nothing to react to: the only useful thing a monitoring tool can do is count the days out loud, early enough that renewing is boring.

```json
{
  "enabled"  : true,
  "hosts"    : ["example.com", "api.example.com:8443", "10.0.0.5:443:api.example.com"],
  "warning"  : 30,
  "critical" : 7,
  "timeout"  : 5
}
```

Each entry is `host`, `host:port`, or `host:port:servername` when the certificate to check is not the one served for the name you connect to. Days remaining are exposed as a metric per host, so the graph shows the sawtooth of renewals and a flat line that should have been one.

Certificates are read, not validated: an expired or self-signed one is exactly what has to be **reported** rather than refused. A host that cannot be reached is reported as the probe failing, not as a certificate being fine — not being able to look is not the same as having looked.

With no hosts configured it stays OK and says so. A machine that serves no TLS has nothing to say here, and saying it in red would be one more red thing nobody can act on.

#### check_systemd

A failed unit is already known: systemd noticed, wrote it down, and stopped trying. What is missing is somebody being told.

```json
{
  "enabled"         : true,
  "ignore"          : ["some-unit-that-fails-on-purpose.service"],
  "status"          : 300,
  "check_not_found" : true
}
```

`ignore` takes exact names rather than patterns — a pattern that quietly grows to cover a unit somebody cared about is how this kind of list stops being trustworthy. Units systemd could not even load are counted separately from units that ran and failed, because they are a different problem: a typo in a name, a dropped file.

#### check_dns

When name resolution stops, everything on the machine breaks at once and nothing says why: connections do not fail, they hang, and the failure surfaces as every *other* service being slow.

```json
{
  "enabled"     : true,
  "nameservers" : [],
  "queries"     : ["example.com", "mail.example.com:MX", "www.example.com:A:203.0.113.10"],
  "warning"     : 500,
  "critical"    : 2000,
  "timeout"     : 3,
  "retry"       : 1
}
```

`nameservers` empty means **the machine's own**, from `resolv.conf` — the one that matters. A probe pointed at a public resolver tells you that resolver is up, not that this host can resolve anything.

Each query is `name`, `name:type`, or `name:type:expected,expected`. Without an expected value the check is that an answer comes back at all, which is deliberately the common case: an A record that legitimately changes should not turn a screen red. Response time is a metric per query, in milliseconds, so slow resolution is visible before it becomes total.

The answer section is filtered to the type actually asked for. A reply routinely carries the CNAME chain that led to the records, and counting those as answers would let a name that resolves to nothing look answered.

**NXDOMAIN, a wrong answer and no answer at all are all CRITICAL, and say which.** A resolver that does not answer is the finding this probe exists to make, not a failure to observe one — unlike `check_ssl_cert`, where being unable to look says nothing about the date. A confident answer pointing at the wrong place is worse still.

Ships with no queries configured, and stays OK saying so.

#### check_docker

Containers fail quietly. A crash loop looks like a running container from the outside — it *is* up, briefly, over and over — and a failing health check is recorded by the engine and told to nobody.

```json
{
  "enabled"  : true,
  "command"  : "docker",
  "expected" : ["postgres", "redis"],
  "ignore"   : ["some-batch-job"],
  "status"   : 300,
  "timeout"  : 10
}
```

**Works against podman**: set `command` to `podman`, the output asked for is the same. One probe rather than two nearly identical ones, so the podman path is the same tested code.

With `expected` empty, only the unambiguous failures are reported: **restarting** and **unhealthy**. A container that is merely stopped may well have been stopped on purpose and the probe cannot know — so a stopped container counts only when its name is in `expected`, where a name that is *absent* entirely is reported too.

`ignore` means not spoken about at all, counts included. Saying "9 containers, 6 running" of a machine where three were deliberately silenced invites the reader to work out that three are down.

Being unable to ask the engine is **500**, not a container problem: it says nothing about the containers. The exit code is checked before the output is parsed — `sh: 1: podman: not found` parses perfectly well as a container called `sh:`.

#### check_backup_age

A backup that stopped running leaves yesterday's file sitting there, and everything looks exactly as it did when it worked. Nobody finds out until the day the backup is needed.

```json
{
  "enabled"  : true,
  "paths"    : ["/var/backups/db:26:1048576", "/srv/dumps", "/var/backups/etc.tar.gz"],
  "warning"  : 26,
  "critical" : 50,
  "minsize"  : 0
}
```

Each entry is `path`, `path:hours`, or `path:hours:minbytes`; a per-entry `hours` sets critical at twice warning. The path is a directory — the newest file directly inside it, which is the shape a rotating backup takes — or a single file, for a job that always writes the same name.

**Two silent failures, not one.** The job that no longer runs, and the job that still runs and writes nothing: a dump to a full disk, or with an expired credential, ends up as a zero-byte file with today's date on it. `minsize` catches the second, and is reported whatever the age says.

Subdirectories are not considered when looking for the newest file. A directory's own mtime changes when anything inside it is touched, so counting it would report a backup as fresh because something unrelated was written next to it.

A path that does not exist is reported as **no backup**, not as an old one — it is usually a mount point that did not come back.

The default of 26 hours rather than 24 is deliberate: a daily job takes time to run, and a threshold at exactly the period goes red every night for as long as the backup takes.



### How long a probe gets to answer

A probe gets its **interval minus one second**, which is what every probe has always got. That is a ceiling, not a sensible wait. A probe wedged on an unreachable server holds on for the whole interval, and the dashboard shows the result from *before* the outage that entire time — a check pitched at five minutes sits on a dead socket for four minutes fifty-nine, when the useful answer, *it did not answer*, was there after ten seconds.

Name a probe in `[ProbeTimeouts]` to shorten that:

```toml
[ProbeTimeouts]
check_ssl_cert            = 15
check_dns                 = 5
```

**It only ever shortens.** Asking for longer than the interval is refused rather than obeyed, and said so at startup:

```
Config : probe timeout for check_slow is 120s but it runs every 60s : ignored, since a run
outliving its interval would overlap the next one. Pitch it at a longer interval instead
```

The scheduler fires a run every interval regardless of whether the last one finished. A run allowed to outlive its interval would overlap the next one: two copies of the same probe talking to the same thing at once, and whichever finished last winning. A probe that genuinely needs more time needs a longer interval, which is a different thing to change — so the operator is told, not quietly given something other than what they asked for.

Everything that will be ignored is named at startup, in a stable order, including a timeout for a probe that does not exist and one for a probe nothing schedules. Those two get different advice, since a name matching no probe is usually a typo while an unscheduled probe is installed and merely turned off.

A recheck asked for by hand takes whichever is shorter, the configured timeout or the 30-second cap: a probe given ten seconds by the config is not handed thirty because somebody clicked, and one that would take an hour on its own schedule does not hold an HTTP request open for it.

### Managing probes through the API

| Endpoint | Description |
|---|---|
| `GET /api/probes` | Every probe of this host with its interval, including the disabled ones. Answers `{ "Hostname", "WriteActionsAllowed", "Probes", "SkippedProbes" }`, so a client knows which host it is looking at, whether changing it would be refused, and which probes asked not to be run again. |
| `POST /api/probes/:probe/disable` | Remove every schedule of the probe. Already disabled is not an error. Takes `?reason=` and `?for=` (a duration, `1h` / `24h` / `168h`). |
| `POST /api/probes/:probe/interval?seconds=300` | Run the probe every 300 seconds, enabling it if it was disabled. |
| `POST /api/probes/:probe/run` | Run the probe now, out of band, and answer its fresh result. |

The two `POST` endpoints return **403** unless `AllowWriteActions` is set in the `[Http]` section. They act on the probes directory directly, so the change takes effect on the next cycle without a restart, and it survives one.

**A host that pushes is governed by `AllowRemoteControl`, not by that.** Three different things can make a host read only from here, and each is fixed in a different file:

| what refuses | where to fix it |
|---|---|
| the caller's role | sign in, or present an operator token |
| this host's own writes | `AllowWriteActions` in `[Http]`, on that host |
| a pushing client that never opted in | `AllowRemoteControl` in `[PushClient]`, on the client |

The API says which, in `ReadOnlyReason` on the schedule, because only it can tell them apart — the interface sees one boolean. It used to print "set AllowWriteActions in `[Http]`" for all three, which sent anyone with a push client editing the wrong file on the wrong machine.

A client reports `AllowRemoteControl` on **every** push, so opening it takes effect on the next one — ten seconds by default, with nothing to do on the master.

A probe must be installed to be acted upon: a name that exists nowhere under `probes/` is refused.

**Disabling never destroys anything.** A schedule is usually a symlink into `examples/`, where the probe itself stays, so the symlink is simply removed. But an administrator may have dropped a script straight into an interval directory, or linked to one outside the probes tree — deleting that would be the only copy gone, and the probe would not even be listed any more, so there would be no way to turn it back on. In that case the entry is moved into `examples/` instead. Either way the probe ends up installed and unscheduled, which is what disabled means.

A probe scheduled from **several** directories at once runs several times per cycle. Asking for an interval means asking for it to run every so often — once — so the extra symlinks are removed and the probe ends up scheduled exactly once. Each removal is logged with the symlink it pointed at. Moving one and leaving the others would keep the probe running from them, which is the very state being corrected.

### Acknowledging and silencing

Stopping the notifications about something without stopping to watch it. Not the same as disabling a probe: a suppressed check keeps running, keeps being displayed and keeps its history — only the message stops. Reaching for disable when you meant "stop telling me for two hours" is how a fleet ends up with blind spots nobody remembers creating.

| Endpoint | Description |
|---|---|
| `GET /api/suppressions` | What is currently held back, and whether this host allows changing it. |
| `POST /api/hosts/:h/ack` | Acknowledge the current state of a host, or of one probe with `?probe=`. |
| `POST /api/hosts/:h/silence?for=2h` | Hold back notifications about a host, or one probe, for a while. |
| `POST /api/hosts/:h/unsuppress` | Notify again. |
| `POST /api/groups/:g/silence?for=2h` | The same for a whole group. |
| `POST /api/groups/:g/unsuppress` | |

All of them take `?reason=`, and the `POST` endpoints return **403** unless `AllowWriteActions` is set.

**An ack says "I know, I am on it".** It has no end date, because nobody knows when the fix will land. It records the status it was taken at, and **anything worse gets through**: acknowledging a WARNING must not swallow its turn to CRITICAL, since that is not what anyone acknowledged. It clears itself when the thing recovers, and when it gets worse — leaving one behind would silently hold back the next problem.

**A silence is a window**, and it must have one: `?for=` is required, between a minute and a year. A silence with no end is an unmonitored host with extra steps.

`GET /api/suppressions` and the **Held back** page list everything currently quiet, with a button to lift each one. That page is the point: a silence set on a group has no card to go back to, and a suppression nobody can find is the same blind spot a disabled probe is. It lists flapping probes too — nobody decided those and there is nothing to lift, but the effect is identical and it belongs on the same page.

### Labels

A host has exactly one `Group`, and a group has to be *chosen*: `par1`, or `prod`, or `db` — never the three at once. Whichever you pick, the other two questions stop being answerable. "Every database" and "everything in par1" are both reasonable things to ask of a monitoring tool, and one `Group` can only answer one of them.

```toml
[Labels]
env  = "prod"
role = "db"
dc   = "par1"
```

**Labels are additive, not a replacement.** `Group` is untouched, still published, still what the group views and the API paths are built on — and it travels as the label `group` as well. So filtering by label reaches *every* host, including the ones running a version that has never heard of labels, which is the whole fleet on the day you upgrade the first one.

| | |
|---|---|
| `GET /api/hosts?labels=env=prod,role=db` | the hosts carrying **all** of them |
| `GET /api/labels` | every label in use, with how many hosts carry each value |

Without the parameter `/api/hosts` answers exactly what it always did. An empty selector matches everything: a filter nobody filled in must not empty the screen.

A selector asking for the same key twice — `env=prod,env=test` — is refused with **400** rather than answered with an empty list. It was probably meant as "either", and nothing can carry both; an empty list reads as *the hosts are gone*, which is the one answer a monitoring tool must not give by accident.

Keys and values take letters, digits, dot, dash and underscore, starting with a letter or a digit — they end up in URLs and log lines, and a value containing a comma or an equals sign would silently split into something else. Anything refused is **named at startup and dropped**, and what a host publishes is already the effective set: the API must not answer that a host carries a label the log says is ignored.

`group` cannot be set in `[Labels]`. It is derived from `Group`, and two places saying which group a host is in is one too many — the config file and the API would disagree the day they differ.

**On screen**, a host's labels sit at the top of its page, before what is happening to it. Each one is a link to the fleet filtered on it: the gesture you want after reading `env=prod` is to see the rest of production. The group is among them, so even a host that has never been labelled has one useful link.

The filter lives in the URL (`/#/?labels=env=prod`) like the level and search filters, so a filtered view can be shared and survives a reload — but unlike the levels it is **not** remembered across sessions. "Only show me production" is a question of the moment, not a preference, and finding it silently reapplied the next day would look like half the fleet had vanished.

Which hosts carry a selector is decided by the server, through `/api/hosts?labels=`, rather than in the browser: the group summaries do not carry each host's labels, and one definition of *carries this label* serving the filter, the API and the notification routing is worth a round trip.

A selector the server refuses is shown with its reason, and **the fleet is still shown in full** rather than emptied — an empty page would say the hosts are gone, which is not what happened.

### Host dependencies

A router goes down and the forty hosts behind it stop answering. Each one is a separate discovery and a separate message, and **none of the forty is the news**: the router is.

```toml
[[Dependencies]]
Group     = "par1"
DependsOn = ["router-par1"]

[[Dependencies]]
Host      = "web3.example.com"
DependsOn = ["sw-2"]
```

While a parent is not answering, nothing is said about what sits behind it. Nothing is said when it comes back either — nobody was told it had broken, and forty "back to normal" messages about problems nobody heard of are the same storm arriving from the other side.

**Only direct parents are looked at.** That removes cycles by construction and loses nothing: a host behind a switch behind a router still works, because a router that is down takes the switch with it, and the host is then held by its own direct parent. Reachability is transitive on its own and does not need walking.

**A parent this wigo has never heard of counts as up**, so the message goes out. Of the two directions that is the safe one: a typo then costs noise, where the other way round it would silently cost the alert. Rules that can never do anything — naming neither a `Host` nor a `Group`, or naming both, or depending on nothing — are reported at startup, because doing nothing at all is exactly what a working dependency looks like.

The status is unchanged: those hosts are still shown as down, still logged, still on their timeline. Only the interruption is held, and the **Held back** page lists what is currently quiet this way.

A group cannot be acknowledged — forty hosts have no single status to say "I am on it" about. Silencing one is fine, since that claims nothing about state, and the group page carries the control. It targets the **label**, so a host that joins the group during the window is covered too.

The most specific suppression wins: one on a probe beats one on its host, which beats one on its group.

These are decided **where the notifications are sent**, which on a fleet is the master, so they are never forwarded to the host being silenced — that host does not send the messages being stopped.

### Metric history

A probe reports a load average, a disk usage, a queue length, and until now all of it was thrown away the moment the next run replaced it — unless an OpenTSDB was configured. Which meant the answer to *was it already climbing an hour ago* was no, and getting one meant deploying a time series database next to a monitoring tool that already has one open.

It is now written to the SQLite that is already there, bounded by `MetricsRetentionDays` (7 by default, `0` to keep none). A week of a machine running twenty probes is around **35 MB**, and reading that whole week back takes about 30 ms.

| Endpoint | |
|---|---|
| `GET /api/probes/:probe/metrics?since=&until=&points=` | The history of one probe of this host. |
| `GET /api/hosts/:h/probes/:probe/metrics` | The same for any polled host of the tree. |

Points are **bucketed**: a week at one point a minute is ten thousand points per series, which no browser should be asked to draw. Each bucket carries its average *and* the range it covers, so the spike that woke somebody up is still visible after being averaged.

**A wigo keeps its own history**, and a master reads a *polled* remote's through that remote's API, the same way it reads its schedule. Storing the whole fleet's series on the master would write everything twice and make its database grow with the size of the fleet, which is the thing that pushes people towards a separate stack.

**With one exception**, and it is not a softening of that rule but the only place it cannot hold: a host that **pushes** cannot be asked anything — it sits behind a NAT. Its measurements arrive with every push and used to be dropped, so those hosts had no graphs at all and the screen answered a 501 explaining why. They are kept now, under the name of the host they came from. The growth is bounded by the number of pushing clients rather than by the fleet, and those clients are precisely the ones with no other way of being read.

A client pushes far more often than its probes run — every ten seconds against every minute is a normal pairing — and every push carries the same result again. Only a measurement newer than the last one seen is written, so one probe run is one row rather than six. Without that, the master's database would grow at the push rate for a probe that answered once.

Nothing about the monitoring depends on this table: losing it loses history and nothing else.

### Graphs

Each probe card carries a chart of what that probe measured, over 1 hour to 7 days. Drawn as plain SVG — no charting library, for the same reason the Prometheus format is written by hand: it would be the largest dependency in the tree, for a line and some axes.

Two decisions are worth knowing about:

**Colour follows the series, never its rank.** Hiding a line from the legend does not repaint the others — a reader who learned "load5 is green" is not lied to a moment later. The eight hues are used in a fixed, validated order and a ninth series is never given a generated colour: past eight, another hue is indistinguishable from one already on screen under colour blindness, so the extra series are named and left out rather than drawn misleadingly.

**The average is not the whole story.** With one or two series the chart shades the min–max band behind the line, and the tooltip always carries the range, so a spike that a bucket averaged away is still visible. A table view is one click away for anything the colours alone cannot carry.

Dark mode uses the same eight hues re-stepped for the dark surface, not an automatic flip. Both were validated against wigo's real card backgrounds.

### Status timeline

Under each probe, and under the host itself, a band of when it was fine and when it was not, over 6 hours to 30 days.

A status is a **state that lasts**, so it is drawn as a band rather than a line: the question a timeline answers is not *what was the value* but *how long did it last*, and the tooltip gives the message that came with it.

The transitions are recorded as what they are — two statuses and a time — in a `status_changes` table bounded by `StatusHistoryDays` (30 by default, `0` to keep none). The `logs` table already carries the sentence "Probe check_load switched from 100 to 300 : load too high", and the timeline could have been built by parsing it; it would also have broken, silently, the day somebody reworded that sentence.

**Unlike the metrics, this is recorded for remotes too.** A status change is not a sample — a handful a day, not one a minute — so there is nothing to save by not storing them, and a master sees things a host cannot record about itself, going down being the obvious one. The endpoint is therefore never forwarded: `GET /api/hosts/:h/timeline?probe=&since=&until=` answers what *this* wigo observed.

**Never watched is not the same as fine.** A probe installed yesterday has no history for the 29 days before it, and that part of the band is hatched rather than green — and the summary line says how much of the window is unknown instead of claiming everything was fine.

### Probe detail

What a probe returned in `Detail` used to be dumped as raw JSON. `iostat` on a machine with twenty-three block devices is two hundred and fifty lines of braces that nobody reads.

It is now rendered as a table when the shape allows one — a flat object becomes name/value rows, and an object of objects (a disk per key, a device, a service) becomes one row each with the shared keys as columns, which is the shape that actually turns up. Measurements are monospaced and never wrapped, so a column of `0.00 kB/s` lines up and can be compared; text still wraps.

**The threshold is deliberately blunt: either every cell is a scalar or a list of scalars, or the whole thing stays JSON.** A table with half its cells holding a JSON blob is harder to read than the JSON was, and it quietly suggests you have seen everything.

The raw JSON is one click away regardless, next to a copy button — that is what goes into a ticket.

### Reachable without a mouse

**Navigation is links.** Every tile on the wall, every host name on the *Held back* and *Disabled probes* pages used to be an `<a>` with a click handler and no `href`. That is not a link: it is out of the tab order, it does nothing on Enter, a screen reader announces it as text, and it cannot be opened in a new tab. They are `router-link` now, which renders a real anchor and gets all of that from the browser.

**In-page actions are buttons.** The four sidebar entries that scroll to an anchor are not navigation, so they stay where they are and take a button's behaviour instead: `role="button"`, `tabindex="0"`, and Enter and Space activating them.

**Status badges let Bootstrap pick the text colour.** `.badge` sets it to white and `.bg-*` only sets a background, so the WARNING badge was white on Bootstrap's yellow — **1.63:1**, where 4.5:1 is the floor for text that size. Using `text-bg-*` hands the choice back to Bootstrap, which computed it:

| | before | after |
|---|---|---|
| WARNING | 1.63 | **12.88** |
| INFO | 4.50 | **10.72** |
| OK, CRITICAL, ERROR, DISABLED | 4.53 – 15.43 | unchanged |

INFO becomes cyan in the process, which is what the counter in the top bar has always been: one level, one hue, everywhere.

**On a phone the sidebar is a drawer.** Below `md` it slides over the content instead of squeezing it, and the collapsed state gets out of the way entirely: what it left behind was a 56-pixel rail of identical chart icons, which is width spent to say nothing. Choosing an entry closes it, and so does touching the veil beside it — a drawer that stays open leaves you standing behind the thing you just asked for.

One trap, since the fix is not the obvious one. The close-on-choose check compared `window.innerWidth` to 768 and never fired: a `position: sticky` table header inside a scrolling probe detail inflates `innerWidth` to **881 on a 390-pixel screen**, so the page believed it was on a desktop. It asks `matchMedia("(max-width: 767.98px)")` now, which is the same question the stylesheet asks — one breakpoint, one answer, no way for the two to drift apart.

### The wall

`/wall` is a dense grid of every host, one tile each, for a screen nobody sits in front of. No sidebar, no filters, no chrome: colour, hostname, and what is wrong.

**The worst first.** On a wall the eye starts top left, so that is where what is broken has to be. Sorted on the numeric status rather than the level name, because between two criticals 400 is worse than 300 and the alphabet does not say so.

**It shows its own liveness.** A frozen page and a healthy fleet look exactly the same — both green, both still. So the age of the last refresh is in the header, ageing every second, and it turns amber once nothing has arrived for three minutes. Without it a dead screen reads as *everything is fine*, which is the worst thing a monitoring screen can say.

**A failed request keeps the previous tiles.** Blanking the grid would not say *the request failed*, it would say *there is nothing*, and that is not the same thing.

Tiles fill the height of the screen and shrink past sixty hosts. The group is on every tile, because short hostnames repeat across groups and `srv1` alone is not an answer.

### Live updates

`GET /api/events` streams what happens as server sent events, so a probe going critical shows up in the interface at once rather than at the next poll — up to a minute of looking at a green screen about a machine that is already down.

What travels is deliberately thin: *what* changed, not what it changed to. The browser refetches, which keeps one serialisation of the state instead of two and means a missed event costs nothing. A subscriber that cannot keep up has its events dropped rather than slowing the scheduler down, for the same reason.

**The periodic refresh is not replaced.** A stream can die quietly — a proxy, a laptop waking up — and a page that has stopped refreshing while looking up to date is worse than no stream at all. The stream adds immediacy; the poll stays the safety net. A keepalive every 25 seconds keeps idle proxies from cutting the connection.

`wigo_event_subscribers` is exposed to Prometheus: a number that only ever grows is a leak.

### Caching the interface

`http.FileServer` sends `Last-Modified` and no `Cache-Control`, which leaves the browser to invent a freshness lifetime of its own. That is the worst of both worlds: `index.html` gets held on to, and the bundle it names is deleted by the next build — Vite empties the output directory — so an upgraded wigo serves a blank page to anyone who has not pressed reload twice.

The two are now told apart:

| | |
|---|---|
| `/` | `Cache-Control: no-cache` — revalidated every time. |
| `/assets/*` | `Cache-Control: public, max-age=31536000, immutable` |

`no-cache` asks, it does not refetch: the answer to an unchanged index is a **304 the size of a header**, which is why it is not `no-store`. Everything under `assets/` is named after a hash of its own content, so it can never change under that name and never needs asking about again.

### The API, written down

`GET /api/openapi.yaml` is an OpenAPI 3.1 document describing every path this wigo answers — served as well as kept in the tree, so whatever reads it can ask *this* wigo what it answers rather than guess from a version number.

**It is compared to the code, not trusted.** A specification nobody checks is a specification that lies within a month, and quietly, which is the only way one hurts. Two tests run on every build:

- every registered route is documented, and every documented path exists — both directions, because an undocumented route leaves a caller guessing and a documented route that answers 404 is worse, since they trusted it;
- every `$ref` points at a component that exists. That one caught six references to a `Logs` schema that was never written: the paths all lined up, and a generator reading it would have produced nothing.

The reader behind those tests is forty lines rather than a YAML dependency. What it needs to know is the paths, the verbs under each, and the references — pulling in a parser to learn that would be a dependency larger than the document it checks.

The big tree under `/api` is deliberately **not** written out field by field. It is the internal state serialised, it is large, and a specification that copies a struct is a specification that disagrees with it.

### Authentication and roles

A single shared basic auth credential was tolerable while everything was read only. It stopped being tolerable the moment the API could disable a probe, silence a host or acknowledge an alert: anyone handed the dashboard URL to look at a graph could switch the monitoring off for the whole fleet.

A caller now has a **role**. Reading needs the credential; changing anything needs an operator.

| Endpoint | |
|---|---|
| `GET /api/whoami` | What the caller may do. The interface uses it to avoid offering a control that would answer 403. |
| `GET /api/tokens` | The tokens, never their secrets. |
| `POST /api/tokens?name=&role=&for=` | Mint one. `role` is `readonly` or `operator`, `for` is a duration or empty for no expiry. |
| `POST /api/tokens/:id/revoke` | |

Present a token as `Authorization: Bearer wigo_…` or `X-Wigo-Token: wigo_…`. Never in a query string: that ends up in access logs.

**The first token is minted on the machine**, not through the API — every other way of minting one goes through the API, which needs a credential, which is what a first token is for. That circle is broken in the only place that owes nobody an authentication: whoever can read wigo's database can already read every secret it holds.

```sh
wigo token create scraper --role=readonly
wigo token create oncall  --role=operator --for=720h
wigo token list
wigo token revoke 3
```

It opens the database and nothing else — not the probes directory, not the log file, not the push server, none of which has anything to do with minting a token and any of which failing would be a reason not to be able to. The table is created if missing, so it works on a wigo that has never been started, and on one that is running right now: a token is read from the database on each request, so a new one works immediately.

**The secret is readable exactly once**, in the answer that created it. Only its SHA-256 is stored, so a stolen database cannot be replayed against the API. A token is 32 random bytes rather than a password, which is why a fast hash is the right one — guessing 256 bits is the problem, not how quickly you can try.

**The shared credential stays, and stays an operator.** An upgrade must not lock an administrator out of their own install: it is what you use to mint the first token, and what you remove once you have. A wigo with no `Login` set stays open, as it was.

**What an anonymous caller may do is its own setting.** It used to be a side effect of whether `Login` was filled in: no `Login` meant everybody could do everything, a `Login` meant nobody could do anything without it, and there was no way to say *anyone may look, only I may act*. `AnonymousRole` in `[Http]` says it directly:

| | |
|---|---|
| `""` | Whatever this install already did — everything with no `Login`, nothing with one. The default, so an upgrade changes nothing. |
| `"none"` | Refused. The credential or a token is required to see anything. |
| `"readonly"` | Anyone who can reach it may look; only the `Login` or an operator token may change anything. |
| `"operator"` | Anyone who can reach it may do whatever `AllowWriteActions` permits. |

An unknown value resolves to `readonly`, not to `operator`: a typo in the setting meant to lock a wigo down must not be what opens it, and being locked out too far is noticed immediately while being open too wide is not.

Presenting a wrong password is refused even where an anonymous caller would have been welcome — serving the typo as anonymous would hide it behind a dashboard that half works. Credentials sent to a wigo that has none configured are ignored rather than refused.

**Signing in from the interface.** A wigo that lets anonymous callers read never sends a 401, so a browser is never challenged and the credential exists without being reachable. `GET /api/login` is the only thing that provokes that challenge on purpose; the top bar links to it and sends you back to the page you were on afterwards.

It is a **navigation, not a request**. A browser shows its credential prompt for a top level navigation reliably; for an XHR it depends on the browser and the version, which is not something to build a sign-in on. `?next=` is checked to be a path on this host — an open redirect on the page somebody just typed a password into is the one place it really matters.

The button appears only when there is something behind it: a `Login` has to be configured, and the caller must not already be an operator. Offering it on a host with no credential would be a door with no key.

`GET /api/logout` answers 401 forever. There is no way to tell a browser to drop a basic auth credential; making it ask again, and cancelling that prompt, is what clears it. The interface says so rather than pretending otherwise.

A token that is presented and refused does **not** fall back on the shared credential — a revoked token would otherwise still work for anyone who also knows the password. Revoking keeps the row: what it was called and when it was turned off is a question somebody asks later. `LastUsed` is recorded so the tokens nobody uses can be found and revoked.

Both gates apply to writes, and both must say yes: the host has to accept being changed (`AllowWriteActions`), and the caller has to be an operator. Forwarding to a remote is checked the same way, so reaching through this host is not a way around the check that just failed.

**Admitting a push client needs an operator**, and nothing else. These two routes predate `AllowWriteActions` and had no check at all, which was harmless while the only way in was the shared credential and stopped being harmless the moment anonymous callers could read: anyone able to reach the dashboard could let an unknown machine push into the fleet, or expel one that was being watched.

The role on its own, deliberately **not** `AllowWriteActions` — that flag is about letting the API change *this host's own probes*, and requiring it would stop every existing push master, since it is off by default, from doing what it has always done. The role check changes nothing for any configuration that existed before.

### The HTTP layer

Everything is served by `net/http` alone. Handlers return a status and a body rather than writing to the response themselves, which keeps a handler a plain function of its request — testable without a server, and the reason every refusal in the API is a sentence rather than a bare status code.

Requests pass through, outermost first: panic recovery, request logging when `Debug` is on, security headers, gzip when `Gzip` is on, and HTTP basic auth when `Login` is set. The credential is compared in constant time, and it guards the interface, the API and `/metrics` alike.

### Docker

```
docker run -d -p 4000:4000 \
  -v wigo-probes:/usr/local/wigo/probes \
  -v wigo-data:/var/lib/wigo \
  ghcr.io/root-gg/wigo:latest
```

Published for `linux/amd64` and `linux/arm64`. `build/docker/docker-compose.yml` starts a master watching two clients, which is the smallest thing that shows what wigo is for: one interface, one `/metrics`, several machines.

The image is **not** built on scratch or alpine even though the binary is static: the probes shipped with wigo are Perl scripts, so a distroless image would start, answer, and monitor absolutely nothing. Debian slim plus the Perl modules those probes actually use is the smallest base on which `docker run wigo` does what the name promises.

**Mount the probes directory.** It is the source of truth for which probes run and how often, so without a volume every probe an operator enabled, disabled or repitched comes back to the defaults on the next start. The defaults are seeded on first start only, marked by `probes/.seeded` — the same rule as the Debian `postinst`, and for the same reason.

Debian packages are built for `amd64`, `armhf` and `arm64`.

### Prometheus

`GET /metrics` exposes the **whole tree** in the Prometheus exposition format. A master already gathers its remotes, so scraping the master gets the fleet and scraping a standalone wigo gets itself — there is no separate exporter to deploy and nothing to keep in step with the topology.

| Metric | |
|---|---|
| `wigo_up` | 1 when scraped. |
| `wigo_host_up{host,group}` | Whether the host is reachable and reporting. A polled remote goes to 0 after `AliveTimeout`. |
| `wigo_host_status{host,group}` | Worst status of its probes. |
| `wigo_probe_status{host,group,probe}` | 100 is OK, higher is worse, below 100 is an error. The raw value, so a rule can draw its own line. |
| `wigo_probe_last_run_timestamp_seconds{...}` | A value that stops moving is a probe that stopped running. |
| `wigo_probe_interval_seconds{...}` | |
| `wigo_probe_metric{host,group,probe,tag_*}` | **What the probe itself measured.** Its own tags become `tag_` labels. |
| `wigo_probe_flapping{...}` | 1 when a probe's notifications are held back for changing too often. |
| `wigo_notifications_suppressed{scope,target,probe,kind}` | One per ack or silence currently in force. |

The last two are there so you can alert on your own alerting: an ack nobody ever lifted and a probe flapping for a week are both monitoring that has quietly stopped reaching anyone, and neither shows up in a status.

`/metrics` sits behind the same HTTP basic auth as the rest, so give Prometheus a `basic_auth` block when `Login` is set.

### Repeating, escalating and quiet hours

Notifications fire on a status **change**. A probe that goes critical at 3am and stays critical produces exactly one message, at 3am, and then six hours of silence that look exactly like six hours of everything being fine.

| Option | |
|---|---|
| `RenotifyInterval` | Say it again this often, in seconds, while the problem lasts. `0` keeps the old behaviour. |
| `EscalateAfter` | After this long unattended, also notify the apprise targets marked `Escalation = true`. |
| `QuietHoursFrom` / `QuietHoursTo` | The window nobody wants to be woken in, as `"22:00"` / `"08:00"`. |
| `QuietHoursMinLevelToSend` | What gets through it anyway. |

Repeats go through the same door as everything else, so **acknowledging something is how you tell it to stop repeating itself** — as is silencing it, and a flapping probe is not repeated about either.

**Quiet hours delay, they do not drop.** A notification held there is not recorded as sent, so the repeat loop says it as soon as the window closes. That needs `RenotifyInterval` to be set; without it, quiet hours would lose the message, which is not something a monitoring tool should do.

Escalation counts from when the problem appeared, not from the last repeat, and a recovery forgets it — the next problem starts its own clock instead of inheriting one. Escalation targets are left out of the normal path: the point of being second in line is not to be woken first.

### Flap detection

A probe oscillating OK to CRITICAL and back every minute sends two notifications a minute, all night. None of them is wrong, and together they are worse than useless: the real incident of the evening ends up buried under fifty messages about a link that keeps renegotiating.

Once a probe has changed status `FlapThreshold` times inside `FlapWindow` seconds it is **called out once** and then left alone until it settles. Defaults: 5 changes in an hour, on. Set `FlapDetection = false` in `[Notifications]` to turn it off.

Nothing is hidden. The probe keeps running, its status keeps being computed and displayed, and the early transitions are notified normally — the threshold is only reached after several of them, so nobody ever learns about a problem for the first time from a flapping notice. `GET /api/suppressions` lists what is currently flapping, and the interface badges it.

Settling takes more than crossing back over the threshold: a probe is called steady again once it drops to *half* of it. Without that gap a probe sitting exactly on the boundary would flap in and out of the flapping state itself, and every crossing would be worth a notification.

### Managing the probes of a remote host

The same three actions exist for any host a master polls, so the web interface works the same whether you are looking at the master or at one of its remotes:

| Endpoint |
|---|
| `GET /api/hosts/:hostname/schedule` |
| `POST /api/hosts/:hostname/probes/:probe/disable` |
| `POST /api/hosts/:hostname/probes/:probe/interval?seconds=300` |

The master **forwards** the call to the remote's own API, so **both ends must allow it**:

| `AllowWriteActions` on the master | on the remote | Result |
|---|---|---|
| true | true | applied |
| true | false | **403**, refused by the remote in its own words |
| false | either | **403**, the master will not forward |

A master that has write actions off performs none and will not act as a jump host onto the fleet either. A remote cannot tell a forwarded call from an administrator running `curl`, so its own flag is the one that actually protects it.

Credentials used to reach a remote never appear in the API output.

#### Hosts that push instead of being polled

A push client sits behind a NAT and cannot be called, so it asks for its orders on the connection it already keeps open, at every `PushInterval`. The API answers **202** and says so: the change is applied on the next push, not straight away.

`DisableRecords` says who turned a probe off, when, why and until when — one entry per probe an operator actually decided about. Most disabled probes are not in it: they were never enabled, and attributing those to somebody would be a lie.

The record is **metadata and nothing else**. Whether a probe runs is decided by the probes directory alone, so a database that is missing, corrupted or read-only cannot silently stop the monitoring. It is read to act in exactly one place — the expiry loop, which can only ever *start* a probe. A probe put back by hand, by a package upgrade or by anything not going through the API leaves a stale record behind; the directory is what is true, so such a record is deleted rather than reported.

`?for=` records a deadline and the interval the probe was running at, and the probe is scheduled again at that interval once the deadline passes. It is refused on a probe that is not currently running, since there would be nothing to bring it back to. Without it a probe stays off until somebody turns it back on — which is the right default for "no raid on this host", and the wrong one for "quiet during the migration".

There is no author to record yet: the API is behind a single shared basic auth credential, so `Author` is the login that was used and the address it came from. When a master drives another host, the author it recorded travels with the order and is kept *alongside* who actually connected, not instead of it — it is a claim, not a fact, until F8 lands.

`SkippedProbes` lists the probes that ran, exited with the special code **13** and asked not to be run again — `check_mdadm` on a machine with no raid array is the usual case. They are scheduled and produce no result, which from the outside is indistinguishable from a probe that has never run, so they are named rather than left to be guessed at. A restart clears the list, and so does rechecking one: if nothing changed it exits 13 again during that very run and takes itself back out.

`run` exists for the wait after a fix: an hourly probe that just went critical is an hour of not knowing whether the repair worked. The schedule is untouched — the probe runs once and its next scheduled run happens as it would have. A disabled probe is refused, since putting a fresh result on screen for a check that is not happening is the one thing being disabled has to stay visible as. The wait is capped at 30 seconds rather than the probe's usual interval-minus-one, because an HTTP request is held open on it.

A push client also reports its **whole probe schedule** on every update. Without it a probe it has disabled would be invisible from the server: a disabled probe produces no result, and results are all a client used to send. A client too old to report one is listed as unreadable rather than as having nothing disabled.

A client only ever receives an order if it opted in with `AllowRemoteControl` in its own `[PushClient]` section. It reports that on every update, so:

- a client that never said anything — including one running a version that predates this — is treated as refusing, and the server answers **403** naming the option to set;
- turning the option off drops whatever was already queued for it, so nothing is applied the day it reconnects for another reason;
- a client that stops asking does not make the server grow: its queue is capped and the oldest orders are dropped.

Orders go through the same checks as the local API, so a probe name arriving over the wire is validated and an interval bounded exactly as one arriving over HTTP: a server cannot reach outside a client's probes directory.

A host sitting **behind another wigo** is neither polled nor pushing here, and answers **501** with an explanation.

#### Finding what is turned off

A disabled probe is a blind spot, and on a fleet it is invisible from the host it sits on. The **Disabled probes** page of the web interface lists every one of them across every host the master can read, with a control to bring each back. Probes shipped but never enabled are listed too — nothing schedules them either.

It says out loud when a host could not be read, and names it: a list that quietly skipped a host would be worse than no list at all.

### Writing a probe

A probe is an **executable** (any language) that prints a single JSON object to stdout. Required field: **`Status`** (integer, see [Status codes](#status-codes)). Optional: `Message`, `Detail`, `Version`, `Metrics` (for OpenTSDB).

Example output:

```json
{
  "Status": 100,
  "Message": "0.38 0.26 0.24",
  "Detail": "",
  "Version": "0.11",
  "Metrics": [
    { "Value": 0.38, "Tags": { "load": "load5" } },
    { "Value": 0.26, "Tags": { "load": "load10" } },
    { "Value": 0.24, "Tags": { "load": "load15" } }
  ]
}
```

If the process fails (non-zero exit, timeout), Wigo reports status **500** for that probe. Exit code **12** disables the probe until restart; **13** disables and removes its result.

Perl probes get `Wigo::Probe` from `lib/`, which `use lib` puts at the **front** of `@INC`. Nothing else lives there: the modules the probes need are Debian packages, named in the `Depends:` line of the .deb — `libjson-perl`, `libnet-dns-perl`, `libnet-ntp-perl`, `libwww-perl`.

`Net::NTP` used to be shipped in `lib/` as well, *and* depended on. Since `use lib` prepends, the shipped copy won: the package paid for a dependency it then shadowed with a copy three versions behind (1.2 against Debian's 1.5). Vendoring a CPAN module here is not a fallback, it is an override — so it is not done, and a probe that needs a module gets it from the distribution.

---

## Building from source

### Prerequisites

- Go and GCC
- Node.js and npm (for the frontend)
- `GOPATH` set in your environment

### Install dependencies

```sh
make deps
```

### Build

```sh
make clean release
```

### Build for all supported architectures (amd64 & armhf)

Requires `arm-linux-gnueabihf-gcc`:

```sh
make clean releases
```

### Build Debian packages

Requires `dpkg-deb`:

```sh
make debs
```

### Run the tests

```sh
make test
```

Runs the Go test suite with the race detector. `make lint` checks `gofmt` and
`go vet` without touching the files, `make fmt` rewrites them, and `make all`
runs the lint and the tests along with the build.

### A note on languages

**Everything a user can read is in English**: labels, titles, `aria-label`,
placeholders, and every message the API can answer with. The interface declares
`lang="en"` and there is no i18n layer, so a French string in there is simply a
string most readers cannot read.

**Comments are in French**, and deliberately so — they are read by whoever
maintains this, not by whoever runs it. The two rules are independent, which is
easy to forget while writing a component that has both a comment and a title on
the same line.

### Releasing

Releases are driven by the `VERSION` file:

```sh
echo 0.75.0 > VERSION
git commit -am "version v0.75.0" && git push
```

The CI builds and tests that commit, then the release workflow takes over: it
creates the `v0.75.0` tag on it, downloads the debian packages the CI has just
built, and publishes the release with them attached and the list of the commits
since the previous tag.

Nothing happens when `VERSION` holds a version that is already tagged, so
ordinary pushes are left alone. A failing CI releases nothing, and pushing the
fix is enough to trigger the release again.

### Development mode

Run Wigo with hot-reload for backend and frontend:

```sh
make run-dev
```

The web UI is at `http://localhost:4400/` (or the port in `etc/wigo-dev.conf`). Stop with `Ctrl+C`.

**Note:** Development mode needs `sudo`. Artifacts are stored in `dev/`.

---

## Usage

### Web interface

Web interface requires the HTTP API to be enabled locally in `[Http]` (see [Configuration](#configuration)).

By default the web interface is at: **`http://[your-ip]:4000/`**

### CLI

CLI requires the HTTP API to be enabled locally in `[Http]` (see [Configuration](#configuration)).

```sh
/usr/local/wigo/bin/wigocli
```

Example output:

```
Wigo v0.51.5 running on backbone.root.gg
Local Status    : 100
Global Status   : 250

Remote Wigos :

    1.2.3.4:4000 ( ns2 ) - Wigo v0.51.5:
        apt-security              : 100  No security packages availables
        hardware_disks            : 250  Highest occupation percentage is 93% in partition /dev/md0
        hardware_load_average     : 100  0.09 0.04 0.05
        ...
```

- **Local Status** — worst status among probes on this host.
- **Global Status** — same as local here; for the whole view, check each remote’s status in the list.

#### Exit codes

`wigocli` exits with what a monitoring scheduler expects, taken from **the worst thing it showed**:

| | |
|---|---|
| `0` | OK — includes INFO |
| `1` | WARNING |
| `2` | CRITICAL |
| `3` | UNKNOWN — wigo's ERROR level, and anything that stopped it answering at all |

Not being able to ask is `3`, not `0`: a check that could not run is not a check that passed. The same goes for an unreadable answer, an unknown probe name, or a `--status` nobody can parse.

**This is a change**: it used to exit `0` whatever it found.

#### Options

| | |
|---|---|
| `--json` | Print what was asked for as JSON instead of a summary. |
| `--group=GROUP` | Only hosts in this group — including this one, which is a host like any other. |
| `--status=STATUS` | Only what is at or above this status: a level name (`ok`, `info`, `warning`, `critical`, `error`) or a number, because the scale is finer than its five names. |
| `--watch=SECONDS` | Print again every SECONDS until interrupted. |

Filtering happens on the tree before anything is rendered, so the summary, the JSON and the exit code agree by construction. The exit code is read from **what was shown**, not from what was fetched: asking about one group and being told about another one's outage is the kind of answer that costs somebody an hour.

```sh
# Nothing critical anywhere? (for a scheduler)
wigocli --status=critical

# What is wrong in one group, as json
wigocli --group=databases --status=warning --json
```
