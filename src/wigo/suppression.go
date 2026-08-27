package wigo

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// Stopping the notifications about something without stopping to watch it.
//
// This is not disabling a probe, and the difference matters : a disabled probe
// is not executed at all and produces no data, while a suppressed one keeps
// running, keeps being displayed and keeps its history. Only the notifications
// stop. Reaching for disable when what you meant was "stop telling me for two
// hours" is how a fleet ends up with blind spots nobody remembers creating.
//
// Two shapes, because two different things happen in practice :
//
//   - a silence is a window. "We are migrating this database until 4pm." It
//     expires on its own, and it must expire : a silence with no end is just an
//     unmonitored host with extra steps.
//
//   - an ack says "I know, I am on it". It has no end date because nobody knows
//     when the fix will land. It clears itself when the thing recovers, and it
//     deliberately does not survive the situation getting worse.
//
// That last point is the one worth being careful about. Acknowledging a
// WARNING must not swallow its turn to CRITICAL : what was acknowledged is not
// what is happening any more. So an ack records the status it was taken at,
// stops the notifications at or below it, and steps aside for anything worse.

const (
	SuppressionAck     = "ack"
	SuppressionSilence = "silence"

	SuppressionScopeHost  = "host"
	SuppressionScopeGroup = "group"
)

// A silence must end. A year is already far past "maintenance window", and a
// deadline nobody will ever see is the thing this exists to prevent.
const maxSilenceDuration = 365 * 24 * time.Hour

// Suppression is one decision to stop notifying about something.
type Suppression struct {
	Id int64

	Kind  string // ack or silence
	Scope string // host or group

	// The host or the group, depending on Scope.
	Target string

	// One probe of that target, or empty for all of them.
	Probe string

	Reason string
	Author string

	// The status an ack was taken at. Anything worse is notified anyway, since
	// it is not what anyone acknowledged. Unused by a silence.
	Status int

	CreatedAt int64

	// When a silence ends. Zero for an ack, which ends when the problem does.
	Until int64
}

const createSuppressionsTable = `
    CREATE TABLE IF NOT EXISTS suppressions (
        id integer not null primary key,
        kind text not null,
        scope text not null,
        target text not null,
        probe text,
        reason text,
        author text,
        status int,
        created_at int,
        until int
    ) ;
    `

// Active reports whether the suppression still applies at that time.
func (suppression Suppression) Active(now int64) bool {
	if suppression.Kind == SuppressionSilence {
		return suppression.Until > now
	}

	return true
}

// Covers reports whether the suppression is about this host, group and probe.
func (suppression Suppression) Covers(hostname string, group string, probe string) bool {
	switch suppression.Scope {
	case SuppressionScopeHost:
		if suppression.Target != hostname {
			return false
		}
	case SuppressionScopeGroup:
		if suppression.Target != group || group == "" {
			return false
		}
	default:
		return false
	}

	// An empty probe covers the whole target, including the host up and down
	// notifications, which belong to no probe.
	return suppression.Probe == "" || suppression.Probe == probe
}

// Suppresses reports whether a notification at that status must be held back.
//
// An ack steps aside for anything worse than what was acknowledged : higher is
// worse on this scale, and the point of acknowledging a WARNING is not to be
// kept in the dark when it turns CRITICAL.
func (suppression Suppression) Suppresses(status int) bool {
	if suppression.Kind != SuppressionAck {
		return true
	}

	return status <= suppression.Status
}

// AddSuppression records one, replacing whatever covered the same target.
//
// Two suppressions on the same thing would mean deciding which wins on every
// notification, and an operator re-acknowledging a probe means "this, now",
// not "this as well as what I said an hour ago".
func AddSuppression(suppression Suppression) error {
	if LocalWigo == nil || LocalWigo.sqlLiteConn == nil {
		return fmt.Errorf("no database to record it in")
	}

	if suppression.Target == "" {
		return fmt.Errorf("a suppression needs a host or a group to be about")
	}
	if suppression.Kind != SuppressionAck && suppression.Kind != SuppressionSilence {
		return fmt.Errorf("unknown suppression kind %q", suppression.Kind)
	}
	if suppression.Scope != SuppressionScopeHost && suppression.Scope != SuppressionScopeGroup {
		return fmt.Errorf("unknown suppression scope %q", suppression.Scope)
	}
	if suppression.Probe != "" && !IsValidProbeName(suppression.Probe) {
		return fmt.Errorf("invalid probe name %q", suppression.Probe)
	}
	if suppression.Kind == SuppressionSilence && suppression.Until <= time.Now().Unix() {
		return fmt.Errorf("a silence must end in the future")
	}

	if err := RemoveSuppression(suppression.Scope, suppression.Target, suppression.Probe); err != nil {
		return err
	}

	LocalWigo.sqlLiteLock.Lock()
	defer LocalWigo.sqlLiteLock.Unlock()

	_, err := LocalWigo.sqlLiteConn.Exec(
		`INSERT INTO suppressions(kind,scope,target,probe,reason,author,status,created_at,until)
		 VALUES(?,?,?,?,?,?,?,?,?);`,
		suppression.Kind, suppression.Scope, suppression.Target, suppression.Probe,
		suppression.Reason, suppression.Author, suppression.Status,
		suppression.CreatedAt, suppression.Until)

	return err
}

