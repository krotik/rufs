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

	"devt.de/krotik/common/bitutil"
)

/*
cmdSync Make sure dst has the same files and directories as src.
*/
func cmdSync(tt *TreeTerm, arg ...string) (string, error) {
	var res string

	lenArg := len(arg)
	err := fmt.Errorf("sync requires a source and a destination directory")

	if lenArg > 1 {

		src := tt.parsePathParam(arg[0])
		dst := tt.parsePathParam(arg[1])

		updFunc := func(op, srcFile, dstFile string, writtenBytes, totalBytes, currentFile, totalFiles int64) {

			if writtenBytes > 0 {
				tt.WriteStatus(fmt.Sprintf("%v (%v/%v) writing: %v -> %v %v / %v", op,
					currentFile, totalFiles, srcFile, dstFile,
					bitutil.ByteSizeString(writtenBytes, false),
					bitutil.ByteSizeString(totalBytes, false)))
			} else {
				tt.ClearStatus()
				fmt.Fprint(tt.out, fmt.Sprintln(fmt.Sprintf("%v (%v/%v) %v -> %v", op,
					currentFile, totalFiles, srcFile, dstFile)))
			}
		}

		if err = tt.tree.Sync(src, dst, true, updFunc); err == nil {
			res = "Done"
		}
	}

	return res, err
}
