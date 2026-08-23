package wigo

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The probes directory is the source of truth for which probes run and how
// often. A probe is a file, usually a symlink into probes/examples/, sitting in
// probes/<interval in seconds>/ while it is scheduled and in probes/disabled/
// once it is turned off.
//
// Nothing else records that state : a database that disagreed with the
// directory could silently stop the monitoring, and the deb postinst only seeds
// the directory on a fresh install so an upgrade never undoes a choice made
// here.

const DisabledProbesDirectory = "disabled"

// The execution timeout of a probe is its interval minus one second, so an
// interval of one would kill every run on the spot.
const MinProbeInterval = 2

// A day. Longer is almost certainly a typo, and the probe would look stale
// forever without ever being reported as such.
const MaxProbeInterval = 86400

const maxProbeNameLength = 255

// A probe name is turned into a path, so it is matched against an allow list
// rather than sanitised. Requiring an alphanumeric first character rules out
// ".", ".." and hidden files, and the character set contains no separator.
var validProbeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ProbeLocation is where a probe currently sits in the probes directory.
type ProbeLocation struct {
	Name      string
	Directory string // "300" while scheduled, "disabled" once turned off
	Interval  int    // 0 when disabled
	Enabled   bool
}

// IsValidProbeName reports whether a name may be used to build a path inside
// the probes directory.
func IsValidProbeName(name string) bool {
	if name == "" || len(name) > maxProbeNameLength {
		return false
	}

	return validProbeName.MatchString(name)
}

// ProbeDirectoryInterval returns the check interval a probes sub directory
// stands for. Directories not named after an interval, such as examples and
// disabled, are not schedules and report false.
func ProbeDirectoryInterval(directory string) (int, bool) {
	interval, err := strconv.Atoi(directory)
	if err != nil || interval < 1 {
		return 0, false
	}

	return interval, true
}

// probePath builds the path of a probe inside the probes directory, and makes
// sure the result really is under it.
func probePath(root string, directory string, name string) (string, error) {
	if !IsValidProbeName(name) {
		return "", fmt.Errorf("invalid probe name %q", name)
	}

	if _, isSchedule := ProbeDirectoryInterval(directory); !isSchedule && directory != DisabledProbesDirectory {
		return "", fmt.Errorf("invalid probes directory %q", directory)
	}

	cleanRoot := filepath.Clean(root)
	full := filepath.Join(cleanRoot, directory, name)

	// Defence in depth : the allow list above already makes an escape
	// impossible, this catches a mistake in the allow list itself.
	if filepath.Dir(full) != filepath.Join(cleanRoot, directory) {
		return "", fmt.Errorf("probe path %q escapes the probes directory", full)
	}
	if !strings.HasPrefix(full, cleanRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("probe path %q escapes the probes directory", full)
	}

	return full, nil
}

// findProbeLocationsIn lists every directory a probe is currently installed in.
// More than one means it is scheduled at several intervals at once and runs
// several times per cycle.
func findProbeLocationsIn(root string, name string) ([]ProbeLocation, error) {
	if !IsValidProbeName(name) {
		return nil, fmt.Errorf("invalid probe name %q", name)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("fail to read the probes directory %s : %s", root, err)
	}

	locations := make([]ProbeLocation, 0)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		directory := entry.Name()
		interval, isSchedule := ProbeDirectoryInterval(directory)
		if !isSchedule && directory != DisabledProbesDirectory {
			continue
		}

		path, err := probePath(root, directory, name)
		if err != nil {
			continue
		}

		// Lstat, not Stat : a probe is a symlink and we act on the link, which
		// must be found even when whatever it points at is gone.
		if _, err := os.Lstat(path); err != nil {
			continue
		}

		locations = append(locations, ProbeLocation{
			Name:      name,
			Directory: directory,
			Interval:  interval,
			Enabled:   isSchedule,
		})
	}

	sort.Slice(locations, func(i, j int) bool {
		return locations[i].Directory < locations[j].Directory
	})

	return locations, nil
}