// RemoveSuppression lifts the one covering exactly that target.
func RemoveSuppression(scope string, target string, probe string) error {
	if LocalWigo == nil || LocalWigo.sqlLiteConn == nil {
		return fmt.Errorf("no database to remove it from")
	}

	LocalWigo.sqlLiteLock.Lock()
	defer LocalWigo.sqlLiteLock.Unlock()

	_, err := LocalWigo.sqlLiteConn.Exec(
		`DELETE FROM suppressions WHERE scope = ? AND target = ? AND probe = ?;`, scope, target, probe)

	return err
}

// Suppressions returns the ones that still apply, dropping those that do not
// on the way. A silence that has run out is deleted rather than reported : it
// is over, and a list of expired silences is noise on a page whose job is to
// say what is currently quiet.
func Suppressions() []Suppression {
	if LocalWigo == nil || LocalWigo.sqlLiteConn == nil {
		return nil
	}

	LocalWigo.sqlLiteLock.Lock()
	rows, err := LocalWigo.sqlLiteConn.Query(
		`SELECT id,kind,scope,target,probe,reason,author,status,created_at,until
		 FROM suppressions ORDER BY created_at;`)
	if err != nil {
		LocalWigo.sqlLiteLock.Unlock()
		return nil
	}

	all := make([]Suppression, 0)
	for rows.Next() {
		var suppression Suppression
		if err := rows.Scan(&suppression.Id, &suppression.Kind, &suppression.Scope,
			&suppression.Target, &suppression.Probe, &suppression.Reason, &suppression.Author,
			&suppression.Status, &suppression.CreatedAt, &suppression.Until); err != nil {
			continue
		}
		all = append(all, suppression)
	}
	rows.Close()
	LocalWigo.sqlLiteLock.Unlock()

	now := time.Now().Unix()
	active := make([]Suppression, 0, len(all))
	expired := make([]int64, 0)

	for _, suppression := range all {
		if suppression.Active(now) {
			active = append(active, suppression)
		} else {
			expired = append(expired, suppression.Id)
		}
	}

	if len(expired) > 0 {
		forgetSuppressions(expired)
	}

	return active
}

func forgetSuppressions(ids []int64) {
	if len(ids) == 0 {
		return
	}

	placeholders := make([]string, len(ids))
	values := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		values[i] = id
	}

	LocalWigo.sqlLiteLock.Lock()
	defer LocalWigo.sqlLiteLock.Unlock()

	if _, err := LocalWigo.sqlLiteConn.Exec(
		`DELETE FROM suppressions WHERE id IN (`+strings.Join(placeholders, ",")+`);`, values...); err != nil {
		log.Printf("Unable to forget %d expired suppressions : %s", len(ids), err)
	}
}

// suppressionFor returns the one holding back a notification about this host,
// group, probe and status, if there is one.
//
// The most specific wins : a silence on one probe is a finer statement than one
// on its whole host, and an operator who set both meant the narrower one to
// apply to that probe.
func suppressionFor(hostname string, group string, probe string, status int) (Suppression, bool) {
	var best Suppression
	found := false

	for _, suppression := range Suppressions() {
		if !suppression.Covers(hostname, group, probe) {
			continue
		}
		if !suppression.Suppresses(status) {
			continue
		}

		if !found || suppressionIsMoreSpecific(suppression, best) {
			best = suppression
			found = true
		}
	}

	return best, found
}

func suppressionIsMoreSpecific(candidate Suppression, current Suppression) bool {
	if (candidate.Probe != "") != (current.Probe != "") {
		return candidate.Probe != ""
	}

	return candidate.Scope == SuppressionScopeHost && current.Scope == SuppressionScopeGroup
}

// clearAckOn drops the acks covering a target that is no longer in the state
// they were taken for : it recovered, or it got worse.
//
// Leaving one behind would be the dangerous half of this feature. An ack on a
// problem that has come and gone would silently hold back the next one, and an
// ack taken at WARNING would go on covering a host that has been CRITICAL for
// a week.
func clearAckOn(hostname string, group string, probe string, status int) {
	for _, suppression := range Suppressions() {
		if suppression.Kind != SuppressionAck {
			continue
		}
		if !suppression.Covers(hostname, group, probe) {
			continue
		}
		if status == suppression.Status {
			continue
		}

		if err := RemoveSuppression(suppression.Scope, suppression.Target, suppression.Probe); err != nil {
			log.Printf("Unable to clear the ack on %s : %s", suppression.Target, err)
			continue
		}

		if status < suppression.Status {
			log.Printf("Ack on %s cleared : it recovered", describeSuppressionTarget(suppression))
		} else {
			log.Printf("Ack on %s cleared : it got worse, from %d to %d",
				describeSuppressionTarget(suppression), suppression.Status, status)
		}
	}
}

func describeSuppressionTarget(suppression Suppression) string {
	if suppression.Probe != "" {
		return fmt.Sprintf("%s/%s", suppression.Target, suppression.Probe)
	}

	return suppression.Target
}
