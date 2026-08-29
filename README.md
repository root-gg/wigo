# Wigo

**Wigo** (What Is Going On) is a lightweight pull/push monitoring tool written in Go.

## Features

- **Probes in any language** — Write probes as binaries in the language of your choice
- **Notifications** — HTTP, email, and Apprise alerts when probe or host status changes
- **Proxy mode** — Monitor hosts behind NAT/gateways via PUSH mode
- **Metrics** — Send probe metrics to OpenTSDB

## Screenshots

### Main view

![Main view](https://github.com/user-attachments/assets/999138a2-0602-4da1-bb20-337a3f0433ed)

### Group view

![Group view](https://github.com/user-attachments/assets/2938c5c7-a5f7-497f-8f4d-9da7affdbccd)

### Host view

![Host view](https://github.com/user-attachments/assets/8a4a1aa4-afa2-4461-86b4-ff70dc910efa)

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

Packages for Debian 12 (Bookworm) are available from the project repository:

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
| `[[Notifications.AppriseTargets]]` | Apprise urls restricted to some groups and/or hosts (see below). |

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
```

| Field | Description |
|-------|-------------|
| `Name` | Optional label, shown in the logs when a notification is sent. Defaults to the position of the target in the file (`#1`, `#2`, …). |
| `Urls` | Apprise urls notified when the target matches. |
| `Groups` | Groups to notify. Matches the `Group` of the host that raised the notification. |
| `Hosts` | Hostnames to notify. |

`Groups` and `Hosts` are OR'ed: a notification matches if its group is listed
**or** its host is listed. Matching is case insensitive and `"*"` matches
everything. A target with neither `Groups` nor `Hosts` never matches — use
`AppriseUrls` to notify every host. Urls matched by several targets are
deduplicated, so a notification is never sent twice to the same url.

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

The Debian package only seeds the default symlinks on a **fresh install**, so upgrading never re-enables a probe you disabled nor recreates one you moved.

Probes config files are located in `ProbesConfigDirectory` (e.g. `/etc/wigo/conf.d`).

### Managing probes through the API

| Endpoint | Description |
|---|---|
| `GET /api/probes` | Every probe of this host with its interval, including the disabled ones. Answers `{ "Hostname", "WriteActionsAllowed", "Probes", "SkippedProbes" }`, so a client knows which host it is looking at, whether changing it would be refused, and which probes asked not to be run again. |
| `POST /api/probes/:probe/disable` | Remove every schedule of the probe. Already disabled is not an error. Takes `?reason=` and `?for=` (a duration, `1h` / `24h` / `168h`). |
| `POST /api/probes/:probe/interval?seconds=300` | Run the probe every 300 seconds, enabling it if it was disabled. |
| `POST /api/probes/:probe/run` | Run the probe now, out of band, and answer its fresh result. |

The two `POST` endpoints return **403** unless `AllowWriteActions` is set in the `[Http]` section. They act on the probes directory directly, so the change takes effect on the next cycle without a restart, and it survives one.

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

**Each wigo keeps its own history, and only its own.** A master reads a remote's through that remote's API, the same way it reads its schedule — storing the fleet's series on the master as well would write everything twice and make its database grow with the size of the fleet, which is the thing that pushes people towards a separate stack. A host that pushes rather than being polled cannot be asked, and says so.

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

### Live updates

`GET /api/events` streams what happens as server sent events, so a probe going critical shows up in the interface at once rather than at the next poll — up to a minute of looking at a green screen about a machine that is already down.

What travels is deliberately thin: *what* changed, not what it changed to. The browser refetches, which keeps one serialisation of the state instead of two and means a missed event costs nothing. A subscriber that cannot keep up has its events dropped rather than slowing the scheduler down, for the same reason.

**The periodic refresh is not replaced.** A stream can die quietly — a proxy, a laptop waking up — and a page that has stopped refreshing while looking up to date is worse than no stream at all. The stream adds immediacy; the poll stays the safety net. A keepalive every 25 seconds keeps idle proxies from cutting the connection.

`wigo_event_subscribers` is exposed to Prometheus: a number that only ever grows is a leak.

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

A token that is presented and refused does **not** fall back on the shared credential — a revoked token would otherwise still work for anyone who also knows the password. Revoking keeps the row: what it was called and when it was turned off is a question somebody asks later. `LastUsed` is recorded so the tokens nobody uses can be found and revoked.

Both gates apply to writes, and both must say yes: the host has to accept being changed (`AllowWriteActions`), and the caller has to be an operator. Forwarding to a remote is checked the same way, so reaching through this host is not a way around the check that just failed.

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
