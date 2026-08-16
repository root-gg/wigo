package wigo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStatusToString(t *testing.T) {

	tests := []struct {
		status   int
		expected string
	}{
		{0, "OK"},
		{100, "OK"},
		{101, "INFO"},
		{199, "INFO"},
		{200, "WARN"},
		{299, "WARN"},
		{300, "CRIT"},
		{499, "CRIT"},
		{500, "ERROR"},
		{999, "ERROR"},
	}

	for _, test := range tests {
		if got := StatusToString(test.status); got != test.expected {
			t.Errorf("StatusToString(%d) = %s, expected %s", test.status, got, test.expected)
		}
	}
}

func TestIsStringInArray(t *testing.T) {

	list := []string{"databases", "frontend"}

	if !IsStringInArray("frontend", list) {
		t.Errorf("frontend should be found in %v", list)
	}
	if IsStringInArray("backend", list) {
		t.Errorf("backend should not be found in %v", list)
	}
	// The comparison is exact, no case folding here
	if IsStringInArray("Frontend", list) {
		t.Errorf("the lookup should be case sensitive")
	}
	if IsStringInArray("frontend", nil) {
		t.Errorf("nothing can be found in an empty list")
	}
}

func TestToJson(t *testing.T) {

	probe := &ProbeResult{Name: "load", Status: 300, Message: "load too high"}

	decoded := new(ProbeResult)
	if err := json.Unmarshal([]byte(ToJson(probe)), decoded); err != nil {
		t.Fatalf("ToJson did not produce valid json : %s", err)
	}

	if decoded.Name != probe.Name || decoded.Status != probe.Status || decoded.Message != probe.Message {
		t.Errorf("Round trip gave %+v, expected %+v", decoded, probe)
	}
}

func TestListProbesInDirectory(t *testing.T) {

	directory := t.TempDir()

	if err := os.WriteFile(filepath.Join(directory, "probe.sh"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("Fail to write the test probe : %s", err)
	}
	// Files are listed whatever their permissions, only directories are skipped
	if err := os.WriteFile(filepath.Join(directory, "notes.txt"), []byte("hello"), 0644); err != nil {
		t.Fatalf("Fail to write the test file : %s", err)
	}
	if err := os.Mkdir(filepath.Join(directory, "subdirectory"), 0755); err != nil {
		t.Fatalf("Fail to create the test subdirectory : %s", err)
	}

	probes, err := ListProbesInDirectory(directory)
	if err != nil {
		t.Fatalf("ListProbesInDirectory() returned an error : %s", err)
	}

	found := make([]string, 0)
	for e := probes.Front(); e != nil; e = e.Next() {
		found = append(found, e.Value.(string))
	}

	if len(found) != 2 {
		t.Fatalf("Got %v, expected the two files and not the subdirectory", found)
	}
	if !IsStringInArray("probe.sh", found) || !IsStringInArray("notes.txt", found) {
		t.Errorf("Got %v, expected probe.sh and notes.txt", found)
	}
}

func TestListProbesInMissingDirectory(t *testing.T) {

	probes, err := ListProbesInDirectory(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Errorf("Expected an error for a missing directory")
	}
	if probes != nil {
		t.Errorf("Expected no probe list for a missing directory")
	}
}

func TestListProbesDirectories(t *testing.T) {

	wigo := setupTestWigo(t, "databases")

	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "60"), 0755); err != nil {
		t.Fatalf("Fail to create the test subdirectory : %s", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "README"), []byte("hello"), 0644); err != nil {
		t.Fatalf("Fail to write the test file : %s", err)
	}
	wigo.GetConfig().Global.ProbesDirectory = directory

	directories, err := ListProbesDirectories()
	if err != nil {
		t.Fatalf("ListProbesDirectories() returned an error : %s", err)
	}

	// Only subdirectories are returned, they are the probe intervals
	if len(directories) != 1 || directories[0] != "60" {
		t.Errorf("Got %v, expected only the 60 subdirectory", directories)
	}
}
