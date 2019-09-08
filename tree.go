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
	"bytes"
	"crypto/tls"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"devt.de/krotik/common/bitutil"
	"devt.de/krotik/common/fileutil"
	"devt.de/krotik/common/stringutil"
	"devt.de/krotik/rufs/config"
	"devt.de/krotik/rufs/node"
)

/*
Tree models a Rufs client which combines several branches.
*/
type Tree struct {
	client      *node.Client             // RPC client
	treeLock    *sync.RWMutex            // Lock for maps
	root        *treeItem                // Tree root item
	branches    []map[string]string      // Added working branches
	branchesAll []map[string]string      // All added branches also not working
	mapping     []map[string]interface{} // Mappings from working branches
	mappingAll  []map[string]interface{} // All used mappings
}

/*
NewTree creates a new tree.
*/
func NewTree(cfg map[string]interface{}, cert *tls.Certificate) (*Tree, error) {
	var err error
	var t *Tree

	// Make sure the given config is ok

	if err = config.CheckTreeConfig(cfg); err == nil {

		// Create RPC client

		c := node.NewClient(fileutil.ConfStr(cfg, config.TreeSecret), cert)

		// Create the tree

		t = &Tree{c, &sync.RWMutex{}, &treeItem{make(map[string]*treeItem),
			[]string{}, []bool{}}, []map[string]string{},
			[]map[string]string{}, []map[string]interface{}{},
			[]map[string]interface{}{}}
	}

	return t, err
}

/*
Config returns the current tree configuration as a JSON string.
*/
func (t *Tree) Config() string {
	t.treeLock.RLock()
	defer t.treeLock.RUnlock()

	out, _ := json.MarshalIndent(map[string]interface{}{
		"branches": t.branches,
		"tree":     t.mapping,
	}, "", "  ")

	return string(out)
}

/*
SetMapping adds a given tree mapping configuration in a JSON string.
*/
func (t *Tree) SetMapping(config string) error {
	var err error
	var conf map[string][]map[string]interface{}

	// Unmarshal the config

	if err = json.Unmarshal([]byte(config), &conf); err == nil {

		// Reset the whole tree

		t.Reset(true)

		if branches, ok := conf["branches"]; ok {
			for _, b := range branches {
				t.AddBranch(b["branch"].(string), b["rpc"].(string), b["fingerprint"].(string))
			}
		}

		if mounts, ok := conf["tree"]; ok {
			for _, m := range mounts {
				t.AddMapping(m["path"].(string), m["branch"].(string), m["writeable"].(bool))
			}
		}
	}

	return err
}

/*
KnownBranches returns a map of all known branches (active or not reachable).
Caution: This map contains also the map of active branches with their fingerprints
it should only be used for read operations.
*/
func (t *Tree) KnownBranches() map[string]map[string]string {
	ret := make(map[string]map[string]string)

	t.treeLock.RLock()
	t.treeLock.RUnlock()

	for _, b := range t.branchesAll {
		ret[b["branch"]] = b
	}

	return ret
}

/*
ActiveBranches returns a list of all known active branches and their fingerprints.
*/
func (t *Tree) ActiveBranches() ([]string, []string) {
	return t.client.Peers()
}

/*
NotReachableBranches returns a map of all known branches which couldn't be
reached. The map contains the name and the definition of the branch.
*/
func (t *Tree) NotReachableBranches() map[string]map[string]string {
	ret := make(map[string]map[string]string)

	t.treeLock.RLock()
	t.treeLock.RUnlock()

	activeBranches := make(map[string]map[string]string)

	for _, b := range t.branches {
		activeBranches[b["branch"]] = b
	}

	for _, b := range t.branchesAll {
		name := b["branch"]

		if _, ok := activeBranches[name]; !ok {
			ret[name] = b
		}
	}

	return ret
}

