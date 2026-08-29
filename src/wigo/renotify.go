package wigo

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// Saying it again, because saying it once is not the same as being heard.
//
// Notifications fire on a status *change*. A probe that goes critical at 3am
// and stays critical produces exactly one message, at 3am, and then six hours
// of silence that look exactly like six hours of everything being fine. The
// only thing that ever breaks that silence is the recovery.
//
// So a problem that is still there is said again on an interval, escalated to
// more people if it stays unattended, and held rather than dropped during the
// hours somebody asked not to be woken.
//
// All three go through SendNotification, which means an ack, a silence and a
// flapping probe all keep working : acknowledging something is precisely how
// you tell it to stop repeating itself.

// A notification that was held is not a notification that was sent, so nothing
// is recorded for it and it goes out as soon as whatever held it lets go. That
// is what makes quiet hours a delay rather than a loss.
var lastNotified = struct {
	sync.Mutex
	at map[string]int64

	// When the problem currently being repeated about first appeared, which is
	// what escalation counts from.
	since map[string]int64
}{
	at:    make(map[string]int64),
	since: make(map[string]int64),
}

func notifyKey(hostname string, probe string) string {
	return hostname + "\x00" + probe
}

// recordNotified notes that something actually went out.
func recordNotified(hostname string, probe string, at int64) {
	key := notifyKey(hostname, probe)

	lastNotified.Lock()
	defer lastNotified.Unlock()

	lastNotified.at[key] = at
	if _, known := lastNotified.since[key]; !known {
		lastNotified.since[key] = at
	}
}

// forgetNotified drops what is remembered about something that is fine again,
// so the next problem starts its own clock rather than inheriting one.
func ForgetNotified(hostname string, probe string) {
	key := notifyKey(hostname, probe)

	lastNotified.Lock()
	defer lastNotified.Unlock()

	delete(lastNotified.at, key)
	delete(lastNotified.since, key)
}

func notifiedState(hostname string, probe string) (int64, int64) {
	key := notifyKey(hostname, probe)

	lastNotified.Lock()
	defer lastNotified.Unlock()

	return lastNotified.at[key], lastNotified.since[key]
}

func renotifyInterval() int64 {
	return int64(GetLocalWigo().GetConfig().Notifications.RenotifyInterval)
}

func escalateAfter() int64 {
	return int64(GetLocalWigo().GetConfig().Notifications.EscalateAfter)
}

// StartRenotify repeats what is still wrong, on an interval.
//
// It only ever repeats what a status change already said : it reads the current
// state and sends nothing that a change would not have sent.
func StartRenotify() {
	if renotifyInterval() <= 0 {
		log.Printf("Notifications : problems are reported once and not repeated, set RenotifyInterval to change that")
		return
	}

	log.Printf("Notifications : a problem that is still there is reported again every %s",
		time.Duration(renotifyInterval())*time.Second)

	go func() {
		for {
			time.Sleep(renotifyTick())
			renotifyOpenProblems()
		}
	}()
}

// renotifyTick is how often the tree is looked at.
//
// A minute is plenty for the usual intervals, but checking every minute for a
// repeat asked for every twenty seconds would silently turn it into a minute.
// The floor is there because repeating faster than that is not re-notification,
// it is the same spam this feature exists to avoid.
func renotifyTick() time.Duration {
	tick := time.Duration(renotifyInterval()) * time.Second

	if tick > time.Minute {
		return time.Minute
	}
	if tick < 10*time.Second {
		return 10 * time.Second
	}

	return tick
}

// renotifyOpenProblems walks the tree and says again what is still wrong.
func renotifyOpenProblems() {
	interval := renotifyInterval()
	if interval <= 0 {
		return
	}

	now := time.Now().Unix()
	minLevel := GetLocalWigo().GetConfig().Notifications.MinLevelToSend

	for _, open := range openProblems(minLevel) {
		at, since := notifiedState(open.Hostname, open.Probe)

		// Never notified at all means the change itself was held back, by an
		// ack, a silence or the quiet hours. Repeating it is exactly what has
		// to happen once that lets go, so it is treated as long overdue.
		if at != 0 && now-at < interval {
			continue
		}

		notification := NewNotificationFromMessageForHost(
			describeStillWrong(open, now, since), open.Hostname, open.Group)
		notification.SetStatus(open.Status)
		notification.Probe = open.Probe
		notification.Summary = open.Message

		// Escalation is about how long the problem has been open, not about how
		// long we have been repeating ourselves.
		if after := escalateAfter(); after > 0 && since > 0 && now-since >= after {
			notification.Escalated = true
		}

		SendNotification(notification)
	}
}

