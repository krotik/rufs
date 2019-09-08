/*
 * Rufs - Remote Union File System
 *
 * Copyright 2017 Matthias Ladkau. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the MIT
 * License, If a copy of the MIT License was not distributed with this
 * file, You can obtain one at https://opensource.org/licenses/MIT.
 */

package rufs

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

/*
Special unit test flag - use a common mode to gloss over OS specific
defaults
*/
var unitTestModes = false

/*
FileInfo implements os.FileInfo in an platform-agnostic way
*/
type FileInfo struct {
	FiName     string      // Base name
	FiSize     int64       // Size in bytes
	FiMode     os.FileMode // File mode bits
	FiModTime  time.Time   // Modification time
	FiChecksum string      // Checksum of files

	// Private fields which will not be transferred via RPC

	isSymLink     bool   // Flag if this is a symlink (unix)
	symLinkTarget string // Target file/directory of the symlink
}

/*
WrapFileInfo wraps a single os.FileInfo object in a serializable FileInfo.
*/
func WrapFileInfo(path string, i os.FileInfo) os.FileInfo {
	var realPath string

	// Check if we have a symlink

	mode := i.Mode()
	size := i.Size()

	isSymlink := i.Mode()&os.ModeSymlink != 0

	if isSymlink {
		var err error

		if realPath, err = filepath.EvalSymlinks(filepath.Join(path, i.Name())); err == nil {
			var ri os.FileInfo
			if ri, err = os.Stat(realPath); err == nil {

				// Write in the size of the target and file mode

				mode = ri.Mode()
				size = ri.Size()
			}
		}
	}

	// Unit test fixed file modes

	if unitTestModes {
		mode = mode & os.ModeDir

		if mode.IsDir() {
			mode = mode | 0777
			size = 4096
		} else {
			mode = mode | 0666
		}
	}

	return &FileInfo{i.Name(), size, mode, i.ModTime(), "",
		isSymlink, realPath}
}

/*
WrapFileInfos wraps a list of os.FileInfo objects into a list of
serializable FileInfo objects. This function will modify the given
list.
*/
func WrapFileInfos(path string, is []os.FileInfo) []os.FileInfo {
	for i, info := range is {
		is[i] = WrapFileInfo(path, info)
	}
	return is
}

/*
Name returns the base name.
*/
func (rfi *FileInfo) Name() string {
	return rfi.FiName
}

/*
Size returns the length in bytes.
*/
func (rfi *FileInfo) Size() int64 {
	return rfi.FiSize
}

/*
Mode returns the file mode bits.
*/
func (rfi *FileInfo) Mode() os.FileMode {
	return rfi.FiMode
}

/*
ModTime returns the modification time.
*/
func (rfi *FileInfo) ModTime() time.Time {
	return rfi.FiModTime
}

/*
Checksum returns the checksum of this file. May be an empty string if it was
not calculated.
*/
func (rfi *FileInfo) Checksum() string {
	return rfi.FiChecksum
}

/*
IsDir returns if this is a directory.
*/
func (rfi *FileInfo) IsDir() bool {
	return rfi.FiMode.IsDir()
}

/*
Sys should return the underlying data source but will always return nil
for FileInfo nodes.
*/
func (rfi *FileInfo) Sys() interface{} {
	return nil
}

func (rfi *FileInfo) String() string {
	sum := rfi.Checksum()
	if sum != "" {
		return fmt.Sprintf("%v %s [%v] %v (%v) - %v", rfi.Name(), sum, rfi.Size(),
			rfi.Mode(), rfi.ModTime(), rfi.Sys())
	}
	return fmt.Sprintf("%v [%v] %v (%v) - %v", rfi.Name(), rfi.Size(),
		rfi.Mode(), rfi.ModTime(), rfi.Sys())
}