// moveProbeIn moves a probe to targetDirectory, creating that directory when
// needed.
func moveProbeIn(root string, name string, targetDirectory string) error {
	locations, err := findProbeLocationsIn(root, name)
	if err != nil {
		return err
	}

	if len(locations) == 0 {
		return fmt.Errorf("probe %q was not found in the probes directory", name)
	}

	// Being in two directories at once is pathological, and guessing which one
	// to act on would leave the probe running from the other. Say so instead.
	if len(locations) > 1 {
		directories := make([]string, 0, len(locations))
		for _, location := range locations {
			directories = append(directories, location.Directory)
		}
		return fmt.Errorf("probe %q is installed in several directories at once (%s), please resolve it by hand first",
			name, strings.Join(directories, ", "))
	}

	current := locations[0]
	if current.Directory == targetDirectory {
		return nil
	}

	source, err := probePath(root, current.Directory, name)
	if err != nil {
		return err
	}

	destination, err := probePath(root, targetDirectory, name)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Join(filepath.Clean(root), targetDirectory), 0755); err != nil {
		return fmt.Errorf("fail to create the %s directory : %s", targetDirectory, err)
	}

	// Rename replaces the destination without a word, and findProbeLocationsIn
	// just told us it should not exist.
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("%s already exists", destination)
	}

	// Rename is atomic within a filesystem : the probe is never present in both
	// directories, which would run it twice, nor missing from both.
	if err := os.Rename(source, destination); err != nil {
		return fmt.Errorf("fail to move probe %q from %s to %s : %s", name, current.Directory, targetDirectory, err)
	}

	return nil
}

// parseProbeInterval reads a check interval coming from a request and checks it
// is one we may schedule.
func parseProbeInterval(seconds string) (int, error) {
	interval, err := strconv.Atoi(seconds)
	if err != nil {
		return 0, fmt.Errorf("invalid interval %q, expected a number of seconds", seconds)
	}

	if interval < MinProbeInterval || interval > MaxProbeInterval {
		return 0, fmt.Errorf("interval must be between %d and %d seconds, got %d", MinProbeInterval, MaxProbeInterval, interval)
	}

	return interval, nil
}

func scheduleProbeIn(root string, name string, interval int) error {
	if interval < MinProbeInterval || interval > MaxProbeInterval {
		return fmt.Errorf("interval must be between %d and %d seconds, got %d", MinProbeInterval, MaxProbeInterval, interval)
	}

	return moveProbeIn(root, name, strconv.Itoa(interval))
}

func unscheduleProbeIn(root string, name string) error {
	return moveProbeIn(root, name, DisabledProbesDirectory)
}

// probeLocationsIn lists every probe of the directory with where it sits. A
// probe installed several times appears once per location.
func probeLocationsIn(root string) ([]ProbeLocation, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("fail to read the probes directory %s : %s", root, err)
	}

	locations := make([]ProbeLocation, 0)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		directory := entry.Name()
		interval, isSchedule := ProbeDirectoryInterval(directory)
		if !isSchedule && directory != DisabledProbesDirectory {
			continue
		}

		probes, err := os.ReadDir(filepath.Join(filepath.Clean(root), directory))
		if err != nil {
			continue
		}

		for _, probe := range probes {
			if probe.IsDir() || !IsValidProbeName(probe.Name()) {
				continue
			}

			locations = append(locations, ProbeLocation{
				Name:      probe.Name(),
				Directory: directory,
				Interval:  interval,
				Enabled:   isSchedule,
			})
		}
	}

	sort.Slice(locations, func(i, j int) bool {
		if locations[i].Name != locations[j].Name {
			return locations[i].Name < locations[j].Name
		}
		return locations[i].Directory < locations[j].Directory
	})

	return locations, nil
}

// probeIsScheduledIn reports whether a probe runs from some interval
// directory. Being unable to tell counts as not scheduled, so a probe that
// really went away is still forgotten.
func probeIsScheduledIn(root string, name string) bool {
	locations, err := findProbeLocationsIn(root, name)
	if err != nil {
		return false
	}

	for _, location := range locations {
		if location.Enabled {
			return true
		}
	}

	return false
}

func probesRoot() string {
	return GetLocalWigo().GetConfig().Global.ProbesDirectory
}

// ScheduleProbe runs a probe every interval seconds, enabling it when it was
// disabled.
func ScheduleProbe(name string, interval int) error {
	return scheduleProbeIn(probesRoot(), name, interval)
}

// UnscheduleProbe stops a probe from being scheduled at all.
func UnscheduleProbe(name string) error {
	return unscheduleProbeIn(probesRoot(), name)
}

// FindProbeLocations lists every directory a probe is currently installed in.
func FindProbeLocations(name string) ([]ProbeLocation, error) {
	return findProbeLocationsIn(probesRoot(), name)
}

// ProbeLocations lists every probe of the probes directory with where it sits.
func ProbeLocations() ([]ProbeLocation, error) {
	return probeLocationsIn(probesRoot())
}

// IsProbeScheduled reports whether a probe runs from some interval directory.
// Leaving one directory does not mean a probe is gone : it may have been
// repitched to another interval, and dropping its result on the way would
// delete what the new directory just produced.
func IsProbeScheduled(name string) bool {
	return probeIsScheduledIn(probesRoot(), name)
}