// openProblem is one thing currently worth talking about.
type openProblem struct {
	Hostname string
	Group    string
	Probe    string
	Status   int
	Message  string
}

// openProblems lists every probe of the tree currently bad enough to notify
// about, plus the hosts that are down.
func openProblems(minLevel int) []openProblem {
	problems := make([]openProblem, 0)

	collect := func(wigo *Wigo) {
		if wigo == nil {
			return
		}

		host := wigo.GetLocalHost()
		if host == nil {
			return
		}

		if !wigo.IsAlive {
			problems = append(problems, openProblem{
				Hostname: wigo.GetHostname(),
				Group:    wigo.GetGroup(),
				Status:   500,
				Message:  wigo.GlobalMessage,
			})
			return
		}

		for item := range host.Probes.IterBuffered() {
			result, ok := item.Val.(*ProbeResult)
			if !ok || result.Status < minLevel {
				continue
			}

			problems = append(problems, openProblem{
				Hostname: wigo.GetHostname(),
				Group:    wigo.GetGroup(),
				Probe:    result.Name,
				Status:   result.Status,
				Message:  result.Message,
			})
		}
	}

	collect(GetLocalWigo())
	for item := range GetLocalWigo().RemoteWigos.IterBuffered() {
		if remote, ok := item.Val.(*Wigo); ok {
			collect(remote)
		}
	}

	return problems
}

func describeStillWrong(open openProblem, now int64, since int64) string {
	what := fmt.Sprintf("Host %s", open.Hostname)
	if open.Probe != "" {
		what = fmt.Sprintf("Probe %s on host %s", open.Probe, open.Hostname)
	}

	if since > 0 {
		return fmt.Sprintf("%s is still at status %d after %s : %s",
			what, open.Status, humanDuration(now-since), open.Message)
	}

	return fmt.Sprintf("%s is still at status %d : %s", what, open.Status, open.Message)
}

// humanDuration keeps a notification readable. Being exact to the second says
// nothing useful about how long something has been broken.
func humanDuration(seconds int64) string {
	if seconds < 60 {
		return "less than a minute"
	}
	if seconds < 3600 {
		return plural(seconds/60, "minute")
	}
	if seconds < 86400 {
		return plural(seconds/3600, "hour")
	}

	return plural(seconds/86400, "day")
}

func plural(count int64, word string) string {
	if count > 1 {
		return fmt.Sprintf("%d %ss", count, word)
	}

	return fmt.Sprintf("%d %s", count, word)
}

// Quiet hours : the window somebody asked not to be woken in.
//
// Nothing is dropped. A notification held here is simply not recorded as sent,
// so the repeat loop says it as soon as the window closes -- which is the only
// version of this feature that is safe in a monitoring tool. A level can still
// get through, for the things worth being woken for.

// inQuietHours reports whether now falls inside the configured window.
func inQuietHours(now time.Time) bool {
	config := GetLocalWigo().GetConfig().Notifications

	from, okFrom := parseHourMinute(config.QuietHoursFrom)
	to, okTo := parseHourMinute(config.QuietHoursTo)
	if !okFrom || !okTo || from == to {
		return false
	}

	minutes := now.Hour()*60 + now.Minute()

	// A window that crosses midnight is the usual case, hence the two shapes
	if from < to {
		return minutes >= from && minutes < to
	}

	return minutes >= from || minutes < to
}

func parseHourMinute(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}

	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, false
	}

	var hour, minute int
	if _, err := fmt.Sscanf(parts[0], "%d", &hour); err != nil {
		return 0, false
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &minute); err != nil {
		return 0, false
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, false
	}

	return hour*60 + minute, true
}

// heldByQuietHours reports whether this notification waits for morning.
func heldByQuietHours(status int) bool {
	if !inQuietHours(time.Now()) {
		return false
	}

	// Some things are worth being woken for, and the operator says which
	if through := GetLocalWigo().GetConfig().Notifications.QuietHoursMinLevelToSend; through > 0 && status >= through {
		return false
	}

	return true
}

func logQuietHours(notification INotification) {
	config := GetLocalWigo().GetConfig().Notifications
	log.Printf("Notification held until %s, quiet hours are on : %s",
		config.QuietHoursTo, notification.GetMessage())
}
