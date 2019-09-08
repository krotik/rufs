// +build linux

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
Package export contains export bindings for Rufs.
*/
package export

/*
This file contains Rufs bindings for FUSE (Filesystem in Userspace) enabling
a user to operate on Rufs as if it was a local file system.

This uses GO-FUSE: https://github.com/hanwen/go-fuse
Distributed under the New BSD License
Copyright (c) 2010 the Go-FUSE Authors. All rights reserved.
*/

import (
	"log"
	"os"
	"path"
	"path/filepath"

	"devt.de/krotik/rufs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/fuse/nodefs"
	"github.com/hanwen/go-fuse/v2/fuse/pathfs"
)

/*
RufsFuse is the Rufs specific FUSE filesystem API that uses paths rather
than inodes.
*/
type RufsFuse struct {
	pathfs.FileSystem
	Tree *rufs.Tree
}

/*
GetAttr is the main entry point, through which FUSE discovers which
files and directories exist.
*/
func (rf *RufsFuse) GetAttr(name string, context *fuse.Context) (*fuse.Attr, fuse.Status) {

	if name == "" {

		// Mount point is always a directory

		return &fuse.Attr{
			Mode: fuse.S_IFDIR | 0755,
		}, fuse.OK
	}

	var a *fuse.Attr

	status := fuse.ENOENT

	// Construct path and filename

	name = path.Join("/", name)
	dir, file := filepath.Split(name)

	// Query the tree

	_, fis, err := rf.Tree.Dir(dir, "", false, false)

	if err != nil {
		log.Print(err)
		status = fuse.EIO
	}

	if len(fis) > 0 {

		// Create attribute entries

		for _, fi := range fis[0] {

			if fi.Name() == file {

				a = &fuse.Attr{
					Mode: OSModeToFuseMode(fi.Mode()),
					Size: uint64(fi.Size()),
				}

				status = fuse.OK
			}
		}
	}

	return a, status
}

/*
OpenDir handles directories.
*/
func (rf *RufsFuse) OpenDir(name string,
	context *fuse.Context) ([]fuse.DirEntry, fuse.Status) {
	var c []fuse.DirEntry

	// Construct path and filename

	name = path.Join("/", name)
	status := fuse.ENOENT

	// Query the tree

	_, fis, err := rf.Tree.Dir(name, "", false, false)

	if err != nil {
		LogError(err)
		return nil, fuse.EIO
	}

	if len(fis) > 0 {

		// Create entries

		for _, fi := range fis[0] {
			c = append(c, fuse.DirEntry{
				Name: fi.Name(),
				Mode: OSModeToFuseMode(fi.Mode()),
			})
		}

		status = fuse.OK
	}

	return c, status
}

/*
Open file handling.
*/
func (rf *RufsFuse) Open(name string, flags uint32, context *fuse.Context) (file nodefs.File, code fuse.Status) {
	return &RufsFile{nodefs.NewDefaultFile(), path.Join("/", name), rf.Tree}, fuse.OK
}

// File related objects
// ====================

/*
RufsFile models a file of Rufs.
*/
type RufsFile struct {
	nodefs.File
	name string
	tree *rufs.Tree
}

/*
Read reads a portion of the file.
*/
func (f *RufsFile) Read(buf []byte, off int64) (fuse.ReadResult, fuse.Status) {
	var res fuse.ReadResult

	status := fuse.OK

	n, err := f.tree.ReadFile(f.name, buf, off)

	if err != nil {
		LogError(err)
		status = fuse.EIO
	} else {
		res = &RufsReadResult{buf, n}
	}

	return res, status
}

/*
RufsReadResult is an implementation of fuse.ReadResult.
*/
type RufsReadResult struct {
	buf []byte
	n   int
}

/*
Bytes returns the raw bytes for the read.
*/
func (r *RufsReadResult) Bytes(buf []byte) ([]byte, fuse.Status) {
	return r.buf, fuse.OK
}

/*
Size returns how many bytes this return value takes at most.
*/
func (r *RufsReadResult) Size() int {
	return r.n
}

/*
Done is called after sending the data to the kernel.
*/
func (r *RufsReadResult) Done() {}

// Helper functions
// ================

/*
OSModeToFuseMode converts a given os.FileMode to a Fuse Mode
*/
func OSModeToFuseMode(fm os.FileMode) uint32 {
	m := uint32(fm)

	m = m & 0x0FFF // Remove special bits

	if fm.IsDir() {
		m = fuse.S_IFDIR | m
	} else {
		m = fuse.S_IFREG | m
	}

	return m
}