/*
PingBranch sends a ping to a remote branch and returns its fingerprint or an error.
*/
func (t *Tree) PingBranch(node string, rpc string) (string, error) {
	_, fp, err := t.client.SendPing(node, rpc)
	return fp, err
}

/*
Reset either resets only all mounts or if the branches flag is specified
also all known branches.
*/
func (t *Tree) Reset(branches bool) {

	if branches {
		peers, _ := t.client.Peers()
		for _, p := range peers {
			t.client.RemovePeer(p)
		}

		t.branches = []map[string]string{}
		t.branchesAll = []map[string]string{}
	}

	t.treeLock.Lock()
	defer t.treeLock.Unlock()

	t.mapping = []map[string]interface{}{}
	t.mappingAll = []map[string]interface{}{}

	t.root = &treeItem{make(map[string]*treeItem), []string{}, []bool{}}
}

/*
Refresh refreshes all known branches and mappings. Only reachable branches will
be mapped into the tree.
*/
func (t *Tree) Refresh() {
	addBranches := make(map[string]map[string]string)
	delBranches := make(map[string]map[string]string)

	nrBranches := t.NotReachableBranches()

	// Check all known branches and decide if they should be added or removed

	t.treeLock.RLock()

	for _, data := range t.branchesAll {
		branchName := data["branch"]
		branchRPC := data["rpc"]

		_, knownAsNotWorking := nrBranches[branchName]

		// Ping the branch

		_, _, err := t.client.SendPing(branchName, branchRPC)

		if err == nil && knownAsNotWorking {

			// Success branch can now be reached

			addBranches[branchName] = data

		} else if err != nil && !knownAsNotWorking {

			// Failure branch can no longer be reached

			delBranches[branchName] = data
		}
	}

	t.treeLock.RUnlock()

	// Now lock the tree and add/remove branches

	t.treeLock.Lock()

	for i, b := range t.branches {
		branchName := b["branch"]

		if _, ok := delBranches[branchName]; ok {
			t.client.RemovePeer(branchName)
			t.branches = append(t.branches[:i], t.branches[i+1:]...)
		}
	}

	for _, b := range addBranches {
		branchName := b["branch"]
		branchRPC := b["rpc"]
		branchFingerprint := b["fingerprint"]

		t.client.RegisterPeer(branchName, branchRPC, branchFingerprint)
		t.branches = append(t.branches, b)
	}

	// Rebuild all mappings

	mappings := t.mappingAll

	t.mapping = []map[string]interface{}{}
	t.mappingAll = []map[string]interface{}{}
	t.root = &treeItem{make(map[string]*treeItem), []string{}, []bool{}}

	t.treeLock.Unlock()

	for _, m := range mappings {
		t.AddMapping(fmt.Sprint(m["path"]), fmt.Sprint(m["branch"]), m["writeable"].(bool))
	}
}

/*
AddBranch adds a branch to the tree.
*/
func (t *Tree) AddBranch(branchName string, branchRPC string, branchFingerprint string) error {

	branchMap := map[string]string{
		"branch":      branchName,
		"rpc":         branchRPC,
		"fingerprint": branchFingerprint,
	}

	t.branchesAll = append(t.branchesAll, branchMap)

	// First ping the branch and see if we get a response

	_, fp, err := t.client.SendPing(branchName, branchRPC)

	// Only add the branch as active if we've seen it

	if err == nil {

		if branchFingerprint != "" && branchFingerprint != fp {
			err = fmt.Errorf("Remote branch has an unexpected fingerprint\nPresented fingerprint: %s\nExpected fingerprint : %s", branchFingerprint, fp)

		} else {

			t.treeLock.Lock()
			defer t.treeLock.Unlock()

			if err = t.client.RegisterPeer(branchName, branchRPC, fp); err == nil {

				// Once we know and accepted the fingerprint we change it
				//
				// Remote branches will never change their fingerprint
				// during a single network session

				branchMap["fingerprint"] = fp

				t.branches = append(t.branches, branchMap) // Store the added branch

			}
		}
	}

	return err
}

