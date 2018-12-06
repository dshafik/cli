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
	"os"
	"path/filepath"
	"strings"

	akamai "github.com/akamai/cli-common-golang"
	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	"github.com/urfave/cli"
	"gopkg.in/src-d/go-git.v4"
)

// TODO: add support for --save flag
func cmdInstall(c *cli.Context) error {
	if !c.Args().Present() {
		return install()
	}

	oldCmds := getCommands()

	// TODO: Split repos on @ to get versions

	for _, repo := range c.Args() {
		repo = githubize(repo)
		err := installPackage(repo, c.Bool("force"))
		if err != nil {
			// Only track public github repos
			if isPublicRepo(repo) {
				trackEvent("package.install", "failed", repo)
			}
			return err
		}

		if isPublicRepo(repo) {
			trackEvent("package.install", "success", repo)
		}
	}

	packageListDiff(oldCmds)

	return nil
}

func isPublicRepo(repo string) bool {
	return !strings.Contains(repo, ":") || strings.HasPrefix(repo, "https://github.com/")
}

// TODO: support version argument
func installPackage(repoURL string, forceBinary bool) error {
	srcPath, err := getSrcPath()
	if err != nil {
		return err
	}

	_ = os.MkdirAll(srcPath, 0700)

	akamai.StartSpinner(fmt.Sprintf("Attempting to fetch command from %s...", repoURL), fmt.Sprintf("Attempting to fetch command from %s...", repoURL)+"... ["+color.GreenString("OK")+"]\n")

	packagePath := getPackagePath(srcPath, repoURL)
	_, err = clonePackage(packagePath, repoURL, 1)
	if err != nil {
		akamai.StopSpinnerFail()
		if err == git.ErrRepositoryAlreadyExists {
			return cli.NewExitError(color.RedString("Package directory already exists (%s)", packagePath), 1)
		}
		os.RemoveAll(packagePath)
		return cli.NewExitError(color.RedString("Unable to clone repository: "+err.Error()), 1)
	}

	akamai.StopSpinnerOk()

	if strings.HasPrefix(repoURL, "https://github.com/akamai/cli-") != true && strings.HasPrefix(repoURL, "git@github.com:akamai/cli-") != true {
		fmt.Fprintln(akamai.App.ErrWriter, color.CyanString("Disclaimer: You are installing a third-party package, subject to its own terms and conditions. Akamai makes no warranty or representation with respect to the third-party package."))
	}

	if !installPackageDependencies(packagePath, forceBinary) {
		os.RemoveAll(packagePath)
		return cli.NewExitError("", 1)
	}

	return nil
}

// TODO: support installation of cli package dependencies
func installPackageDependencies(dir string, forceBinary bool) bool {
	akamai.StartSpinner("Installing...", "Installing...... ["+color.GreenString("OK")+"]\n")

	cmdPackage, err := readPackage(dir)

	if err != nil {
		akamai.StopSpinnerFail()
		fmt.Fprintln(akamai.App.ErrWriter, err.Error())
		return false
	}

	lang := determineCommandLanguage(cmdPackage)

	var success bool
	switch lang {
	case "php":
		success, err = installPHP(dir, cmdPackage)
	case "javascript":
		success, err = installJavaScript(dir, cmdPackage)
	case "ruby":
		success, err = installRuby(dir, cmdPackage)
	case "python":
		success, err = installPython(dir, cmdPackage)
	case "go":
		success, err = installGolang(dir, cmdPackage)
	default:
		akamai.StopSpinnerWarnOk()
		fmt.Fprintln(akamai.App.Writer, color.CyanString("Package installed successfully, however package type is unknown, and may or may not function correctly."))
		return true
	}

	if success && err == nil {
		akamai.StopSpinnerOk()
		return true
	}

	first := true
	for _, cmd := range cmdPackage.Commands {
		if cmd.Bin != "" {
			if first {
				first = false
				akamai.StopSpinnerWarn()
				fmt.Fprintln(akamai.App.Writer, color.CyanString(err.Error()))
				if !forceBinary {
					if !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd()) {
						return false
					}

					fmt.Fprint(akamai.App.ErrWriter, "Binary command(s) found, would you like to try download and install it? (Y/n): ")
					answer := ""
					fmt.Scanln(&answer)
					if answer != "" && strings.ToLower(answer) != "y" {
						return false
					}
				}

				os.MkdirAll(filepath.Join(dir, "bin"), 0700)
			}

			akamai.StartSpinner("Downloading binary...", "Downloading binary...... ["+color.GreenString("OK")+"]\n")
			if downloadBin(filepath.Join(dir, "bin"), cmd) {
				akamai.StopSpinnerOk()
				return true
			}

			akamai.StopSpinnerFail()
			fmt.Fprintln(akamai.App.ErrWriter, color.RedString("Unable to download binary: "+err.Error()))
			return false
		}

		if first {
			first = false
			akamai.StopSpinnerFail()
			fmt.Fprintln(akamai.App.ErrWriter, color.RedString(err.Error()))
			return false
		}
	}

	return true
}

// TODO: cleanup the args here, this should be more standalone
func clonePackage(dest string, repoURL string, depth int) (*git.Repository, error) {
	repo, err := git.PlainClone(dest, false, &git.CloneOptions{
		URL:      repoURL,
		Progress: nil,
		Depth:    depth,
	})

	return repo, err
}

func getPackagePath(dest string, repoURL string) string {
	dirName := strings.TrimSuffix(filepath.Base(repoURL), ".git")
	return filepath.Join(dest, dirName)
}

func install() error {
	akamai.StartSpinner("Installing packages...", "Installing packages...... ["+color.GreenString("OK")+"]\n")
	root, _ := os.Getwd()
	pkg, err := readPackage(root)
	if err != nil {
		akamai.StopSpinner("... ["+color.RedString("FAIL")+"]\n", true)
		return err
	}

	err = installProjectPackages(pkg)
	fmt.Printf("%#v\n", err)
	akamai.StopSpinner("... ["+color.CyanString("OK")+"]\n", true)

	return err
}

func importPath(packageName string) string {
	return strings.TrimPrefix(
		strings.TrimPrefix(
			strings.TrimPrefix(
				strings.TrimSuffix(
					githubize(packageName),
					".git",
				),
				"https://",
			),
			"http://",
		),
		"file://",
	)
}
