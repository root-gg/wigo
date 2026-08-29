# Wigo in a container.
#
# The image is deliberately not built on scratch or alpine, even though the
# binary is static and would happily run there. Wigo without its probes is a web
# interface showing nothing, and the probes shipped with it are Perl scripts :
# a distroless image would produce something that starts, answers, and monitors
# absolutely nothing.
#
# Debian slim plus the Perl modules the shipped probes actually use is the
# smallest base on which `docker run wigo` does what the name promises.

FROM golang:1-bookworm AS build

WORKDIR /src

# Dependencies first, so a change to the source does not refetch them
COPY go.mod go.sum ./
RUN go mod download

COPY VERSION ./
COPY src ./src

# Static, so the runtime stage only needs what the probes need
RUN CGO_ENABLED=0 go build \
        -tags "netgo osusergo" \
        -ldflags "-s -w -X github.com/root-gg/wigo/src/wigo.Version=$(cat VERSION)" \
        -o /out/wigo ./src/cmd/wigo \
 && CGO_ENABLED=0 go build \
        -tags "netgo osusergo" \
        -ldflags "-s -w -X github.com/root-gg/wigo/src/wigo.Version=$(cat VERSION)" \
        -o /out/wigocli ./src/cmd/wigocli


FROM node:22-bookworm-slim AS frontend

# The vite build writes to ../../../public, so the tree has to keep its shape
WORKDIR /src/src/public
COPY src/public/package.json src/public/package-lock.json ./
RUN npm ci

COPY src/public ./
RUN npm run build


FROM debian:bookworm-slim

# What the probes shipped in probes/examples/ need to run. Without these the
# container starts and every probe fails, which is worse than not starting.
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
        perl \
        libjson-perl \
        libwww-perl \
        libnet-ntp-perl \
        procps \
        coreutils \
        ca-certificates \
 && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/wigo /out/wigocli /usr/local/bin/
COPY lib /usr/local/wigo/lib
COPY etc/conf.d /usr/local/wigo/etc/conf.d

# Kept outside the probes volume on purpose. A named volume is seeded from the
# image, but a bind mount onto an empty host directory is not : without a
# pristine copy to restore from, `-v ./probes:/usr/local/wigo/probes` would
# produce a wigo with no probes at all and no way to get any.
COPY probes/examples /usr/local/share/wigo/probes-examples
COPY etc/wigo.conf /usr/local/wigo/etc/wigo.conf.sample
COPY --from=frontend /src/public /usr/local/wigo/public

COPY build/docker/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

# martini.Static("public") is relative to the working directory, and the probes
# resolve their library through it too. The systemd unit sets the same one.
WORKDIR /usr/local/wigo

# The probes directory is the source of truth for what runs and how often, so it
# is a volume : losing it on every container restart would silently undo every
# probe an operator enabled, disabled or repitched.
VOLUME ["/usr/local/wigo/probes", "/var/lib/wigo"]

EXPOSE 4000

# Answering the api is the only thing that proves the whole chain is up : the
# process running is not the same as the interface being served.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
    CMD wigocli || exit 1

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["wigo", "--config", "/etc/wigo/wigo.conf"]
