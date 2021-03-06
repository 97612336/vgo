// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package modfetch

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"cmd/go/internal/modconv"
	"cmd/go/internal/modfile"
	"cmd/go/internal/module"
	"cmd/go/internal/par"
	"cmd/go/internal/semver"
)

// ConvertLegacyConfig converts legacy config to modfile.
// The file argument is slash-delimited.
func ConvertLegacyConfig(f *modfile.File, file string, data []byte) error {
	i := strings.LastIndex(file, "/")
	j := -2
	if i >= 0 {
		j = strings.LastIndex(file[:i], "/")
	}
	convert := modconv.Converters[file[i+1:]]
	if convert == nil && j != -2 {
		convert = modconv.Converters[file[j+1:]]
	}
	if convert == nil {
		return fmt.Errorf("unknown legacy config file %s", file)
	}
	require, err := convert(file, data)
	if err != nil {
		return fmt.Errorf("parsing %s: %v", file, err)
	}

	// Convert requirements block, which may use raw SHA1 hashes as versions,
	// to valid semver requirement list, respecting major versions.
	var work par.Work
	for _, r := range require {
		if r.Path == "" {
			continue
		}

		// TODO: Something better here.
		if strings.HasPrefix(r.Path, "github.com/") || strings.HasPrefix(r.Path, "golang.org/x/") {
			f := strings.Split(r.Path, "/")
			if len(f) > 3 {
				r.Path = strings.Join(f[:3], "/")
			}
		}
		work.Add(r)
	}

	var (
		mu   sync.Mutex
		need = make(map[string]string)
	)
	work.Do(10, func(item interface{}) {
		r := item.(module.Version)
		info, err := Stat(r.Path, r.Version)
		if err != nil {
			fmt.Fprintf(os.Stderr, "vgo: stat %s@%s: %v\n", r.Path, r.Version, err)
			return
		}
		mu.Lock()
		need[r.Path] = semver.Max(need[r.Path], info.Version)
		mu.Unlock()
	})

	var paths []string
	for path := range need {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		f.AddNewRequire(path, need[path])
	}

	return nil
}
