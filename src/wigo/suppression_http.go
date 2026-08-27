package wigo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/codegangsta/martini"
)

// Acking and silencing are decided where the notifications are sent, which on a
// fleet is the master, so these endpoints are answered locally and never
// forwarded. Silencing a remote from its own API would stop the notifications
// it does not send.

// SuppressionList is what GET /api/suppressions answers.
//
// The write flag comes along because silencing is decided here, on the wigo
// that sends the notifications, and not on the host being silenced. A client
// reading a remote host's schedule learns what that remote allows, which is a
// different question and would be the wrong one to go by.
type SuppressionList struct {
	WriteActionsAllowed bool
	Suppressions        []Suppression
}

// HttpSuppressionsHandler lists what is currently held back.
func HttpSuppressionsHandler() (int, string) {

	body, err := json.Marshal(SuppressionList{
		WriteActionsAllowed: GetLocalWigo().GetConfig().Http.AllowWriteActions,
		Suppressions:        Suppressions(),
	})
	if err != nil {
		return 500, fmt.Sprintf("Fail to encode the suppressions : %s", err)
	}

	return 200, string(body)
}

// HttpHostAckHandler acknowledges the current state of a host, or of one of its
// probes with ?probe=name.
func HttpHostAckHandler(params martini.Params, r *http.Request) (int, string) {
	return addSuppressionFrom(SuppressionAck, SuppressionScopeHost, params["hostname"], r)
}

// HttpHostSilenceHandler stops the notifications about a host, or one of its
// probes, for a while.
func HttpHostSilenceHandler(params martini.Params, r *http.Request) (int, string) {
	return addSuppressionFrom(SuppressionSilence, SuppressionScopeHost, params["hostname"], r)
}

// HttpGroupSilenceHandler does the same for a whole group.
func HttpGroupSilenceHandler(params martini.Params, r *http.Request) (int, string) {
	return addSuppressionFrom(SuppressionSilence, SuppressionScopeGroup, params["group"], r)
}

// HttpHostUnsuppressHandler lifts whatever was holding a host, or one of its
// probes, quiet.
func HttpHostUnsuppressHandler(params martini.Params, r *http.Request) (int, string) {
	return removeSuppressionFrom(SuppressionScopeHost, params["hostname"], r)
}

// HttpGroupUnsuppressHandler does the same for a group.
func HttpGroupUnsuppressHandler(params martini.Params, r *http.Request) (int, string) {
	return removeSuppressionFrom(SuppressionScopeGroup, params["group"], r)
}

func addSuppressionFrom(kind string, scope string, target string, r *http.Request) (int, string) {

	if status, message, allowed := httpWriteActionsAllowed(); !allowed {
		return status, message
	}

	if target == "" {
		return 404, "No host or group set in url"
	}

	probe := r.URL.Query().Get("probe")
	if probe != "" && !IsValidProbeName(probe) {
		return 400, fmt.Sprintf("invalid probe name %q", probe)
	}

	suppression := Suppression{
		Kind:      kind,
		Scope:     scope,
		Target:    target,
		Probe:     probe,
		Reason:    r.URL.Query().Get("reason"),
		Author:    httpAuthor(r, r.URL.Query().Get("author")),
		CreatedAt: time.Now().Unix(),
	}

	switch kind {
	case SuppressionSilence:
		duration, err := parseSilenceDuration(r.URL.Query().Get("for"))
		if err != nil {
			return 400, err.Error()
		}
		suppression.Until = suppression.CreatedAt + duration

	case SuppressionAck:
		// What is being acknowledged has to be read now : an ack is only good
		// for the state it was taken at, and anything worse notifies anyway.
		status, err := currentStatusOf(scope, target, probe)
		if err != nil {
			return 400, err.Error()
		}
		suppression.Status = status
	}

	if err := AddSuppression(suppression); err != nil {
		return 400, err.Error()
	}

	GetLocalWigo().AddLog(nil, INFO, describeSuppression(suppression))

	return HttpSuppressionsHandler()
}

func removeSuppressionFrom(scope string, target string, r *http.Request) (int, string) {

	if status, message, allowed := httpWriteActionsAllowed(); !allowed {
		return status, message
	}

	if target == "" {
		return 404, "No host or group set in url"
	}

	probe := r.URL.Query().Get("probe")
	if probe != "" && !IsValidProbeName(probe) {
		return 400, fmt.Sprintf("invalid probe name %q", probe)
	}

	if err := RemoveSuppression(scope, target, probe); err != nil {
		return 400, err.Error()
	}

	GetLocalWigo().AddLog(nil, INFO, fmt.Sprintf("Notifications about %s are back on, lifted by %s",
		describeSuppressionTarget(Suppression{Target: target, Probe: probe}),
		httpAuthor(r, r.URL.Query().Get("author"))))

	return HttpSuppressionsHandler()
}

// currentStatusOf reads what is being acknowledged right now.
//
// A group has no single status, so acking one is refused rather than
// acknowledged at some invented level : "I know about this" is a statement
// about one thing, and a group of forty hosts is not one thing. Silencing a
// group, which makes no claim about state, is allowed.
func currentStatusOf(scope string, target string, probe string) (int, error) {
	if scope != SuppressionScopeHost {
		return 0, fmt.Errorf("a group cannot be acknowledged, it has no single status. Silence it instead")
	}

	host, err := hostNamed(target)
	if err != nil {
		return 0, err
	}

	if probe == "" {
		return host.Status, nil
	}

	stored, found := host.Probes.Get(probe)
	if !found {
		return 0, fmt.Errorf("host %q has no probe named %q to acknowledge", target, probe)
	}

	result, ok := stored.(*ProbeResult)
	if !ok {
		return 0, fmt.Errorf("host %q has no readable result for probe %q", target, probe)
	}

	return result.Status, nil
}

// hostNamed finds a host of the tree by the name it reports for itself.
func hostNamed(hostname string) (*Host, error) {
	if hostname == GetLocalWigo().GetHostname() {
		return GetLocalWigo().GetLocalHost(), nil
	}

	remote := GetLocalWigo().FindRemoteWigoByHostname(hostname)
	if remote == nil {
		return nil, fmt.Errorf("host %q not found", hostname)
	}

	return remote.GetLocalHost(), nil
}

// parseSilenceDuration reads how long a silence lasts. Unlike a disable it is
// required : a silence with no end is an unmonitored host with extra steps.
func parseSilenceDuration(value string) (int64, error) {
	if value == "" {
		return 0, fmt.Errorf("a silence needs a duration, expected something like ?for=2h")
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q, expected something like 2h, 24h or 168h", value)
	}

	if duration < time.Minute || duration > maxSilenceDuration {
		return 0, fmt.Errorf("a silence must last between a minute and a year, got %s", duration)
	}

	return int64(duration.Seconds()), nil
}

func describeSuppression(suppression Suppression) string {
	message := fmt.Sprintf("Notifications about %s held back", describeSuppressionTarget(suppression))

	if suppression.Kind == SuppressionAck {
		message += fmt.Sprintf(" : acknowledged at status %d by %s",
			suppression.Status, describeAuthor(suppression.Author))
	} else {
		message += fmt.Sprintf(" : silenced by %s until %s",
			describeAuthor(suppression.Author), time.Unix(suppression.Until, 0).Format(dateLayout))
	}

	if suppression.Reason != "" {
		message += fmt.Sprintf(" (%s)", suppression.Reason)
	}

	return message
}
