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
Package rufs contains the main API to Rufs.

Rufs is organized as a collection of branches. Each branch represents a physical
file system structure which can be queried and updated by an authorized client.

On the client side one or several branches are organized into a tree. The
single branches can overlay each other. For example:

Branch A
/foo/A
/foo/B
/bar/C

Branch B
/foo/C
/test/D

Tree 1
/myspace => Branch A, Branch B

Accessing tree with:
/myspace/foo/A gets file /foo/A from Branch A while
/myspace/foo/C gets file /foo/C from Branch B

Write operations go only to branches which are mapped as writing branches
and who accept them (i.e. are not set to readonly on the side of the branch).
*/
package rufs

import (
	"bytes"
	"crypto/tls"
	"encoding/gob"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"devt.de/krotik/common/errorutil"
	"devt.de/krotik/common/fileutil"
	"devt.de/krotik/common/pools"
	"devt.de/krotik/common/stringutil"
	"devt.de/krotik/rufs/config"
	"devt.de/krotik/rufs/node"
)

func init() {

	// Make sure we can use the relevant types in a gob operation

	gob.Register([][]os.FileInfo{})
	gob.Register(&FileInfo{})
}

/*
Branch models a single exported branch in Rufs.
*/
type Branch struct {
	rootPath string         // Local directory (absolute path) modeling the branch root
	node     *node.RufsNode // Local RPC node
	readonly bool           // Flag if this branch is readonly
}

/*
NewBranch returns a new exported branch.
*/
func NewBranch(cfg map[string]interface{}, cert *tls.Certificate) (*Branch, error) {
	var err error
	var b *Branch

	// Make sure the given config is ok

	if err = config.CheckBranchExportConfig(cfg); err == nil {

		// Create RPC server

		addr := fmt.Sprintf("%v:%v", fileutil.ConfStr(cfg, config.RPCHost),
			fileutil.ConfStr(cfg, config.RPCPort))

		rn := node.NewNode(addr, fileutil.ConfStr(cfg, config.BranchName),
			fileutil.ConfStr(cfg, config.BranchSecret), cert, nil)

		// Start the rpc server

		if err = rn.Start(cert); err == nil {
			var rootPath string

			//  Construct root path

			if rootPath, err = filepath.Abs(fileutil.ConfStr(cfg, config.LocalFolder)); err == nil {
				b = &Branch{rootPath, rn, fileutil.ConfBool(cfg, config.EnableReadOnly)}
				rn.DataHandler = b.requestHandler
			}
		}
	}

	return b, err
}

/*
Name returns the name of the branch.
*/
func (b *Branch) Name() string {
	return b.node.Name()
}

/*
SSLFingerprint returns the SSL fingerprint of the branch.
*/
func (b *Branch) SSLFingerprint() string {
	return b.node.SSLFingerprint()
}

/*
Shutdown shuts the branch down.
*/
func (b *Branch) Shutdown() error {
	return b.node.Shutdown()
}

/*
IsReadOnly returns if this branch is read-only.
*/
func (b *Branch) IsReadOnly() bool {
	return b.readonly
}

/*
checkReadOnly returns an error if this branch is read-only.
*/
func (b *Branch) checkReadOnly() error {
	var err error

	if b.IsReadOnly() {
		err = fmt.Errorf("Branch %v is read-only", b.Name())
	}

	return err
}

// Branch API
// ==========