/*
AddMapping adds a mapping from tree path to a branch.
*/
func (t *Tree) AddMapping(dir, branchName string, writable bool) error {
	t.treeLock.Lock()
	defer t.treeLock.Unlock()

	err := node.ErrUnknownTarget

	mappingMap := map[string]interface{}{
		"path":      dir,
		"branch":    branchName,
		"writeable": writable,
	}

	t.mappingAll = append(t.mappingAll, mappingMap)

	peers, _ := t.client.Peers()

	for _, p := range peers {
		if p == branchName {

			// Split the given path and add the mapping

			t.root.addMapping(createMappingPath(dir), branchName, writable)
			t.mapping = append(t.mapping, mappingMap)

			err = nil
		}
	}

	return err
}

/*
String returns a string representation of this tree.
*/
func (t *Tree) String() string {

	if t.treeLock != nil {
		t.treeLock.RLock()
		defer t.treeLock.RUnlock()
	}

	var buf bytes.Buffer
	buf.WriteString("/: ")
	if t != nil && t.root != nil {
		t.root.String(1, &buf)
	}
	return buf.String()
}

// Client API
// ==========

/*
Dir returns file listings matching a given pattern of one or more directories.
The contents of the given path is returned. Optionally, also the contents of
all subdirectories can be returned if the recursive flag is set. The return
values is a list of traversed directories and their corresponding contents.
*/
func (t *Tree) Dir(dir string, pattern string, recursive bool, checksums bool) ([]string, [][]os.FileInfo, error) {
	var err error
	var dirs []string
	var fis [][]os.FileInfo

	// Compile pattern

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, nil, err
	}

	t.treeLock.RLock()
	defer t.treeLock.RUnlock()

	// Stip off trailing slashes to normalize the input

	if strings.HasSuffix(dir, "/") {
		dir = dir[:len(dir)-1]
	}

	treeVisitor := func(item *treeItem, treePath string, branchPath []string, branches []string, writable []bool) {

		for _, b := range branches {
			var res []byte

			if err == nil {

				res, err = t.client.SendData(b, map[string]string{
					ParamAction:    OpDir,
					ParamPath:      path.Join(branchPath...),
					ParamPattern:   fmt.Sprint(pattern),
					ParamRecursive: fmt.Sprint(recursive),
					ParamChecksums: fmt.Sprint(checksums),
				}, nil)

				if err == nil {
					var dest []interface{}

					// Unpack the result

					if err = gob.NewDecoder(bytes.NewBuffer(res)).Decode(&dest); err == nil {
						bdirs := dest[0].([]string)
						bfis := dest[1].([][]os.FileInfo)

						// Construct the actual tree path for the returned directories

						for i, d := range bdirs {
							bdirs[i] = path.Join(treePath, d)

							// Merge these results into the overall results

							found := false
							for j, dir := range dirs {

								// Check if a directory from the result is already
								// in the overall result

								if dir == bdirs[i] {
									found = true

									// Create a map of existing names to avoid duplicates

									existing := make(map[string]bool)
									for _, fi := range fis[j] {
										existing[fi.Name()] = true
									}

									// Only add new files to the overall result

									for _, fi := range bfis[i] {
										if _, ok := existing[fi.Name()]; !ok {
											fis[j] = append(fis[j], fi)
										}
									}
								}
							}

							if !found {

								// Just append if the directory is not in the
								// overall results yet

								dirs = append(dirs, bdirs[i])
								fis = append(fis, bfis[i])
							}
						}
					}
				}
			}
		}
	}

	t.root.findPathBranches("/", createMappingPath(dir), recursive, treeVisitor)

	// Add pseudo directories for mapping components which have no corresponding
	// real directories

	dirsMap := make(map[string]int)
	for i, d := range dirs {
		dirsMap[d] = i
	}

	t.root.findPathBranches("/", createMappingPath(dir), recursive,
		func(item *treeItem, treePath string, branchPath []string, branches []string, writable []bool) {

			if !strings.HasPrefix(treePath, dir) {
				return
			}

			idx, ok := dirsMap[treePath]

			if !ok {

				// Create the entry if it does not exist

				dirs = append(dirs, treePath)
				idx = len(dirs) - 1
				dirsMap[treePath] = idx
				fis = append(fis, []os.FileInfo{})
			}

			// Add pseudo dirs if a physical directory is not present

			for n := range item.children {

				found := false
				for _, fi := range fis[idx] {
					if fi.Name() == n {
						found = true
						break
					}
				}

				if found {
					continue
				}

				if re.MatchString(n) {

					// Append if it matches the pattern

					fis[idx] = append(fis[idx], &FileInfo{
						FiName:    n,
						FiSize:    0,
						FiMode:    os.FileMode(os.ModeDir | 0777),
						FiModTime: time.Time{},
					})
				}
			}
		})

	return dirs, fis, err
}

