package wigo

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The probes directory is the source of truth for which probes run and how
// often. Probes live in probes/examples/, and a probe runs when some
// probes/<interval in seconds>/ directory links to it. A probe no interval
// directory links to is disabled : it is installed on the machine and nothing
// executes it.
//
// There is no directory of disabled probes, because being disabled is not a
// place a probe is put, it is the absence of any schedule. Wigo ships around
// thirty probes and the packaging enables half of them, so most disabled probes
// were never turned off by anyone -- they simply were never turned on, which
// amounts to the same missing check.
//
// Nothing else records that state : a database that disagreed with the
// directory could silently stop the monitoring, and the deb postinst only seeds
// the directory on a fresh install so an upgrade never undoes a choice made
// here.

// Where the probes themselves live, and where a probe goes back to when nothing
// schedules it any more.
const ExampleProbesDirectory = "examples"

// An earlier shape of this feature parked disabled probes here. It is still
// read, so a probe turned off back then stays visible rather than quietly
// disappearing from the listing while remaining unscheduled, and any write
// moves the probe out of it.
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
	Directory string // "300" while scheduled, "examples" while nothing schedules it
	Interval  int    // 0 unless scheduled
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

// examplePath builds the path of the probe itself, the file the schedules link
// to.
//
// It is deliberately separate from probePath, which refuses this directory : a
// probe leaving examples means it is running, and one arriving means it stopped,
// so the two must not share a code path where one could pass for the other.
func examplePath(root string, name string) (string, error) {
	if !IsValidProbeName(name) {
		return "", fmt.Errorf("invalid probe name %q", name)
	}

	cleanRoot := filepath.Clean(root)
	full := filepath.Join(cleanRoot, ExampleProbesDirectory, name)

	if filepath.Dir(full) != filepath.Join(cleanRoot, ExampleProbesDirectory) {
		return "", fmt.Errorf("probe path %q escapes the probes directory", full)
	}

	return full, nil
}

// probeExistsInExamples reports whether the probe is installed on this machine,
// that is whether there is something for a schedule to link to.
func probeExistsInExamples(root string, name string) bool {
	path, err := examplePath(root, name)
	if err != nil {
		return false
	}

	// Lstat, not Stat : what is being asked is whether the name is taken. An
	// example that is itself a dangling link is a problem to report, not one to
	// paper over by treating the probe as absent and creating another.
	info, err := os.Lstat(path)
	return err == nil && !info.IsDir()
}

// linkProbeIn schedules a probe by linking to it from an interval directory.
//
// The link is relative and points into examples, which is the shape the
// packaging creates and the shape an administrator reading the directory
// expects.
func linkProbeIn(root string, name string, targetDirectory string) error {
	destination, err := probePath(root, targetDirectory, name)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Join(filepath.Clean(root), targetDirectory), 0755); err != nil {
		return fmt.Errorf("fail to create the %s directory : %s", targetDirectory, err)
	}

	if err := os.Symlink(filepath.Join("..", ExampleProbesDirectory, name), destination); err != nil {
		return fmt.Errorf("fail to schedule probe %q in %s : %s", name, targetDirectory, err)
	}

	return nil
}

// retireProbeTo moves a scheduled entry back into examples.
//
// This is what keeps disabling from destroying anything. The entry is usually a
// link into examples, where the probe already is, and removing it loses nothing.
// But an administrator may have dropped a script straight into an interval
// directory, or linked to one outside the probes tree : deleting that would be
// the only copy gone, and the probe would not even be listed any more, so there
// would be no way to turn it back on.
func retireProbeTo(root string, name string, fromDirectory string) error {
	source, err := probePath(root, fromDirectory, name)
	if err != nil {
		return err
	}

	destination, err := examplePath(root, name)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Join(filepath.Clean(root), ExampleProbesDirectory), 0755); err != nil {
		return fmt.Errorf("fail to create the %s directory : %s", ExampleProbesDirectory, err)
	}

	// A relative link keeps working : the interval directories and examples sit
	// side by side, one level under the probes directory.
	if err := os.Rename(source, destination); err != nil {
		return fmt.Errorf("fail to move probe %q out of %s : %s", name, fromDirectory, err)
	}

	log.Printf("Probe %s is no longer scheduled, it was moved from %s back to %s",
		name, fromDirectory, ExampleProbesDirectory)

	return nil
}