/*
Dir returns file listings matching a given pattern of one or more directories.
The contents of the given path is returned along with checksums if the checksum
flag is specified. Optionally, also the contents of all subdirectories can be
returned if the recursive flag is set. The return values is a list of traversed
directories (platform-agnostic) and their corresponding contents.
*/
func (b *Branch) Dir(spath string, pattern string, recursive bool, checksums bool) ([]string, [][]os.FileInfo, error) {
	var fis []os.FileInfo

	// Compile pattern

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, nil, err
	}

	createRufsFileInfos := func(dirname string, afis []os.FileInfo) []os.FileInfo {
		var fis []os.FileInfo

		fis = make([]os.FileInfo, 0, len(afis))

		for _, fi := range afis {

			// Append if it matches the pattern

			if re.MatchString(fi.Name()) {
				fis = append(fis, fi)
			}
		}

		// Wrap normal file infos and calculate checksum if necessary

		ret := WrapFileInfos(dirname, fis)

		if checksums {
			for _, fi := range fis {
				if !fi.IsDir() {

					// The sum is either there or not ... - access errors should
					// be caught when trying to read the file

					sum, _ := fileutil.CheckSumFileFast(filepath.Join(dirname, fi.Name()))

					fi.(*FileInfo).FiChecksum = sum
				}
			}
		}

		return ret
	}

	subPath, err := b.constructSubPath(spath)

	if err == nil {

		if !recursive {

			if fis, err = ioutil.ReadDir(subPath); err == nil {
				return []string{spath},
					[][]os.FileInfo{createRufsFileInfos(subPath, fis)}, nil
			}

		} else {

			var rpaths []string
			var rfis [][]os.FileInfo
			var addSubDir func(string, string) error

			// Recursive function to walk directories and symlinks
			// in a platform-agnostic way

			addSubDir = func(p string, rp string) error {
				fis, err = ioutil.ReadDir(p)

				if err == nil {
					rpaths = append(rpaths, rp)

					rfis = append(rfis, createRufsFileInfos(p, fis))

					for _, fi := range fis {

						if err == nil && fi.IsDir() {
							err = addSubDir(filepath.Join(p, fi.Name()),
								path.Join(rp, fi.Name()))
						}
					}
				}

				return err
			}

			if err = addSubDir(subPath, spath); err == nil {
				return rpaths, rfis, nil
			}
		}
	}

	// Ignore any not exists errors

	if os.IsNotExist(err) {
		err = nil
	}

	return nil, nil, err
}

/*
ReadFileToBuffer reads a complete file into a given buffer which implements
io.Writer.
*/
func (b *Branch) ReadFileToBuffer(spath string, buf io.Writer) error {
	var n int
	var err error
	var offset int64

	readBuf := make([]byte, DefaultReadBufferSize)

	for err == nil {
		n, err = b.ReadFile(spath, readBuf, offset)

		if err == nil {
			_, err = buf.Write(readBuf[:n])

			offset += int64(n)

		} else if IsEOF(err) {

			// We reached the end of the file

			err = nil
			break
		}
	}

	return err
}

/*
ReadFile reads up to len(p) bytes into p from the given offset. It
returns the number of bytes read (0 <= n <= len(p)) and any error
encountered.
*/
func (b *Branch) ReadFile(spath string, p []byte, offset int64) (int, error) {
	var n int

	subPath, err := b.constructSubPath(spath)

	if err == nil {
		var fi os.FileInfo

		if fi, err = os.Stat(subPath); err == nil {

			if fi.IsDir() {
				err = fmt.Errorf("read /%v: is a directory", spath)

			} else if err == nil {
				var f *os.File

				if f, err = os.Open(subPath); err == nil {
					defer f.Close()

					sr := io.NewSectionReader(f, 0, fi.Size())

					if _, err = sr.Seek(offset, io.SeekStart); err == nil {
						n, err = sr.Read(p)
					}
				}
			}
		}
	}

	return n, err
}

/*
WriteFileFromBuffer writes a complete file from a given buffer which implements
io.Reader.
*/
func (b *Branch) WriteFileFromBuffer(spath string, buf io.Reader) error {
	var err error
	var offset int64

	if err = b.checkReadOnly(); err == nil {

		writeBuf := make([]byte, DefaultReadBufferSize)

		for err == nil {
			var n int

			if n, err = buf.Read(writeBuf); err == nil {

				_, err = b.WriteFile(spath, writeBuf[:n], offset)
				offset += int64(n)

			} else if IsEOF(err) {

				// We reached the end of the file

				b.WriteFile(spath, []byte{}, offset)

				err = nil
				break
			}
		}
	}

	return err
}