/*
Stat returns information about a given item. Use this function to find out
if a given path is a file or directory.
*/
func (t *Tree) Stat(item string) (os.FileInfo, error) {

	dir, file := path.Split(item)

	_, fis, err := t.Dir(dir, file, false, true)

	if len(fis) == 1 {
		for _, fi := range fis[0] {
			if fi.Name() == file {
				return fi, err
			}
		}
	}

	if err == nil {
		err = &node.Error{
			Type:       node.ErrRemoteAction,
			Detail:     os.ErrNotExist.Error(),
			IsNotExist: true,
		}
	}

	return nil, err
}

/*
Copy is a general purpose copy function which creates files and directories.
Destination must be a directory. A non-existing destination
directory will be created.
*/
func (t *Tree) Copy(src []string, dst string,
	updFunc func(file string, writtenBytes, totalBytes, currentFile, totalFiles int64)) error {
	var err error
	var relPaths []string

	files := make(map[string]os.FileInfo) // Make sure any file is only copied once
	paths := make(map[string]string)

	// Add files to be copied to items

	for _, s := range src {
		var fi os.FileInfo

		fi, err = t.Stat(s)

		if fi, err = t.Stat(s); fi != nil {

			if fi.IsDir() {

				// Find all files inside directories

				if dirs, fis, err := t.Dir(s, "", true, false); err == nil {

					for i, d := range dirs {
						for _, fi2 := range fis[i] {

							if !fi2.IsDir() {

								// Calculate the relative path by removing
								// source path from the absolute path

								relPath := path.Join(d, fi2.Name())[len(s):]
								relPath = path.Join("/"+fi.Name(), relPath)

								relPaths = append(relPaths, relPath)
								files[relPath] = fi2
								paths[relPath] = path.Join(d, fi2.Name())
							}
						}
					}
				}

			} else {

				// Single files are just added - these files will always
				// be at the root of the destination

				relPath := "/" + fi.Name()

				relPaths = append(relPaths, relPath)
				files[relPath] = fi
				paths[relPath] = s
			}
		}

		if err != nil {
			err = fmt.Errorf("Cannot stat %v: %v", s, err.Error())
			break
		}
	}

	if err == nil {
		var allFiles, cnt int64

		// Copy all found files

		allFiles = int64(len(files))

		for _, k := range relPaths {
			var totalSize, totalTransferred int64

			cnt++
			fi := files[k]
			totalSize = fi.Size()
			srcFile := paths[k]

			err = t.CopyFile(srcFile, path.Join(dst, k), func(b int) {
				if b >= 0 {
					totalTransferred += int64(b)
					updFunc(k, totalTransferred, totalSize, cnt, allFiles)
				} else {
					updFunc(k, int64(b), totalSize, cnt, allFiles)
				}
			})

			if err != nil {
				err = fmt.Errorf("Cannot copy %v to %v: %v", srcFile, dst, err.Error())
				break
			}
		}
	}

	return err
}

