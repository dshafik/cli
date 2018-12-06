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
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"

	"github.com/urfave/cli"
)

type commandPackage struct {
	Name             string            `json:"name"`
	Version          string            `json:"version"`
	Commands         []command         `json:"commands"`
	Requirements     map[string]string `json:"requirements"`
	MinimumStability string            `json:"minimum-stability"`

	action interface{}
}

func (c *commandPackage) GetMinimumStability() int {
	switch strings.ToLower(c.MinimumStability) {
	case "rc":
		return ReleaseTypeRc
	case "beta":
		return ReleaseTypeBeta
	case "alpha":
		return ReleaseTypeAlpha
	case "dev":
		return ReleaseTypeDev
	default:
		return ReleaseTypeStable
	}
}

func readPackage(dir string) (commandPackage, error) {
	if _, err := os.Stat(filepath.Join(dir, "cli.json")); err != nil {
		dir = filepath.Dir(dir)
		if _, err = os.Stat(filepath.Join(dir, "cli.json")); err != nil {
			return commandPackage{}, cli.NewExitError(ErrPackageNotFound, 1)
		}
	}

	var packageData commandPackage
	cliJSON, err := ioutil.ReadFile(filepath.Join(dir, "cli.json"))
	if err != nil {
		return commandPackage{}, err
	}

	err = json.Unmarshal(cliJSON, &packageData)
	if err != nil {
		return commandPackage{}, err
	}

	for key := range packageData.Commands {
		packageData.Commands[key].Name = strings.ToLower(packageData.Commands[key].Name)
	}

	return packageData, nil
}

func getPackagePaths() []string {
	akamaiCliPath, err := getSrcPath()
	if err == nil && akamaiCliPath != "" {
		paths, _ := filepath.Glob(filepath.Join(akamaiCliPath, "*"))
		if len(paths) > 0 {
			return paths
		}
	}

	return []string{}
}

func getPackageBinPaths() string {
	path := ""
	akamaiCliPath, err := getSrcPath()
	if err == nil && akamaiCliPath != "" {
		paths, _ := filepath.Glob(filepath.Join(akamaiCliPath, "*"))
		if len(paths) > 0 {
			path += strings.Join(paths, string(os.PathListSeparator))
		}
		paths, _ = filepath.Glob(filepath.Join(akamaiCliPath, "*", "bin"))
		if len(paths) > 0 {
			path += string(os.PathListSeparator) + strings.Join(paths, string(os.PathListSeparator))
		}
	}

	return path
}

func findPackageDir(dir string) string {
	if stat, err := os.Stat(dir); err == nil && stat != nil && !stat.IsDir() {
		dir = filepath.Dir(dir)
	}

	if _, err := os.Stat(filepath.Join(dir, "cli.json")); err != nil {
		if os.IsNotExist(err) {
			if filepath.Dir(dir) == "" {
				return ""
			}

			return findPackageDir(filepath.Dir(dir))
		}
	}

	return dir
}

func determineCommandLanguage(cmdPackage commandPackage) string {
	if cmdPackage.Requirements["php"] != "" {
		return "php"
	}

	if cmdPackage.Requirements["node"] != "" {
		return "javascript"
	}

	if cmdPackage.Requirements["ruby"] != "" {
		return "ruby"
	}

	if cmdPackage.Requirements["go"] != "" {
		return "go"
	}

	if cmdPackage.Requirements["python"] != "" {
		return "python"
	}

	return ""
}

func downloadBin(dir string, cmd command) bool {
	cmd.Arch = runtime.GOARCH

	cmd.OS = runtime.GOOS
	if runtime.GOOS == "darwin" {
		cmd.OS = "mac"
	}

	if runtime.GOOS == "windows" {
		cmd.BinSuffix = ".exe"
	}

	t := template.Must(template.New("url").Parse(cmd.Bin))
	buf := &bytes.Buffer{}
	if err := t.Execute(buf, cmd); err != nil {
		return false
	}

	url := buf.String()

	bin, err := os.Create(filepath.Join(dir, "akamai-"+strings.ToLower(cmd.Name)+cmd.BinSuffix))
	bin.Chmod(0775)
	if err != nil {
		return false
	}
	defer bin.Close()

	res, err := http.Get(url)
	if err != nil {
		return false
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return false
	}

	n, err := io.Copy(bin, res.Body)
	if err != nil || n == 0 {
		return false
	}

	return true
}