/*
WriteFile writes p into the given file from the given offset. It
returns the number of written bytes and any error encountered.
*/
func (b *Branch) WriteFile(spath string, p []byte, offset int64) (int, error) {
	var n int
	var m int64

	if err := b.checkReadOnly(); err != nil {
		return 0, err
	}

	buf := byteSlicePool.Get().([]byte)
	defer func() {
		byteSlicePool.Put(buf)
	}()

	growFile := func(f *os.File, n int64) {
		var err error

		toWrite := n

		for err == nil && toWrite > 0 {
			if toWrite > int64(DefaultReadBufferSize) {
				_, err = f.Write(buf[:DefaultReadBufferSize])
				toWrite -= int64(DefaultReadBufferSize)
			} else {
				_, err = f.Write(buf[:toWrite])
				toWrite = 0
			}
		}
	}

	subPath, err := b.constructSubPath(spath)

	if err == nil {
		var fi os.FileInfo
		var f *os.File

		if fi, err = os.Stat(subPath); os.IsNotExist(err) {

			// Ensure path exists

			dir, _ := filepath.Split(subPath)

			if err = os.MkdirAll(dir, 0755); err == nil {

				// Create the file newly

				if f, err = os.OpenFile(subPath, os.O_RDWR|os.O_CREATE, 0644); err == nil {
					defer f.Close()

					if offset > 0 {
						growFile(f, offset)
					}

					m, err = io.Copy(f, bytes.NewBuffer(p))
					n += int(m)
				}
			}

		} else if err == nil {

			// File does exist

			if f, err := os.OpenFile(subPath, os.O_RDWR, 0644); err == nil {
				defer f.Close()

				if fi.Size() < offset {
					f.Seek(fi.Size(), io.SeekStart)
					growFile(f, offset-fi.Size())
				} else {
					f.Seek(offset, io.SeekStart)
				}

				m, err = io.Copy(f, bytes.NewBuffer(p))
				errorutil.AssertOk(err)

				n += int(m)
			}
		}
	}

	return n, err
}

/*
ItemOp parameter
*/
const (
	ItemOpAction  = "itemop_action"  // ItemOp action
	ItemOpName    = "itemop_name"    // Item name
	ItemOpNewName = "itemop_newname" // New item name
)

/*
ItemOp actions
*/
const (
	ItemOpActRename = "rename" // Rename a file or directory
	ItemOpActDelete = "delete" // Delete a file or directory
	ItemOpActMkDir  = "mkdir"  // Create a directory
)

/*
ItemOp executes a file or directory specific operation which can either
succeed or fail (e.g. rename or delete). Actions and parameters should
be given in the opdata map.
*/
func (b *Branch) ItemOp(spath string, opdata map[string]string) (bool, error) {
	res := false

	if err := b.checkReadOnly(); err != nil {
		return false, err
	}

	subPath, err := b.constructSubPath(spath)

	if err == nil {

		action := opdata[ItemOpAction]

		fileFromOpData := func(key string) (string, error) {

			// Make sure we are only dealing with files

			_, name := filepath.Split(opdata[key])

			if name == "" {
				return "", fmt.Errorf("This operation requires a specific file or directory")
			}

			// Build the relative paths

			return filepath.Join(filepath.FromSlash(subPath), name), nil
		}

		if action == ItemOpActMkDir {
			var name string

			// Make directory action

			if name, err = fileFromOpData(ItemOpName); err == nil {

				err = os.MkdirAll(name, 0755)
			}

		} else if action == ItemOpActRename {
			var name, newname string

			// Rename action

			if name, err = fileFromOpData(ItemOpName); err == nil {
				if newname, err = fileFromOpData(ItemOpNewName); err == nil {

					err = os.Rename(name, newname)
				}
			}

		} else if action == ItemOpActDelete {
			var name string

			// Delete action

			if name, err = fileFromOpData(ItemOpName); err == nil {

				del := func(name string) error {
					var err error
					if ok, _ := fileutil.PathExists(name); ok {
						err = os.RemoveAll(name)
					} else {
						err = os.ErrNotExist
					}
					return err
				}

				if strings.Contains(name, "*") {
					var rex string

					// We have a wildcard

					rootdir, glob := filepath.Split(name)

					// Create a regex from the given glob expression

					if rex, err = stringutil.GlobToRegex(glob); err == nil {
						var dirs []string
						var fis [][]os.FileInfo

						if dirs, fis, err = b.Dir(spath, rex, true, false); err == nil {

							for i, dir := range dirs {

								// Remove all files and dirs according to the wildcard

								for _, fi := range fis[i] {
									os.RemoveAll(filepath.Join(rootdir,
										filepath.FromSlash(dir), fi.Name()))
								}
							}
						}
					}

				} else {

					err = del(name)
				}
			}
		}

		// Determine if we succeeded

		res = err == nil || os.IsNotExist(err)
	}

	return res, err
}

// Request handling functions
// ==========================

/*
DefaultReadBufferSize is the default size for file reading.
*/
var DefaultReadBufferSize = 1024 * 16

/*
bufferPool holds buffers which are used to marshal objects.
*/
var bufferPool = pools.NewByteBufferPool()