/*
Sync operations
*/
const (
	SyncCreateDirectory = "Create directory"
	SyncCopyFile        = "Copy file"
	SyncRemoveDirectory = "Remove directory"
	SyncRemoveFile      = "Remove file"
)

/*
Sync a given destination with a given source directory. After this command has
finished the dstDir will have the same files and directories as the srcDir.
*/
func (t *Tree) Sync(srcDir string, dstDir string, recursive bool,
	updFunc func(op, srcFile, dstFile string, writtenBytes, totalBytes, currentFile, totalFiles int64)) error {

	var currentFile, totalFiles int64

	t.treeLock.RLock()
	defer t.treeLock.RUnlock()

	// doSync syncs a given src directory

	doSync := func(dir string, finfos []os.FileInfo) error {

		sdir := path.Join(srcDir, dir)
		ddir := path.Join(dstDir, dir)

		// Query the corresponding destination to see what is there

		_, dstFis, err := t.Dir(ddir, "", false, true)

		if err == nil {
			fileMap := make(map[string]string) // Map to quickly lookup destination files
			dirMap := make(map[string]bool)    // Map to quickly lookup destination directories

			if len(dstFis) > 0 {

				for _, fi := range dstFis[0] {
					if fi.IsDir() {
						dirMap[fi.Name()] = true
					} else {
						fileMap[fi.Name()] = fi.(*FileInfo).Checksum()
					}
				}
			}

			// Go through the given source file infos and see what needs to be copied

			for _, fi := range finfos {
				currentFile++

				//  Check if we have a directory or a file

				if fi.IsDir() {

					if _, ok := dirMap[fi.Name()]; !ok {

						// Create all directories which aren't there

						if updFunc != nil {
							updFunc(SyncCreateDirectory, "", path.Join(ddir, fi.Name()), 0, 0, currentFile, totalFiles)
						}

						_, err = t.ItemOp(ddir, map[string]string{
							ItemOpAction: ItemOpActMkDir,
							ItemOpName:   fi.Name(),
						})
					}

					// Remove existing directories from the map so we can
					// use the map to remove directories which shouldn't
					// be there

					delete(dirMap, fi.Name())

				} else {

					fsum, ok := fileMap[fi.Name()]

					if !ok || fsum != fi.(*FileInfo).Checksum() {
						var u func(b int)

						s := path.Join(sdir, fi.Name())
						d := path.Join(ddir, fi.Name())

						// Copy the file if it does not exist or the checksum
						// is not matching

						if updFunc != nil {
							var totalTransferred, totalSize int64

							totalSize = fi.Size()

							u = func(b int) {

								if b >= 0 {
									totalTransferred += int64(b)
									updFunc(SyncCopyFile, s, d, totalTransferred, totalSize, currentFile, totalFiles)
								} else {
									updFunc(SyncCopyFile, s, d, int64(b), totalSize, currentFile, totalFiles)
								}
							}
						}

						if err = t.CopyFile(s, d, u); err != nil && updFunc != nil {

							// Note at which point the error message was produced

							updFunc(SyncCopyFile, s, d, 0, fi.Size(), currentFile, totalFiles)
						}
					}

					// Remove existing files from the map so we can
					// use the map to remove files which shouldn't
					// be there

					delete(fileMap, fi.Name())
				}

				if err != nil {
					break
				}
			}

			if err == nil {

				// Remove files and directories which are in the destination but
				// not in the source

				for d := range dirMap {
					if err == nil {

						if updFunc != nil {
							p := path.Join(ddir, d)
							updFunc(SyncRemoveDirectory, "", p, 0, 0, currentFile, totalFiles)
						}

						_, err = t.ItemOp(ddir, map[string]string{
							ItemOpAction: ItemOpActDelete,
							ItemOpName:   d,
						})
					}
				}

				for f := range fileMap {
					if err == nil {

						if updFunc != nil {
							p := path.Join(ddir, f)
							updFunc(SyncRemoveFile, "", p, 0, 0, currentFile, totalFiles)
						}

						_, err = t.ItemOp(ddir, map[string]string{
							ItemOpAction: ItemOpActDelete,
							ItemOpName:   f,
						})
					}
				}
			}
		}

		return err
	}

	// We only query the source once otherwise we might end up in an
	// endless loop if for example the dstDir is a subdirectory of srcDir

	srcDirs, srcFis, err := t.Dir(srcDir, "", recursive, true)

	if err == nil {

		for _, fis := range srcFis {
			totalFiles += int64(len(fis))
		}

		for i, dir := range srcDirs {

			if err = doSync(relPath(dir, srcDir), srcFis[i]); err != nil {
				break
			}
		}
	}

	return err
}

