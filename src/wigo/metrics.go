package wigo

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// The Prometheus exposition format, written by hand.
//
// The official client library would pull in a dependency tree larger than this
// whole file for something that is, in the end, a few lines of text : a name,
// some labels, a number. Wigo already holds every value in memory, so there is
// nothing to collect and no registry to keep in sync.
//
// What is exposed is the whole tree, not just this host. A master already
// gathers its remotes, so scraping the master gets the fleet, and scraping a
// standalone wigo gets itself. There is no separate exporter to deploy and
// nothing to keep in step with the wigo topology.

// Everything below 100 and everything above it is an error, 100 is OK, so the
// raw status is exposed rather than a boolean : a rule can decide for itself
// where to draw the line, and it is the same scale the interface shows.
const metricsContentType = "text/plain; version=0.0.4; charset=utf-8"

// HttpMetricsHandler answers the Prometheus exposition of the whole tree.
func HttpMetricsHandler(w http.ResponseWriter, r *http.Request) (int, string) {
	w.Header().Set("Content-Type", metricsContentType)

	return 200, renderMetrics()
}

func renderMetrics() string {
	out := &strings.Builder{}

	writeHelp(out, "wigo_up", "gauge", "Whether this wigo is answering. Always 1 when scraped.")
	fmt.Fprintf(out, "wigo_up 1\n")

	hosts := metricsHosts()

	writeHelp(out, "wigo_host_up", "gauge", "Whether the host is reachable and reporting.")
	for _, host := range hosts {
		fmt.Fprintf(out, "wigo_host_up{%s} %s\n", hostLabels(host), boolValue(host.Up))
	}

	writeHelp(out, "wigo_host_status", "gauge",
		"Worst status of the host's probes. 100 is OK, higher is worse, below 100 is an error.")
	for _, host := range hosts {
		fmt.Fprintf(out, "wigo_host_status{%s} %d\n", hostLabels(host), host.Status)
	}

	writeHelp(out, "wigo_probe_status", "gauge",
		"Status of a probe. 100 is OK, higher is worse, below 100 is an error.")
	forEachProbe(hosts, func(host metricsHost, probe *ProbeResult) {
		fmt.Fprintf(out, "wigo_probe_status{%s} %d\n", probeLabels(host, probe.Name), probe.Status)
	})

	writeHelp(out, "wigo_probe_last_run_timestamp_seconds", "gauge",
		"When the probe last produced this result. A value that stops moving is a probe that stopped running.")
	forEachProbe(hosts, func(host metricsHost, probe *ProbeResult) {
		fmt.Fprintf(out, "wigo_probe_last_run_timestamp_seconds{%s} %d\n",
			probeLabels(host, probe.Name), probe.Timestamp)
	})

	writeHelp(out, "wigo_probe_interval_seconds", "gauge",
		"How often the probe is scheduled to run. Zero when reported by a wigo too old to say.")
	forEachProbe(hosts, func(host metricsHost, probe *ProbeResult) {
		fmt.Fprintf(out, "wigo_probe_interval_seconds{%s} %d\n",
			probeLabels(host, probe.Name), probe.Interval)
	})

	writeProbeMetrics(out, hosts)
	writeQuietMetrics(out, hosts)

	return out.String()
}

// writeProbeMetrics exposes what the probes themselves measured.
//
// This is the half that makes the exporter worth scraping : a load average or a
// disk usage is a number worth graphing, and until now it was lost unless
// OpenTSDB was configured. The probe's own tags become labels, prefixed so they
// can never collide with host, group or probe.
func writeProbeMetrics(out *strings.Builder, hosts []metricsHost) {
	writeHelp(out, "wigo_probe_metric", "gauge", "A value measured by a probe, with its own tags as labels.")

	forEachProbe(hosts, func(host metricsHost, probe *ProbeResult) {
		values, ok := probe.Metrics.([]interface{})
		if !ok {
			return
		}

		for _, entry := range values {
			metric, ok := entry.(map[string]interface{})
			if !ok {
				continue
			}

			value, ok := metric["Value"].(float64)
			if !ok {
				continue
			}

			labels := probeLabels(host, probe.Name)
			for _, tag := range sortedTags(metric["Tags"]) {
				labels += fmt.Sprintf(",tag_%s=%q", sanitiseLabelName(tag.key), escapeLabelValue(tag.value))
			}

			fmt.Fprintf(out, "wigo_probe_metric{%s} %s\n", labels, formatFloat(value))
		}
	})
}