// dropProbeFrom removes a scheduled entry whose probe is kept elsewhere.
func dropProbeFrom(root string, name string, fromDirectory string) error {
	path, err := probePath(root, fromDirectory, name)
	if err != nil {
		return err
	}

	if target, err := os.Readlink(path); err == nil {
		log.Printf("Probe %s is no longer scheduled from %s (that link pointed at %s)", name, fromDirectory, target)
	} else {
		log.Printf("Probe %s is no longer scheduled from %s", name, fromDirectory)
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("fail to unschedule probe %q from %s : %s", name, fromDirectory, err)
	}

	return nil
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

// scheduleProbeIn makes a probe run every interval seconds, and only from
// there.
//
// A probe linked from several interval directories at once runs several times
// per cycle. Asking for an interval means asking for the probe to run every so
// often -- once -- so the extra links are removed rather than left behind :
// moving one of them would leave the probe running from the others, which is
// the very state being corrected.
func scheduleProbeIn(root string, name string, interval int) error {
	if interval < MinProbeInterval || interval > MaxProbeInterval {
		return fmt.Errorf("interval must be between %d and %d seconds, got %d", MinProbeInterval, MaxProbeInterval, interval)
	}

	targetDirectory := strconv.Itoa(interval)

	locations, err := findProbeLocationsIn(root, name)
	if err != nil {
		return err
	}

	if len(locations) == 0 {
		if !probeExistsInExamples(root, name) {
			return fmt.Errorf("probe %q was not found in the probes directory", name)
		}

		// Disabled, which is to say linked from nowhere. Scheduling it is
		// creating the link the packaging would have created.
		if err := linkProbeIn(root, name, targetDirectory); err != nil {
			return err
		}

		log.Printf("Probe %s is now scheduled every %d seconds", name, interval)
		return nil
	}

	// The entry that stays. One already sitting where it belongs is kept, which
	// leaves nothing to move afterwards.
	survivor := locations[0]
	for _, location := range locations {
		if location.Directory == targetDirectory {
			survivor = location
			break
		}
	}

	// The extras go first : should the move below fail, the probe is left
	// scheduled once instead of several times, which is still an improvement on
	// where we started.
	for _, location := range locations {
		if location.Directory == survivor.Directory {
			continue
		}

		if err := dropProbeFrom(root, name, location.Directory); err != nil {
			return err
		}
	}

	if survivor.Directory == targetDirectory {
		return nil
	}

	source, err := probePath(root, survivor.Directory, name)
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

	// Rename replaces the destination without a word. Nothing can be there : an
	// entry in the target directory would have been kept as the survivor.
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("%s already exists", destination)
	}

	// Rename is atomic within a filesystem : the probe is never linked from both
	// directories, which would run it twice, nor from neither.
	if err := os.Rename(source, destination); err != nil {
		return fmt.Errorf("fail to move probe %q from %s to %s : %s", name, survivor.Directory, targetDirectory, err)
	}

	log.Printf("Probe %s is now scheduled every %d seconds instead of from %s", name, interval, survivor.Directory)

	return nil
}

// unscheduleProbeIn stops a probe from running, leaving it installed.
//
// Disabling is not a place a probe is moved to, it is the absence of any
// schedule, so this removes every one of them. The probe itself stays in
// examples, which is what makes it possible to list it as disabled and to turn
// it back on later.
func unscheduleProbeIn(root string, name string) error {
	locations, err := findProbeLocationsIn(root, name)
	if err != nil {
		return err
	}

	if len(locations) == 0 {
		if !probeExistsInExamples(root, name) {
			return fmt.Errorf("probe %q was not found in the probes directory", name)
		}

		// Nothing schedules it, so it is already disabled. Saying so with an
		// error would make an interface refuse a state it is already in.
		return nil
	}

	for _, location := range locations {
		// The probe has to survive somewhere. It usually already does, as the
		// entry is a link into examples, and then the entry is just removed.
		if probeExistsInExamples(root, name) {
			if err := dropProbeFrom(root, name, location.Directory); err != nil {
				return err
			}
			continue
		}

		if err := retireProbeTo(root, name, location.Directory); err != nil {
			return err
		}
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

// probeLocationsIn lists every probe of the directory with where it sits. A
// probe installed several times appears once per location.
func probeLocationsIn(root string) ([]ProbeLocation, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("fail to read the probes directory %s : %s", root, err)
	}

	locations := make([]ProbeLocation, 0)
	scheduled := make(map[string]bool)

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

			scheduled[probe.Name()] = true
		}
	}

	// The disabled probes : the ones nothing links to. Wigo ships thirty and the
	// packaging enables half, so most of these were never turned off by anyone.
	// They are just as unmonitored either way, which is what the listing is for.
	installedProbes, err := os.ReadDir(filepath.Join(filepath.Clean(root), ExampleProbesDirectory))
	if err == nil {
		for _, probe := range installedProbes {
			if probe.IsDir() || scheduled[probe.Name()] || !IsValidProbeName(probe.Name()) {
				continue
			}

			locations = append(locations, ProbeLocation{
				Name:      probe.Name(),
				Directory: ExampleProbesDirectory,
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
	if err := scheduleProbeIn(probesRoot(), name, interval); err != nil {
		return err
	}

	PublishEvent(LiveEvent{Type: EventSchedule, Host: GetLocalWigo().GetHostname(), Probe: name})

	return nil
}

// UnscheduleProbe stops a probe from being scheduled at all.
func UnscheduleProbe(name string) error {
	if err := unscheduleProbeIn(probesRoot(), name); err != nil {
		return err
	}

	PublishEvent(LiveEvent{Type: EventSchedule, Host: GetLocalWigo().GetHostname(), Probe: name})

	return nil
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
