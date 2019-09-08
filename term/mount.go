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
	"bytes"
	"fmt"
)

/*
cmdReset removes all present mount points or branches.
*/
func cmdReset(tt *TreeTerm, arg ...string) (string, error) {

	if len(arg) > 0 {
		if arg[0] == "mounts" {
			tt.tree.Reset(false)
			return "Resetting all mounts\n", nil
		} else if arg[0] == "branches" {
			tt.tree.Reset(true)
			return "Resetting all branches and mounts\n", nil
		}
	}

	return "", fmt.Errorf("Can either reset all [mounts] or all [branches] which includes all mount points")
}

/*
cmdBranch lists all known branches or adds a new branch to the tree.
*/
func cmdBranch(tt *TreeTerm, arg ...string) (string, error) {
	var err error
	var res bytes.Buffer

	writeKnownBranches := func() {
		braches, fps := tt.tree.ActiveBranches()
		for i, b := range braches {
			res.WriteString(fmt.Sprintf("%v [%v]\n", b, fps[i]))
		}
	}

	if len(arg) == 0 {
		writeKnownBranches()

	} else if len(arg) > 1 {
		var fp = ""

		branchName := arg[0]
		branchRPC := arg[1]

		if len(arg) > 2 {
			fp = arg[2]
		}

		err = tt.tree.AddBranch(branchName, branchRPC, fp)

		writeKnownBranches()

	} else {
		err = fmt.Errorf("branch requires either no or at least 2 parameters")
	}

	return res.String(), err
}

/*
cmdMount lists all mount points or adds a new mount point to the tree.
*/
func cmdMount(tt *TreeTerm, arg ...string) (string, error) {
	var err error
	var res bytes.Buffer

	if len(arg) == 0 {
		res.WriteString(tt.tree.String())

	} else if len(arg) > 1 {
		dir := arg[0]
		branchName := arg[1]
		writable := !(len(arg) > 2 && arg[2] == "ro") // Writeable unless stated otherwise

		if err = tt.tree.AddMapping(dir, branchName, writable); err == nil {
			res.WriteString(tt.tree.String())
		}

	} else {
		err = fmt.Errorf("mount requires either 2 or no parameters")
	}

	return res.String(), err
}