/*
byteSlicePool holds buffers which are used to read files
*/
var byteSlicePool = pools.NewByteSlicePool(DefaultReadBufferSize)

/*
Meta parameter
*/
const (
	ParamAction    = "a" // Requested action
	ParamPath      = "p" // Path string
	ParamPattern   = "x" // Pattern string
	ParamRecursive = "r" // Recursive flag
	ParamChecksums = "c" // Checksum flag
	ParamOffset    = "o" // Offset parameter
	ParamSize      = "s" // Size parameter
)

/*
Possible actions
*/
const (
	OpDir    = "dir"    // Read the contents of a path
	OpRead   = "read"   // Read the contents of a file
	OpWrite  = "write"  // Read the contents of a file
	OpItemOp = "itemop" // File or directory operation
)

/*
requestHandler handles incoming requests from other branches or trees.
*/
func (b *Branch) requestHandler(ctrl map[string]string, data []byte) ([]byte, error) {
	var err error
	var res interface{}
	var ret []byte

	action := ctrl[ParamAction]

	// Handle operation requests

	if action == OpDir {
		var dirs []string
		var fis [][]os.FileInfo

		dir := ctrl[ParamPath]
		pattern := ctrl[ParamPattern]
		rec := strings.ToLower(ctrl[ParamRecursive]) == "true"
		sum := strings.ToLower(ctrl[ParamChecksums]) == "true"

		if dirs, fis, err = b.Dir(dir, pattern, rec, sum); err == nil {
			res = []interface{}{dirs, fis}
		}

	} else if action == OpItemOp {

		res, err = b.ItemOp(ctrl[ParamPath], ctrl)

	} else if action == OpRead {
		var size, n int
		var offset int64

		spath := ctrl[ParamPath]

		if size, err = strconv.Atoi(ctrl[ParamSize]); err == nil {
			if offset, err = strconv.ParseInt(ctrl[ParamOffset], 10, 64); err == nil {

				buf := byteSlicePool.Get().([]byte)
				defer func() {
					byteSlicePool.Put(buf)
				}()

				if len(buf) < size {

					// Constantly requesting bigger buffers will
					// eventually replace all default sized buffers

					buf = make([]byte, size)
				}

				if n, err = b.ReadFile(spath, buf[:size], offset); err == nil {
					res = []interface{}{n, buf[:size]}
				}
			}
		}
	} else if action == OpWrite {
		var offset int64

		spath := ctrl[ParamPath]
		if offset, err = strconv.ParseInt(ctrl[ParamOffset], 10, 64); err == nil {

			res, err = b.WriteFile(spath, data, offset)
		}
	}

	// Send the response

	if err == nil {

		// Allocate a new encoding buffer - no need to lock as
		// it is based on sync.Pool

		// Pooled encoding buffers are used to keep expensive buffer
		// reallocations to a minimum. It is better to allocate the
		// actual response buffer once the response size is known.

		bb := bufferPool.Get().(*bytes.Buffer)

		if err = gob.NewEncoder(bb).Encode(res); err == nil {
			toSend := bb.Bytes()

			// Allocate the response array

			ret = make([]byte, len(toSend))

			// Copy encoded result into the response array

			copy(ret, toSend)
		}

		// Return the encoding buffer back to the pool

		go func() {
			bb.Reset()
			bufferPool.Put(bb)
		}()
	}

	if err != nil {

		// Ensure we don't leak local paths - this might not work in
		// all situations and depends on the underlying os. In this
		// error messages might include information on the full local
		// path in error messages.

		absRoot, _ := filepath.Abs(b.rootPath)
		err = fmt.Errorf("%v", strings.Replace(err.Error(), absRoot, "", -1))
	}

	return ret, err
}

// Util functions
// ==============

func (b *Branch) constructSubPath(rpath string) (string, error) {

	// Produce the actual subpath - this should also produce windows
	// paths correctly (i.e. foo/bar -> C:\root\foo\bar)

	subPath := filepath.Join(b.rootPath, filepath.FromSlash(rpath))

	// Check that the new sub path is under the root path

	absSubPath, err := filepath.Abs(subPath)

	if err == nil {

		if strings.HasPrefix(absSubPath, b.rootPath) {
			return subPath, nil
		}

		err = fmt.Errorf("Requested path %v is outside of the branch", rpath)
	}

	return "", err
}
