package wigo

import (
	"fmt"
	"log"
	"time"
)

// Why a probe was turned off, by whom, and until when.
//
// This is metadata and nothing else. Whether a probe runs is decided by the
// probes directory alone -- see probes_directory.go -- and never by this table.
// A database that is missing, corrupted or simply not writable therefore cannot
// silently stop the monitoring, which is the whole reason the effective state
// was never put in it.
//
// What it buys is the difference between a probe somebody turned off and one
// that was never turned on. Both are disabled, both are a check that is not
// happening, but only the first was a decision -- and a decision is worth
// showing, attributing and, if it was meant to be temporary, undoing on time.

// ProbeDisableRecord is one such decision.
type ProbeDisableRecord struct {
	Probe  string
	Reason string

	// Who asked. There is no identity to record yet : the API is behind a
	// single shared basic auth credential, so this is the login that was used
	// and the address it came from, which is all that is actually known. It
	// becomes a real author with F8.
	Author string

	// The interval the probe ran at when it was turned off. Without it an
	// expiring record would have nothing to put the probe back to.
	Interval int

	CreatedAt int64

	// Zero when it was turned off with no end date.
	Until int64
}

// Expired reports whether the record has an end date that has passed.
func (record ProbeDisableRecord) Expired(now int64) bool {
	return record.Until > 0 && record.Until <= now
}

const createDisabledProbesTable = `
    CREATE TABLE IF NOT EXISTS disabled_probes (
        probe text not null primary key,
        reason text,
        author text,
        run_interval int,
        created_at int,
        until int
    ) ;
    `

// recordProbeDisabled notes that somebody turned a probe off.
//
// Failing to write must never fail the disable itself : the probe is already
// stopped by then, and refusing to acknowledge that because a note could not be
// taken would leave the interface disagreeing with the disk.
func recordProbeDisabled(record ProbeDisableRecord) error {
	if LocalWigo == nil || LocalWigo.sqlLiteConn == nil {
		return fmt.Errorf("no database to record it in")
	}

	LocalWigo.sqlLiteLock.Lock()
	defer LocalWigo.sqlLiteLock.Unlock()

	_, err := LocalWigo.sqlLiteConn.Exec(
		`INSERT INTO disabled_probes(probe,reason,author,run_interval,created_at,until)
		 VALUES(?,?,?,?,?,?)
		 ON CONFLICT(probe) DO UPDATE SET reason=excluded.reason, author=excluded.author,
		     run_interval=excluded.run_interval, created_at=excluded.created_at, until=excluded.until;`,
		record.Probe, record.Reason, record.Author, record.Interval, record.CreatedAt, record.Until)

	return err
}

// forgetProbeDisabled drops the record of a probe that runs again.
func forgetProbeDisabled(probe string) error {
	if LocalWigo == nil || LocalWigo.sqlLiteConn == nil {
		return fmt.Errorf("no database to forget it from")
	}

	LocalWigo.sqlLiteLock.Lock()
	defer LocalWigo.sqlLiteLock.Unlock()

	_, err := LocalWigo.sqlLiteConn.Exec(`DELETE FROM disabled_probes WHERE probe = ?;`, probe)

	return err
}

// readProbeDisableRecords returns every record, whether or not it still
// describes reality.
func readProbeDisableRecords() ([]ProbeDisableRecord, error) {
	if LocalWigo == nil || LocalWigo.sqlLiteConn == nil {
		return nil, fmt.Errorf("no database to read them from")
	}

	LocalWigo.sqlLiteLock.Lock()
	defer LocalWigo.sqlLiteLock.Unlock()

	rows, err := LocalWigo.sqlLiteConn.Query(
		`SELECT probe,reason,author,run_interval,created_at,until FROM disabled_probes ORDER BY created_at;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]ProbeDisableRecord, 0)

	for rows.Next() {
		var record ProbeDisableRecord
		if err := rows.Scan(&record.Probe, &record.Reason, &record.Author,
			&record.Interval, &record.CreatedAt, &record.Until); err != nil {
			return nil, err
		}
		records = append(records, record)
	}

	return records, rows.Err()
}

// ProbeDisableRecords returns the records that still describe reality : one for
// every probe an operator turned off and that is indeed not running.
//
// A probe scheduled again from the command line, by a package upgrade or by
// anything else that does not go through this API leaves a record behind. The
// directory is what is true, so such a record is dropped rather than reported ;
// otherwise the interface would attribute to somebody a decision that has since
// been undone.
func ProbeDisableRecords() []ProbeDisableRecord {
	records, err := readProbeDisableRecords()
	if err != nil {
		return nil
	}

	current := make([]ProbeDisableRecord, 0, len(records))

	for _, record := range records {
		if IsProbeScheduled(record.Probe) {
			if err := forgetProbeDisabled(record.Probe); err != nil {
				log.Printf("Unable to forget the disable record of probe %s, which runs again : %s", record.Probe, err)
			}
			continue
		}

		current = append(current, record)
	}

	return current
}

// ExpireDisabledProbes puts back the probes whose disable was meant to be
// temporary and whose time is up.
//
// This is the only place the table causes anything to happen, and it can only
// ever start a probe, never stop one. A record that cannot be read leaves every
// probe exactly where the directory says it should be.
func ExpireDisabledProbes() {
	records, err := readProbeDisableRecords()
	if err != nil {
		return
	}

	now := time.Now().Unix()

	for _, record := range records {
		if !record.Expired(now) {
			continue
		}

		if IsProbeScheduled(record.Probe) {
			// Somebody put it back before the deadline, nothing to do but stop
			// claiming it is off
			if err := forgetProbeDisabled(record.Probe); err != nil {
				log.Printf("Unable to forget the expired disable record of probe %s : %s", record.Probe, err)
			}
			continue
		}

		if record.Interval < MinProbeInterval {
			// It was not running when it was turned off, so there is no
			// interval to put it back to. Say so once and drop the deadline
			// rather than repeat this every minute.
			log.Printf("Probe %s was disabled until now but was not scheduled when it was turned off, "+
				"so there is no interval to bring it back to. Leaving it disabled.", record.Probe)

			record.Until = 0
			if err := recordProbeDisabled(record); err != nil {
				log.Printf("Unable to clear the deadline of probe %s : %s", record.Probe, err)
			}
			continue
		}

		if err := ScheduleProbe(record.Probe, record.Interval); err != nil {
			log.Printf("Unable to bring probe %s back after its disable expired : %s", record.Probe, err)
			continue
		}

		if err := forgetProbeDisabled(record.Probe); err != nil {
			log.Printf("Unable to forget the disable record of probe %s : %s", record.Probe, err)
		}

		GetLocalWigo().AddLog(nil, INFO, fmt.Sprintf(
			"Probe %s was disabled by %s until now, it is scheduled every %d seconds again",
			record.Probe, describeAuthor(record.Author), record.Interval))
	}
}

// describeAuthor keeps a log line readable when nobody was recorded.
func describeAuthor(author string) string {
	if author == "" {
		return "someone"
	}

	return author
}

// StartDisabledProbesExpiry brings probes back as their deadlines pass.
//
// A minute is fine : the point of an expiry is not to be punctual to the
// second, it is that a probe turned off for an afternoon does not stay off for
// eight months because everyone forgot.
func StartDisabledProbesExpiry() {
	go func() {
		for {
			ExpireDisabledProbes()
			time.Sleep(time.Minute)
		}
	}()
}
