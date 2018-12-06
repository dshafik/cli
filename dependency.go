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
	"errors"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/Masterminds/semver"
	log "github.com/sirupsen/logrus"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
)

const (
	ReleaseTypeDev = iota + 1
	ReleaseTypeAlpha
	ReleaseTypeBeta
	ReleaseTypeRc
	ReleaseTypeStable

	MaxDepth = 6
)

var (
	ReleaseTypes = map[int]string{
		ReleaseTypeDev:    "dev",
		ReleaseTypeAlpha:  "alpha",
		ReleaseTypeBeta:   "beta",
		ReleaseTypeRc:     "rc",
		ReleaseTypeStable: "",
	}
)

type version struct {
	semver    semver.Version
	name      string
	canonical uint64
	stability int
}

type versionList []version
type pkgDep struct {
	versions    versionList
	constraints []string
}
type deps map[string]pkgDep

var depth = 0
var dependencies = make(deps)

func installProjectPackages(pkg commandPackage) error {
	err := getDependenciesRecursive(pkg)
	if err != nil {
		return err
	}
	basePath, err := getProjectRoot()
	if err != nil {
		return err
	}
	basePath = filepath.Join(basePath, ".akamai-cli")

	err = os.RemoveAll(basePath)
	if err != nil {
		return err
	}
	_ = os.MkdirAll(basePath, 0755)
	for packageDir, requirement := range dependencies {
		installPath := filepath.Join(basePath, filepath.Base(packageDir))
		_, err := git.PlainClone(installPath, false, &git.CloneOptions{
			URL:        "file://" + packageDir,
			Depth:      0,
			RemoteName: "origin",
			NoCheckout: true,
		})
		if err != nil {
			return err
		}

		version, err := getLatestVersionForConstraints(requirement.constraints, requirement.versions, pkg.GetMinimumStability())
		if err != nil {
			return err
		}
		err = checkoutRef(version.name, installPath)
		return err
	}

	return nil
}

func getDependenciesRecursive(pkg commandPackage) error {
	log.Tracef("Fetching dependencies for %s", pkg.Name)
	depth++
	if depth > MaxDepth {
		return errors.New("dependency tree too deep, unable to resolve")
	}

	dest, err := getPackageCachePath()
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	var errChan = make(chan error)
	for dep, constraint := range pkg.Requirements {
		if isRuntime(dep) {
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			repoURL := githubize(dep)
			dirName := strings.TrimSuffix(filepath.Base(repoURL), ".git")
			packageDir := filepath.Join(dest, dirName)

			var versions versionList
			if _, ok := dependencies[packageDir]; !ok {
				versions, err = getPackageVersions(repoURL)
				if err != nil {
					errChan <- err
					return
				}
			} else {
				versions = dependencies[packageDir].versions
			}

			addToDependencies(packageDir, versions, constraint)

			var ref string
			version, err := getLatestVersionForConstraints([]string{constraint}, versions, pkg.GetMinimumStability())
			if err != nil {
				ref = constraint
			} else {
				ref = version.name
			}
			if err := checkoutRef(ref, packageDir); err != nil {
				errChan <- err
				return
			}

			pkg, err = readPackage(packageDir)
			if err != nil {
				errChan <- err
				return
			}
			err = getDependenciesRecursive(pkg)
			if err != nil {
				errChan <- err
				return
			}
		}()

		wg.Wait()

		select {
		case err, ok := <-errChan:
			if ok {
				return err
			}
		default:
			continue
		}
	}

	return err
}

func addToDependencies(packageDir string, versions versionList, constraint string) {
	var dep pkgDep
	var ok bool
	if dep, ok = dependencies[packageDir]; !ok {
		dep = pkgDep{}
	}
	dep.versions = versions
	dep.constraints = append(dependencies[packageDir].constraints, constraint)
	dependencies[packageDir] = dep
}

