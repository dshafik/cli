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
	"os"
	"path/filepath"
	"strings"
	"time"

	akamai "github.com/akamai/cli-common-golang"
	"github.com/urfave/cli"
)

const (
	// VERSION Application Version
	VERSION = "1.1.0"
)

func getCachePath() (cachePath string, err error) {
	config, err := getConfig()
	if err != nil {
		return
	}

	if cachePath = config.Get("cli", "cache-path"); cachePath != "" {
		return
	}

	cliHome, _ := getCliHome()

	cachePath = filepath.Join(cliHome, "cache")
	err = os.MkdirAll(cachePath, 0700)
	if err != nil {
		return
	}
	config.Set("cli", "cache-path", cachePath)
	config.Save()

	return
}

func createApp() {
	akamai.CreateApp("", "Akamai CLI", "", VERSION, "", commandLocator)

	akamai.App.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "bash",
			Usage: "Output bash auto-complete",
		},
		cli.BoolFlag{
			Name:  "zsh",
			Usage: "Output zsh auto-complete",
		},
		cli.StringFlag{
			Name:  "proxy",
			Usage: "Set a proxy to use",
		},
		cli.BoolFlag{
			Name:   "daemon",
			Usage:  "Keep Akamai CLI running in the background, particularly useful for Docker containers",
			Hidden: true,
			EnvVar: "AKAMAI_CLI_DAEMON",
		},
	}

	akamai.App.Action = func(c *cli.Context) {
		defaultAction(c)
	}

	akamai.App.Before = func(c *cli.Context) error {
		if c.IsSet("proxy") {
			proxy := c.String("proxy")
			os.Setenv("HTTP_PROXY", proxy)
			os.Setenv("http_proxy", proxy)
			if strings.HasPrefix(proxy, "https") {
				os.Setenv("HTTPS_PROXY", proxy)
				os.Setenv("https_proxy", proxy)
			}
		}

		if c.IsSet("daemon") {
			for {
				time.Sleep(time.Hour * 24)
			}
		}
		return nil
	}
}

func checkUpgrade() {
	if latestVersion := checkUpgradeVersion(false); latestVersion != "" {
		if upgradeCli(latestVersion) {
			trackEvent("upgrade.auto", "success", "to: "+latestVersion+" from: "+VERSION)
			return
		}
		trackEvent("upgrade.auto", "failed", "to: "+latestVersion+" from: "+VERSION)
	}
}