/*
CopyFile copies a given file using a simple io.Pipe.
*/
func (t *Tree) CopyFile(srcPath, dstPath string, updFunc func(writtenBytes int)) error {
	var pw io.WriteCloser
	var err, rerr error

	t.treeLock.RLock()
	defer t.treeLock.RUnlock()

	// Use a pipe to stream the contents of the source file to the destination file

	pr, pw := io.Pipe()

	if updFunc != nil {

		// Wrap the writer of the pipe

		pw = &statusUpdatingWriter{pw, updFunc}
	}

	// Make sure the src exists

	if _, rerr = t.ReadFile(srcPath, []byte{}, 0); rerr == nil {

		// Read the source in a go routine

		go func() {
			rerr = t.ReadFileToBuffer(srcPath, pw)
			pw.Close()
		}()

		// Write the destination file - this will return once the
		// writer is closed

		err = t.WriteFileFromBuffer(dstPath, pr)
	}

	if rerr != nil {

		// Check if we got an empty file

		if IsEOF(rerr) {

			_, err = t.WriteFile(dstPath, nil, 0)

			updFunc(0) // Report the creation of the empty file

			rerr = nil

		} else {

			// Read errors are reported before write errors

			err = rerr
		}
	}

	pr.Close()

	return err
}

/*
ReadFileToBuffer reads a complete file into a given buffer which implements
io.Writer.
*/
func (t *Tree) ReadFileToBuffer(spath string, buf io.Writer) error {
	var n int
	var err error
	var offset int64

	readBuf := make([]byte, DefaultReadBufferSize)

	for err == nil {
		n, err = t.ReadFile(spath, readBuf, offset)

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
func (t *Tree) ReadFile(spath string, p []byte, offset int64) (int, error) {
	var err error
	var n int
	var success bool

	t.treeLock.RLock()
	defer t.treeLock.RUnlock()

	err = &node.Error{
		Type:       node.ErrRemoteAction,
		Detail:     os.ErrNotExist.Error(),
		IsNotExist: true,
	}

	dir, file := path.Split(spath)

	t.root.findPathBranches(dir, createMappingPath(dir), false,
		func(item *treeItem, treePath string, branchPath []string, branches []string, writable []bool) {

			for _, b := range branches {

				if !success { // Only try other branches if we didn't have a success before

					var res []byte

					rpath := path.Join(branchPath...)
					rpath = path.Join(rpath, file)

					if res, err = t.client.SendData(b, map[string]string{
						ParamAction: OpRead,
						ParamPath:   rpath,
						ParamOffset: fmt.Sprint(offset),
						ParamSize:   fmt.Sprint(len(p)),
					}, nil); err == nil {
						var dest []interface{}

						// Unpack the result

						if err = gob.NewDecoder(bytes.NewBuffer(res)).Decode(&dest); err == nil {
							n = dest[0].(int)
							buf := dest[1].([]byte)

							copy(p, buf)
						}
					}

					success = err == nil

					// Special case EOF

					if IsEOF(err) {
						success = true
					}
				}
			}
		})

	return n, err
}

/*
WriteFileFromBuffer writes a complete file from a given buffer which implements
io.Reader.
*/
func (t *Tree) WriteFileFromBuffer(spath string, buf io.Reader) error {
	var err error
	var offset int64

	writeBuf := make([]byte, DefaultReadBufferSize)

	for err == nil {
		var n int

		if n, err = buf.Read(writeBuf); err == nil {

			_, err = t.WriteFile(spath, writeBuf[:n], offset)
			offset += int64(n)

		} else if IsEOF(err) {

			// We reached the end of the file

			t.WriteFile(spath, []byte{}, offset)

			err = nil
			break
		}
	}

	return err
}

/*
WriteFile writes p into the given file from the given offset. It
returns the number of written bytes and any error encountered.
*/
func (t *Tree) WriteFile(spath string, p []byte, offset int64) (int, error) {
	var err error
	var n, totalCount, ignoreCount int

	t.treeLock.RLock()
	defer t.treeLock.RUnlock()

	dir, file := path.Split(spath)

	t.root.findPathBranches(dir, createMappingPath(dir), false,
		func(item *treeItem, treePath string, branchPath []string, branches []string, writable []bool) {

			for i, b := range branches {
				var res []byte

				if err == nil {

					totalCount++

					if !writable[i] {

						// Ignore all non-writable branches

						ignoreCount++

						continue
					}

					rpath := path.Join(branchPath...)
					rpath = path.Join(rpath, file)

					if res, err = t.client.SendData(b, map[string]string{
						ParamAction: OpWrite,
						ParamPath:   rpath,
						ParamOffset: fmt.Sprint(offset),
					}, p); err == nil {
						err = gob.NewDecoder(bytes.NewBuffer(res)).Decode(&n)
					}
				}
			}

		})

	if err == nil && totalCount == ignoreCount {
		err = fmt.Errorf("All applicable branches for the requested path were mounted as not writable")
	}

	return n, err
}

/*
ItemOp executes a file or directory specific operation which can either
succeed or fail (e.g. rename or delete). Actions and parameters should
be given in the opdata map.
*/
func (t *Tree) ItemOp(dir string, opdata map[string]string) (bool, error) {
	var err error
	var ret, recurse bool
	var totalCount, ignoreCount, notFoundCount int

	t.treeLock.RLock()
	defer t.treeLock.RUnlock()

	data := make(map[string]string)

	for k, v := range opdata {
		data[k] = v
	}

	data[ParamAction] = OpItemOp

	// Check if we should recurse

	if r, ok := data[ItemOpName]; ok {
		recurse = strings.HasSuffix(r, "**")
	}

	t.root.findPathBranches(dir, createMappingPath(dir), recurse,
		func(item *treeItem, treePath string, branchPath []string,
			branches []string, writable []bool) {

			for i, b := range branches {
				var res []byte

				totalCount++

				if !writable[i] {

					// Ignore all non-writable branches

					ignoreCount++

					continue
				}

				if err == nil {

					data[ParamPath] = path.Join(branchPath...)

					res, err = t.client.SendData(b, data, nil)

					if rerr, ok := err.(*node.Error); ok && rerr.IsNotExist {

						// Only count the not exist errors as this might only
						// be true for some branches

						notFoundCount++
						err = nil

					} else if err == nil {
						var bres bool

						// Execute the OpItem function

						err = gob.NewDecoder(bytes.NewBuffer(res)).Decode(&bres)

						ret = ret || bres // One positive result is enough
					}
				}
			}
		})

	if totalCount == ignoreCount {
		err = fmt.Errorf("All applicable branches for the requested path were mounted as not writable")
	} else if totalCount == notFoundCount+ignoreCount {
		err = &node.Error{
			Type:       node.ErrRemoteAction,
			Detail:     os.ErrNotExist.Error(),
			IsNotExist: true,
		}
	}

	return ret, err
}

// Util functions
// ==============

/*
createMappingPath properly splits a given path into a mapping path.
*/
func createMappingPath(path string) []string {
	var ret []string

	for _, i := range strings.Split(path, "/") {
		if i == "" {

			// Ignore empty child names

			continue
		}
		ret = append(ret, i)
	}

	return ret
}

/*
DirResultToString formats a given Dir result into a human-readable string.
*/
func DirResultToString(paths []string, infos [][]os.FileInfo) string {
	var buf bytes.Buffer

	// Sort the paths

	sort.Sort(&dirResult{paths, infos})

	// Sort the FileInfos within the paths

	for _, fis := range infos {
		sort.Sort(fileInfoSlice(fis))
	}

	for i, p := range paths {
		var maxlen int

		fis := infos[i]

		buf.WriteString(p)
		buf.WriteString("\n")

		sizeStrings := make([]string, 0, len(fis))

		for _, fi := range fis {
			sizeString := bitutil.ByteSizeString(fi.Size(), false)
			if strings.HasSuffix(sizeString, " B") {
				sizeString += "  " // Unit should always be 3 runes
			}
			if l := utf8.RuneCountInString(sizeString); l > maxlen {
				maxlen = l
			}
			sizeStrings = append(sizeStrings, sizeString)
		}

		for j, fi := range fis {
			sizeString := sizeStrings[j]
			sizePrefix := stringutil.GenerateRollingString(" ",
				maxlen-utf8.RuneCountInString(sizeString))

			if rfi, ok := fi.(*FileInfo); ok && rfi.FiChecksum != "" {
				buf.WriteString(fmt.Sprintf("%v %v%v %v [%s]\n", fi.Mode(), sizePrefix,
					sizeString, fi.Name(), rfi.Checksum()))
			} else {
				buf.WriteString(fmt.Sprintf("%v %v%v %v\n", fi.Mode(), sizePrefix,
					sizeString, fi.Name()))
			}
		}

		if i < len(paths)-1 {
			buf.WriteString("\n")
		}
	}

	return buf.String()
}

// Helper functions
// ================

// Helper function to normalise relative paths

/*
relPath create a normalized relative path by removing a given path prefix.
*/
func relPath(path, prefix string) string {

	norm := func(path string) string {
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		if strings.HasSuffix(path, "/") {
			path = path[:len(path)-1]
		}

		return path
	}

	path = norm(path)
	prefix = norm(prefix)

	if strings.HasPrefix(path, prefix) {
		path = path[len(prefix):]

		if path == "" {
			path = "/"
		}
	}

	return path
}

// Helper objects to sort dir results

type dirResult struct {
	paths []string
	infos [][]os.FileInfo
}

func (r *dirResult) Len() int           { return len(r.paths) }
func (r *dirResult) Less(i, j int) bool { return r.paths[i] < r.paths[j] }
func (r *dirResult) Swap(i, j int) {
	r.paths[i], r.paths[j] = r.paths[j], r.paths[i]
	r.infos[i], r.infos[j] = r.infos[j], r.infos[i]
}

type fileInfoSlice []os.FileInfo

func (f fileInfoSlice) Len() int           { return len(f) }
func (f fileInfoSlice) Less(i, j int) bool { return f[i].Name() < f[j].Name() }
func (f fileInfoSlice) Swap(i, j int)      { f[i], f[j] = f[j], f[i] }

// Helper object to given status updates when copying files

/*
statusUpdatingWriter is an internal io.WriteCloser which is used for status
updates.
*/
type statusUpdatingWriter struct {
	io.WriteCloser
	statusUpdate func(writtenBytes int)
}

/*
Write writes len(p) bytes from p to the writer.
*/
func (w *statusUpdatingWriter) Write(p []byte) (int, error) {
	n, err := w.WriteCloser.Write(p)
	w.statusUpdate(n)
	return n, err
}

/*
Close closes the writer.
*/
func (w *statusUpdatingWriter) Close() error {
	w.statusUpdate(-1)
	return w.WriteCloser.Close()
}