func checkoutRef(ref string, packageDir string) (err error) {
	repo, err := git.PlainOpen(packageDir)
	if err != nil {
		return
	}
	wt, err := repo.Worktree()
	if err != nil {
		return
	}

	err = wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewTagReferenceName(ref),
		Force:  true,
	})
	if err != nil {
		err = wt.Checkout(&git.CheckoutOptions{
			Branch: plumbing.NewBranchReferenceName(ref),
			Force:  true,
		})

		if err != nil {
			err = wt.Checkout(&git.CheckoutOptions{
				Hash:  plumbing.NewHash(ref),
				Force: true,
			})

			if err != nil {
				return
			}
		}
	}

	return nil
}

func isRuntime(val string) bool {
	switch strings.ToLower(val) {
	case "python":
		return true
	case "go":
		return true
	case "node":
		return true
	case "ruby":
		return true
	case "php":
		return true
	}

	return false
}

func getLatestVersionForConstraints(versionConstraints []string, versions versionList, stability int) (ver version, err error) {
	var constraints []semver.Constraint
	var c semver.Constraint
	for _, constraint := range versionConstraints {
		// Add implicit stability if not specified
		if stability < ReleaseTypeStable && !strings.Contains(constraint, "-") {
			constraint += "-" + ReleaseTypes[stability]
		}
		c, err = semver.NewConstraint(constraint)
		if err != nil {
			return
		}
		constraints = append(constraints, c)
	}

	// Check to see if we have mutually exclusive constraints
	for i, c1 := range constraints {
		for j, c2 := range constraints {
			if i != j && !c1.MatchesAny(c2) {
				return ver, NewExitErrorf(1, "Mutually exclusive constraints (%s and %s) found.", c1, c2)
			}

			// Versions are sorted newest to oldest, so return the first one
			for _, version := range versions {
				if version.stability >= stability {
					if err := c1.Matches(version.semver); err == nil {
						return version, nil
					} else {
						continue
					}
				}
			}
		}
	}

	return ver, errors.New("No version found")
}

func getPackageVersions(repoURL string) (versions versionList, err error) {
	var tags []string
	cachePath, err := getCachePath()
	if err != nil {
		return
	}

	dest := path.Join(cachePath, "package-cache")
	packagePath := getPackagePath(dest, repoURL)

	var repo *git.Repository
	if _, err = os.Stat(packagePath); err != nil {
		repo, err = clonePackage(packagePath, repoURL, 0)
		if err != nil {
			return
		}
	} else {
		repo, err = git.PlainOpen(packagePath)
		err = repo.Fetch(
			&git.FetchOptions{
				Tags:  git.AllTags,
				Depth: 0,
				Force: true,
			},
		)
		if err != nil && err != git.NoErrAlreadyUpToDate {
			return
		}
	}

	tagRefs, err := repo.Tags()
	if err != nil {
		return
	}

	err = tagRefs.ForEach(func(t *plumbing.Reference) error {
		tags = append(tags, t.Name().Short())
		return nil
	})

	for _, tag := range tags {
		var prerelease, v, stability int

		ver, err := semver.NewVersion(tag)
		if err != nil {
			continue
		}

		parts := strings.Split(ver.Prerelease(), ".")
		if len(parts) > 1 {
			v, _ = strconv.Atoi(parts[1])
		}

		switch strings.ToLower(parts[0]) {
		case "":
			stability = ReleaseTypeStable
		case "alpha":
			stability = ReleaseTypeAlpha
		case "beta":
			stability = ReleaseTypeBeta
		case "rc":
			stability = ReleaseTypeRc
		default:
			stability = ReleaseTypeDev
		}

		prerelease = (stability * 10) + v
		versions = append(
			versions,
			version{
				name: ver.String(),
				canonical: (ver.Major() * 100000) +
					(ver.Minor() * 10000) +
					(ver.Patch() * 1000) +
					uint64(prerelease),
				semver:    ver,
				stability: stability,
			},
		)
	}

	sort.Slice(versions, func(left, right int) bool { return versions[left].canonical > versions[right].canonical })
	return versions, err
}
