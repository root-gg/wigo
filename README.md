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

Probes config files are located in `ProbesConfigDirectory` (e.g. `/etc/wigo/conf.d`).

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
