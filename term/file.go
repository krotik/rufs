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
	"path"
	"path/filepath"

	"devt.de/krotik/common/bitutil"
	"devt.de/krotik/rufs"
)

/*
cmdCat reads and prints the contents of a file.
*/
func cmdCat(tt *TreeTerm, arg ...string) (string, error) {
	err := fmt.Errorf("cat requires a file path")

	if len(arg) > 0 {
		err = tt.tree.ReadFileToBuffer(tt.parsePathParam(arg[0]), tt.out)
	}

	return "", err
}

/*
cmdGet Retrieve a file and store it locally (in the current directory).
*/
func cmdGet(tt *TreeTerm, arg ...string) (string, error) {
	var res string

	lenArg := len(arg)
	err := fmt.Errorf("get requires at least a source file path")

	if lenArg > 0 {
		var f *os.File

		src := tt.parsePathParam(arg[0])
		dst := src

		if lenArg > 1 {
			dst = tt.parsePathParam(arg[1])
		}

		// Make sure we only write files to the local folder

		_, dst = filepath.Split(dst)

		if f, err = os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0660); err == nil {
			defer f.Close()

			if err = tt.tree.ReadFileToBuffer(src, f); err == nil {
				res = fmt.Sprintf("Written file %s", dst)
			}
		}
	}

	return res, err
}

/*
cmdPut Read a local file and store it.
*/
func cmdPut(tt *TreeTerm, arg ...string) (string, error) {
	var res string

	lenArg := len(arg)
	err := fmt.Errorf("put requires a source and destination file path")

	if lenArg > 0 {
		var f *os.File

		src := arg[0]
		dst := tt.parsePathParam(arg[1])

		if f, err = os.Open(src); err == nil {
			defer f.Close()

			if err = tt.tree.WriteFileFromBuffer(dst, f); err == nil {
				res = fmt.Sprintf("Written file %s", dst)
			}
		}
	}

	return res, err
}

/*
cmdRm Delete a file or directory.
*/
func cmdRm(tt *TreeTerm, arg ...string) (string, error) {
	var res string

	lenArg := len(arg)
	err := fmt.Errorf("rm requires a file path")

	if lenArg > 0 {

		p := tt.parsePathParam(arg[0])
		dir, file := path.Split(p)

		if file == "" {

			// If a path is give just chop off the last slash and try again

			dir, file = path.Split(dir[:len(dir)-1])
		}

		_, err = tt.tree.ItemOp(dir, map[string]string{
			rufs.ItemOpAction: rufs.ItemOpActDelete,
			rufs.ItemOpName:   file,
		})
	}

	return res, err
}

/*
cmdRen Rename a file or directory.
*/
func cmdRen(tt *TreeTerm, arg ...string) (string, error) {
	var res string

	lenArg := len(arg)
	err := fmt.Errorf("ren requires a filename and a new filename")

	if lenArg > 1 {

		p := tt.parsePathParam(arg[0])
		p2 := tt.parsePathParam(arg[1])

		dir1, file1 := path.Split(p)
		dir2, file2 := path.Split(p2)

		if file2 == "" || dir2 != "/" {
			err = fmt.Errorf("new filename must not have a path")

		} else {

			if file1 == "" {

				// If a path is give just chop off the last slash and try again

				dir1, file1 = path.Split(dir1[:len(dir1)-1])
			}

			_, err = tt.tree.ItemOp(dir1, map[string]string{
				rufs.ItemOpAction:  rufs.ItemOpActRename,
				rufs.ItemOpName:    file1,
				rufs.ItemOpNewName: file2,
			})
		}
	}

	return res, err
}

/*
cmdMkDir Create a new direectory.
*/
func cmdMkDir(tt *TreeTerm, arg ...string) (string, error) {
	var res string

	lenArg := len(arg)
	err := fmt.Errorf("mkdir requires a directory path")

	if lenArg > 0 {

		p := tt.parsePathParam(arg[0])
		dir, newdir := path.Split(p)

		if newdir == "" {

			// If a path is given just chop off the last slash and try again

			dir, newdir = path.Split(dir[:len(dir)-1])
		}

		_, err = tt.tree.ItemOp(dir, map[string]string{
			rufs.ItemOpAction: rufs.ItemOpActMkDir,
			rufs.ItemOpName:   newdir,
		})
	}

	return res, err
}

/*
cmdCp Copy a file.
*/
func cmdCp(tt *TreeTerm, arg ...string) (string, error) {
	var res string

	lenArg := len(arg)
	err := fmt.Errorf("cp requires a source file or directory and a destination directory")

	if lenArg > 1 {

		src := tt.parsePathParam(arg[0])
		dst := tt.parsePathParam(arg[1])

		updFunc := func(file string, writtenBytes, totalBytes, currentFile, totalFiles int64) {

			if writtenBytes > 0 {
				tt.WriteStatus(fmt.Sprintf("Copy %v: %v / %v (%v of %v)", file,
					bitutil.ByteSizeString(writtenBytes, false),
					bitutil.ByteSizeString(totalBytes, false),
					currentFile, totalFiles))
			} else {
				tt.ClearStatus()
			}
		}

		if err = tt.tree.Copy([]string{src}, dst, updFunc); err == nil {
			res = "Done"
		}
	}

	return res, err
}
