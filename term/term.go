/*
 * Rufs - Remote Union File System
 *
 * Copyright 2017 Matthias Ladkau. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the MIT
 * License, If a copy of the MIT License was not distributed with this
 * file, You can obtain one at https://opensource.org/licenses/MIT.
 */

package term

import (
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"devt.de/krotik/common/stringutil"
	"devt.de/krotik/rufs"
)

/*
TreeTerm models a command processor for Rufs trees.
*/
type TreeTerm struct {
	tree       *rufs.Tree // Tree which we operate on
	cd         string     // Current directory
	out        io.Writer  // Output writer
	lastStatus string     // Last status line
}

/*
NewTreeTerm returns a new command processor for Rufs trees.
*/
func NewTreeTerm(t *rufs.Tree, out io.Writer) *TreeTerm {
	return &TreeTerm{t, "/", out, ""}
}

/*
WriteStatus writes a status line to the output writer.
*/
func (tt *TreeTerm) WriteStatus(line string) {
	fmt.Fprint(tt.out, "\r")
	fmt.Fprint(tt.out, line)

	ll := len(tt.lastStatus)
	lc := len(line)

	if ll > lc {
		fmt.Fprint(tt.out, stringutil.GenerateRollingString(" ", ll-lc))
	}

	tt.lastStatus = line
}

/*
ClearStatus removes the last status line and returns the cursor to the initial position.
*/
func (tt *TreeTerm) ClearStatus() {
	if tt.lastStatus != "" {
		toClear := utf8.RuneCountInString(tt.lastStatus)
		fmt.Fprint(tt.out, "\r")
		fmt.Fprint(tt.out, stringutil.GenerateRollingString(" ", toClear))
		fmt.Fprint(tt.out, "\r")
	}
}

/*
CurrentDir returns the current directory of this TreeTerm.
*/
func (tt *TreeTerm) CurrentDir() string {
	return tt.cd
}

/*
AddCmd adds a new command to the terminal
*/
func (tt *TreeTerm) AddCmd(cmd, helpusage, help string,
	cmdFunc func(*TreeTerm, ...string) (string, error)) {

	cmdMap[cmd] = cmdFunc
	helpMap[helpusage] = help
}

/*
Cmds returns a list of available terminal commands.
*/
func (tt *TreeTerm) Cmds() []string {
	var cmds []string

	for k := range cmdMap {
		cmds = append(cmds, k)
	}

	sort.Strings(cmds)

	return cmds
}

/*
Run executes a given command line. And return its output as a string. File
output and other streams to the console are written to the output writer.
*/
func (tt *TreeTerm) Run(line string) (string, error) {
	var err error
	var res string
	var arg []string

	// Parse the input

	c := strings.Split(line, " ")

	cmd := c[0]

	if len(c) > 1 {
		arg = c[1:]
	}

	// Execute the given command

	if f, ok := cmdMap[cmd]; ok {
		res, err = f(tt, arg...)
	} else {
		err = fmt.Errorf("Unknown command: %s", cmd)
	}

	return res, err
}

/*
cmdPing pings a remote branch.
*/
func cmdPing(tt *TreeTerm, arg ...string) (string, error) {
	var res string

	err := fmt.Errorf("ping requires at least a branch name")

	if len(arg) > 0 {
		var fp, rpc string

		if len(arg) > 1 {
			rpc = arg[1]
		}

		if fp, err = tt.tree.PingBranch(arg[0], rpc); err == nil {
			res = fmt.Sprint("Response ok - fingerprint: ", fp, "\n")
		}
	}

	return res, err
}

/*
cmdRefresh refreshes all known branches and connects depending on if the
branches are reachable.
*/
func cmdRefresh(tt *TreeTerm, arg ...string) (string, error) {
	tt.tree.Refresh()

	return "Done", nil
}

/*
parsePathParam parse a given path parameter and return an absolute path.
*/
func (tt *TreeTerm) parsePathParam(p string) string {
	if !strings.HasPrefix(p, "/") {
		p = path.Join(tt.cd, p) // Take care of relative paths
	}
	return p
}
