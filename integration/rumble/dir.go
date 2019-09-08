/*
 * Rufs - Remote Union File System
 *
 * Copyright 2017 Matthias Ladkau. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the MIT
 * License, If a copy of the MIT License was not distributed with this
 * file, You can obtain one at https://opensource.org/licenses/MIT.
 */

/*
Package rumble contains Rumble functions which interface with Rufs.
*/
package rumble

import (
	"fmt"
	"os"
	"regexp"

	"devt.de/krotik/common/defs/rumble"
	"devt.de/krotik/common/stringutil"
	"devt.de/krotik/rufs/api"
)

// Function: dir
// =============

/*
DirFunc queries a directory in a tree.
*/
type DirFunc struct {
}

/*
Name returns the name of the function.
*/
func (f *DirFunc) Name() string {
	return "fs.dir"
}

/*
Validate is called for parameter validation and to reset the function state.
*/
func (f *DirFunc) Validate(argsNum int, rt rumble.Runtime) rumble.RuntimeError {
	var err rumble.RuntimeError

	if argsNum != 3 && argsNum != 4 {
		err = rt.NewRuntimeError(rumble.ErrInvalidConstruct,
			"Function dir requires 3 or 4 parameters: tree, a path, a glob expression and optionally a recursive flag")
	}

	return err
}

/*
Execute executes the rumble function.
*/
func (f *DirFunc) Execute(argsVal []interface{}, vars rumble.Variables,
	rt rumble.Runtime) (interface{}, rumble.RuntimeError) {

	var res interface{}
	var paths []string
	var fiList [][]os.FileInfo

	treeName := fmt.Sprint(argsVal[0])
	path := fmt.Sprint(argsVal[1])
	pattern := fmt.Sprint(argsVal[2])
	recursive := argsVal[3] == true

	conv := func(re *regexp.Regexp, fis []os.FileInfo) []interface{} {
		r := make([]interface{}, 0, len(fis))

		for _, fi := range fis {

			if !fi.IsDir() && !re.MatchString(fi.Name()) {
				continue
			}

			r = append(r, map[interface{}]interface{}{
				"name":    fi.Name(),
				"mode":    fmt.Sprint(fi.Mode()),
				"modtime": fmt.Sprint(fi.ModTime()),
				"isdir":   fi.IsDir(),
				"size":    fi.Size(),
			})
		}

		return r
	}

	tree, ok, err := api.GetTree(treeName)

	if !ok {
		if err == nil {
			err = fmt.Errorf("Unknown tree: %v", treeName)
		}
	}

	if err == nil {
		var globPattern string

		// Create regex for files

		if globPattern, err = stringutil.GlobToRegex(pattern); err == nil {
			var re *regexp.Regexp

			if re, err = regexp.Compile(globPattern); err == nil {

				// Query the file system

				paths, fiList, err = tree.Dir(path, "", recursive, false)

				pathData := make([]interface{}, 0, len(paths))
				fisData := make([]interface{}, 0, len(paths))

				// Convert the result into a Rumble data structure

				for i := range paths {
					fis := conv(re, fiList[i])

					// If we have a regex then only include directories which have files

					pathData = append(pathData, paths[i])
					fisData = append(fisData, fis)
				}

				res = []interface{}{pathData, fisData}
			}
		}
	}

	if err != nil {

		// Wrap error message in RuntimeError

		err = rt.NewRuntimeError(rumble.ErrInvalidState,
			fmt.Sprintf("Cannot list files: %v", err.Error()))
	}

	return res, err
}