// writeQuietMetrics exposes what is currently not being notified about.
//
// Alerting on your alerting is the point : an ack nobody ever lifted and a
// probe that has been flapping for a week are both monitoring that has quietly
// stopped reaching anyone, and neither shows up in a status.
func writeQuietMetrics(out *strings.Builder, hosts []metricsHost) {
	writeHelp(out, "wigo_probe_flapping", "gauge",
		"1 when a probe changes status so often that its notifications are held back.")

	flappingSet := make(map[string]bool)
	for _, name := range FlappingProbes() {
		flappingSet[name] = true
	}

	forEachProbe(hosts, func(host metricsHost, probe *ProbeResult) {
		fmt.Fprintf(out, "wigo_probe_flapping{%s} %s\n", probeLabels(host, probe.Name),
			boolValue(flappingSet[host.Name+"/"+probe.Name]))
	})

	writeHelp(out, "wigo_notifications_suppressed", "gauge",
		"1 for each ack or silence currently holding notifications back. Decided on this wigo.")

	for _, suppression := range Suppressions() {
		fmt.Fprintf(out, "wigo_notifications_suppressed{scope=%q,target=%q,probe=%q,kind=%q} 1\n",
			escapeLabelValue(suppression.Scope), escapeLabelValue(suppression.Target),
			escapeLabelValue(suppression.Probe), escapeLabelValue(suppression.Kind))
	}
}

// metricsHost is one host of the tree, flattened for the exposition.
type metricsHost struct {
	Name   string
	Group  string
	Up     bool
	Status int
	Probes *Host
}

// metricsHosts flattens the tree : this host and every remote it knows about.
func metricsHosts() []metricsHost {
	hosts := make([]metricsHost, 0)

	add := func(wigo *Wigo) {
		if wigo == nil || wigo.GetLocalHost() == nil {
			return
		}

		hosts = append(hosts, metricsHost{
			Name:   wigo.GetHostname(),
			Group:  wigo.GetGroup(),
			Up:     wigo.IsAlive,
			Status: wigo.GetLocalHost().Status,
			Probes: wigo.GetLocalHost(),
		})
	}

	add(GetLocalWigo())
	for item := range GetLocalWigo().RemoteWigos.IterBuffered() {
		if remote, ok := item.Val.(*Wigo); ok {
			add(remote)
		}
	}

	sort.Slice(hosts, func(i, j int) bool { return hosts[i].Name < hosts[j].Name })

	return hosts
}

// forEachProbe walks every probe of every host, in a stable order so a diff
// between two scrapes is readable by a human.
func forEachProbe(hosts []metricsHost, visit func(metricsHost, *ProbeResult)) {
	for _, host := range hosts {
		results := make([]*ProbeResult, 0)
		for item := range host.Probes.Probes.IterBuffered() {
			if probe, ok := item.Val.(*ProbeResult); ok {
				results = append(results, probe)
			}
		}

		sort.Slice(results, func(i, j int) bool { return results[i].Name < results[j].Name })

		for _, probe := range results {
			visit(host, probe)
		}
	}
}

func writeHelp(out *strings.Builder, name string, kind string, help string) {
	fmt.Fprintf(out, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, kind)
}

func hostLabels(host metricsHost) string {
	return fmt.Sprintf("host=%q,group=%q", escapeLabelValue(host.Name), escapeLabelValue(host.Group))
}

func probeLabels(host metricsHost, probe string) string {
	return fmt.Sprintf("%s,probe=%q", hostLabels(host), escapeLabelValue(probe))
}

func boolValue(value bool) string {
	if value {
		return "1"
	}

	return "0"
}

// formatFloat keeps the value readable and exact. %v would print 1e+06 for a
// byte count, which is valid but painful to read in a raw scrape.
func formatFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", value), "0"), ".")
}

type metricTag struct {
	key   string
	value string
}

// sortedTags makes a scrape stable : Go map iteration is deliberately random,
// and label order changing between scrapes makes any diff useless.
func sortedTags(raw interface{}) []metricTag {
	values, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}

	tags := make([]metricTag, 0, len(values))
	for key, value := range values {
		text, ok := value.(string)
		if !ok {
			text = fmt.Sprintf("%v", value)
		}
		tags = append(tags, metricTag{key: key, value: text})
	}

	sort.Slice(tags, func(i, j int) bool { return tags[i].key < tags[j].key })

	return tags
}

// sanitiseLabelName turns a probe's own tag into something Prometheus accepts.
// The tags come from probe authors, not from us, so anything can turn up.
func sanitiseLabelName(name string) string {
	out := make([]rune, 0, len(name))

	for i, r := range name {
		valid := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' ||
			(i > 0 && r >= '0' && r <= '9')
		if valid {
			out = append(out, r)
		} else {
			out = append(out, '_')
		}
	}

	if len(out) == 0 {
		return "_"
	}

	return string(out)
}

func escapeLabelValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "\n", `\n`)

	return value
}
