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

	. "github.com/akamai/cli-common-golang"
	"github.com/kardianos/osext"
	"github.com/urfave/cli"
)

func main() {
	os.Setenv("AKAMAI_CLI", "1")
	os.Setenv("AKAMAI_CLI_VERSION", VERSION)

	getCachePath()
	setupConfig()
	createApp()

	setupLogging()

	firstRun()
	checkUpgrade()
	checkPing()
	App.Run(os.Args)
}

func setupConfig() {
	migrateConfig()
	config, err := getConfig()
	if err != nil {
		fmt.Fprintf(App.ErrWriter, "Unable to read configuration file: %s\n", config.Path)
		cli.OsExiter(1)
		return
	}
	config.ExportEnv()
}

func defaultAction(c *cli.Context) {
	cmd, err := osext.Executable()
	if err != nil {
		cmd = self()
	}

	zshScript := `set -k
# To enable zsh auto-completion, run: eval "$(` + cmd + ` --zsh)"
# We recommend adding this to your .zshrc file
autoload -U compinit && compinit
autoload -U bashcompinit && bashcompinit`

	bashComments := `# To enable bash auto-completion, run: eval "$(` + cmd + ` --bash)"
# We recommend adding this to your .bashrc or .bash_profile file`

	bashScript := `_akamai_cli_bash_autocomplete() {
    local cur opts base
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    opts=$( ${COMP_WORDS[@]:0:$COMP_CWORD} --generate-auto-complete )
    COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
    return 0
}

complete -F _akamai_cli_bash_autocomplete ` + self()

	if c.Bool("bash") {
		fmt.Fprintln(App.Writer, bashComments)
		fmt.Fprintln(App.Writer, bashScript)
		return
	}

	if c.Bool("zsh") {
		fmt.Fprintln(App.Writer, zshScript)
		fmt.Fprintln(App.Writer, bashScript)
		return
	}

	cli.ShowAppHelpAndExit(c, 0)
}
