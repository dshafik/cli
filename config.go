// Copyright 2018. Akamai Technologies, Inc
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/akamai/cli-common-golang"
	conf "github.com/akamai/cli-common-golang/config"
)

const (
	configVersion string = "1.1"
)

func getConfig() (*conf.Config, error) {
	cliPath, err := getCliHome()
	if err != nil {
		return nil, err
	}

	global, err := conf.NewConfig(filepath.Join(cliPath, "config"))
	if err != nil {
		return nil, err
	}

	return global, nil
}

func migrateConfig() {
	conf, err := getConfig()
	if err != nil {
		fmt.Fprintln(App.ErrWriter, err.Error())
		return
	}

	var currentVersion string
	// Do we need to migrate from an older version?
	currentVersion = conf.Get("cli", "config-version")
	if currentVersion == configVersion {
		return
	}

	switch currentVersion {
	case "":
		// Create v1
		cliPath, _ := getCliHome()

		var data []byte
		upgradeFile := filepath.Join(cliPath, ".upgrade-check")
		if _, err := os.Stat(upgradeFile); err == nil {
			data, _ = ioutil.ReadFile(upgradeFile)
		} else {
			upgradeFile = filepath.Join(cliPath, ".update-check")
			if _, err := os.Stat(upgradeFile); err == nil {
				data, _ = ioutil.ReadFile(upgradeFile)
			}
		}

		if len(data) != 0 {
			date := string(data)
			if date == "never" || date == "ignore" {
				conf.Set("cli", "last-upgrade-check", date)
			} else {
				if m := strings.LastIndex(date, "m="); m != -1 {
					date = date[0 : m-1]
				}
				lastUpgrade, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", date)
				if err == nil {
					conf.Set("cli", "last-upgrade-check", lastUpgrade.Format(time.RFC3339))
				}
			}

			os.Remove(upgradeFile)
		}

		conf.Set("cli", "config-version", "1")
	case "1":
		// Upgrade to v1.1
		if conf.Get("cli", "enable-cli-statistics") == "true" {
			conf.Set("cli", "stats-version", "1.0")
		}
		conf.Set("cli", "config-version", "1.1")
	}

	conf.Save()
	migrateConfig()
}
