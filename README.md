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

Two subdirectories are not check intervals and are never executed:

| Directory | Meaning |
|---|---|
| `examples/` | The probe executables themselves. A probe here is installed but not scheduled. |
| `disabled/` | Probes turned off. Moving a symlink here stops it from running; moving it back into an interval directory starts it again. |

**This directory is the source of truth** for which probes run and how often. Changing a probe's interval means moving its symlink to another interval directory, creating it if needed — `probes/900/` is as valid as `probes/60/`. Nothing else records that state, so a database that disagreed with the directory can never silently stop the monitoring.

The Debian package only seeds the default symlinks on a **fresh install**, so upgrading never re-enables a probe you disabled nor recreates one you moved.

Probes config files are located in `ProbesConfigDirectory` (e.g. `/etc/wigo/conf.d`).

### Managing probes through the API

| Endpoint | Description |
|---|---|
| `GET /api/probes` | Every probe of this host with its interval, including the disabled ones. Answers `{ "Hostname", "WriteActionsAllowed", "Probes" }`, so a client knows which host it is looking at and whether changing it would be refused. |
| `POST /api/probes/:probe/disable` | Move the probe to `disabled/`. |
| `POST /api/probes/:probe/interval?seconds=300` | Run the probe every 300 seconds, re-enabling it if it was disabled. |

The two `POST` endpoints return **403** unless `AllowWriteActions` is set in the `[Http]` section. They act on the probes directory directly, so the change takes effect on the next cycle without a restart, and it survives one.

A probe must already be installed to be acted upon: a name that only exists in `examples/`, or does not exist at all, is refused. A probe installed in **several** interval directories at once is also refused rather than half-moved — resolve it by hand first.

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

A push client also reports its **whole probe schedule** on every update. Without it a probe it has disabled would be invisible from the server: a disabled probe produces no result, and results are all a client used to send. A client too old to report one is listed as unreadable rather than as having nothing disabled.

A client only ever receives an order if it opted in with `AllowRemoteControl` in its own `[PushClient]` section. It reports that on every update, so:

- a client that never said anything — including one running a version that predates this — is treated as refusing, and the server answers **403** naming the option to set;
- turning the option off drops whatever was already queued for it, so nothing is applied the day it reconnects for another reason;
- a client that stops asking does not make the server grow: its queue is capped and the oldest orders are dropped.

Orders go through the same checks as the local API, so a probe name arriving over the wire is validated and an interval bounded exactly as one arriving over HTTP: a server cannot reach outside a client's probes directory.

A host sitting **behind another wigo** is neither polled nor pushing here, and answers **501** with an explanation.

#### Finding what is turned off

A disabled probe is a blind spot, and on a fleet it is invisible from the host it sits on. The **Disabled probes** page of the web interface lists every one of them across every host the master can read, with a control to bring each back.

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
