package wigo

import (
	"container/list"
	"encoding/json"
	"fmt"
	"io/ioutil"
)

// List probes in directory
func ListProbesInDirectory(directory string) (probesList *list.List, error error) {

	probesList = new(list.List)

	// List checks directory
	files, err := ioutil.ReadDir(directory)
	if err != nil {
		return nil, err
	}

	// Return only executables files
	for _, f := range files {
		if !f.IsDir() {
			probesList.PushBack(f.Name())
		}
	}

	return probesList, nil
}

// List probes directories
func ListProbesDirectories() ([]string, error) {

	// List checks directory
	files, err := ioutil.ReadDir(GetLocalWigo().GetConfig().Global.ProbesDirectory)
	if err != nil {
		return nil, err
	}

	// Init array
	subdirectories := make([]string, 0)

	// Return only subdirectories
	for _, f := range files {
		if f.IsDir() {
			subdirectories = append(subdirectories, f.Name())
		}
	}

	return subdirectories, nil
}

// Misc
func Dump(data interface{}) {
	json, _ := json.MarshalIndent(data, "", "   ")
	fmt.Printf("%s\n", string(json))
}
