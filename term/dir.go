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
	"os"

	"devt.de/krotik/common/stringutil"
	"devt.de/krotik/rufs"
)

/*
cmdCd show or change the current directory.
*/
func cmdCd(tt *TreeTerm, arg ...string) (string, error) {
	if len(arg) > 0 {
		tt.cd = tt.parsePathParam(arg[0])
	}

	return fmt.Sprint(tt.cd, "\n"), nil
}

/*
cmdDir shows a directory listing.
*/
func cmdDir(tt *TreeTerm, arg ...string) (string, error) {
	return cmdDirListing(tt, false, false, arg...)
}

/*
cmdChecksum shows a directory listing and the checksums.
*/
func cmdChecksum(tt *TreeTerm, arg ...string) (string, error) {
	return cmdDirListing(tt, false, true, arg...)
}

/*
cmdTree shows the listing of a directory and its subdirectorie
*/
func cmdTree(tt *TreeTerm, arg ...string) (string, error) {
	return cmdDirListing(tt, true, false, arg...)
}

/*
cmdDirListing shows a directory listing and optional also its subdirectories.
*/
func cmdDirListing(tt *TreeTerm, recursive bool, checksum bool, arg ...string) (string, error) {
	var dirs []string
	var fis [][]os.FileInfo
	var err error
	var res, rex string

	dir := tt.cd

	if len(arg) > 0 {
		dir = tt.parsePathParam(arg[0])

		if len(arg) > 1 {
			rex, err = stringutil.GlobToRegex(arg[1])
		}
	}

	if err == nil {
		if dirs, fis, err = tt.tree.Dir(dir, rex, recursive, checksum); err == nil {
			res = rufs.DirResultToString(dirs, fis)
		}
	}

	return res, err
}
