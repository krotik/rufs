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
	"io/ioutil"
	"os"
	"testing"

	"devt.de/krotik/common/errorutil"
	"devt.de/krotik/rufs"
	"devt.de/krotik/rufs/config"
)

func TestSimpleFileOperations(t *testing.T) {
	var buf bytes.Buffer

	// Build up a tree with multiple branches

	cfg := map[string]interface{}{
		config.TreeSecret: "123",
	}

	tree, _ := rufs.NewTree(cfg, clientCert)

	fooRPC := fmt.Sprintf("%v:%v", branchConfigs["footest"][config.RPCHost], branchConfigs["footest"][config.RPCPort])
	fooFP := footest.SSLFingerprint()
	barRPC := fmt.Sprintf("%v:%v", branchConfigs["bartest"][config.RPCHost], branchConfigs["bartest"][config.RPCPort])
	barFP := bartest.SSLFingerprint()
	tmpRPC := fmt.Sprintf("%v:%v", branchConfigs["tmptest"][config.RPCHost], branchConfigs["tmptest"][config.RPCPort])
	tmpFP := tmptest.SSLFingerprint()

	tree.AddBranch(footest.Name(), fooRPC, fooFP)
	tree.AddBranch(bartest.Name(), barRPC, barFP)
	tree.AddBranch(tmptest.Name(), tmpRPC, tmpFP)

	tree.AddMapping("/", footest.Name(), false)
	tree.AddMapping("/backup", bartest.Name(), true)

	term := NewTreeTerm(tree, &buf)

	if res, err := term.Run("tree"); err != nil || (res != `
/
drwxrwxrwx   0 B   backup
drwxrwx--- 4.0 KiB sub1
-rwxrwx---  10 B   test1
-rwxrwx---  10 B   test2

/backup
-rwxrwx--- 10 B   test1

/sub1
-rwxrwx--- 17 B   test3
`[1:] && res != `
/
drwxrwxrwx   0 B   backup
drwxr-x--- 4.0 KiB sub1
-rwxr-x---  10 B   test1
-rwxr-x---  10 B   test2

/backup
-rwxr-x--- 10 B   test1

/sub1
-rwxr-x--- 17 B   test3
`[1:] && res != `
/
drwxrwxrwx  0 B   backup
drwxrwxrwx  0 B   sub1
-rw-rw-rw- 10 B   test1
-rw-rw-rw- 10 B   test2

/backup
-rw-rw-rw- 10 B   test1

/sub1
-rw-rw-rw- 17 B   test3
`[1:]) {
		t.Error("Unexpected result: ", res, err)
		return
	}

	res, err := term.Run("cat /sub1/test3")
	if err != nil {
		t.Error(err)
		return
	}

	res += buf.String()
	res += "\n"
	buf.Reset()

	if res != `
Sub dir test file
`[1:] {
		t.Error("Unexpected result: ", res)
		return
	}

	// Read file and store as same filename

	res, err = term.Run("get /sub1/test3")
	if err != nil || res != "Written file test3" {
		t.Error(res, err)
		return
	}

	defer os.Remove("test3")

	content, err := ioutil.ReadFile("test3")
	if err != nil || string(content) != "Sub dir test file" {
		t.Error(string(content), err)
		return
	}

	// Read file and store as same different filename - extra path is ignored

	res, err = term.Run("get /sub1/test3 foo42/test123")
	if err != nil || res != "Written file test123" {
		t.Error(res, err)
		return
	}

	defer os.RemoveAll("test123")

	content, err = ioutil.ReadFile("test123")
	if err != nil || string(content) != "Sub dir test file" {
		t.Error(string(content), err)
		return
	}

	_, err = term.Run("cat /sub1/test4")
	if err == nil || (err.Error() != "RufsError: Remote error (stat /sub1/test4: no such file or directory)" &&
		err.Error() != `RufsError: Remote error (CreateFile \sub1\test4: The system cannot find the file specified.)`) {
		t.Error(err)
		return
	}

	// Try writing files - only backup is mounted as writable

	ioutil.WriteFile("testfile66", []byte("write test"), 0660)
	defer os.Remove("testfile66")

	res, err = term.Run("put testfile66 /testfile66")
	if err == nil || err.Error() != "All applicable branches for the requested path were mounted as not writable" {
		t.Error(res, err)
		return
	}

	res, err = term.Run("put testfile66 /backup/testfile66")
	if err != nil || res != "Written file /backup/testfile66" {
		t.Error(res, err)
		return
	}

	if res, err := term.Run("tree /backup"); err != nil || (res != `
/backup
-rwxrwx--- 10 B   test1
-rw-r--r-- 10 B   testfile66
`[1:] && res != `
/backup
-rwxr-x--- 10 B   test1
-rw-r--r-- 10 B   testfile66
`[1:] && res != `
/backup
-rw-rw-rw- 10 B   test1
-rw-rw-rw- 10 B   testfile66
`[1:]) {
		t.Error("Unexpected result: ", res, err)
		return
	}

	res, err = term.Run("cat /backup/testfile66")
	if err != nil {
		t.Error(err)
		return
	}

	res += buf.String()
	res += "\n"
	buf.Reset()

	if res != `
write test
`[1:] {
		t.Error("Unexpected result: ", res)
		return
	}

	if res := dirLocal("./bar"); res != `
test1
testfile66
`[1:] {
		t.Error("Unexpected result:", res)
		return
	}

	tree.AddMapping("/backup/tmp", tmptest.Name(), true)

	if res, err := term.Run("tree /backup"); err != nil || (res != `
/backup
-rwxrwx--- 10 B   test1
-rw-r--r-- 10 B   testfile66
drwxrwxrwx  0 B   tmp

/backup/tmp
`[1:] && res != `
/backup
-rwxr-x--- 10 B   test1
-rw-r--r-- 10 B   testfile66
drwxrwxrwx  0 B   tmp

/backup/tmp
`[1:] && res != `
/backup
-rw-rw-rw- 10 B   test1
-rw-rw-rw- 10 B   testfile66
drwxrwxrwx  0 B   tmp

/backup/tmp
`[1:]) {
		t.Error("Unexpected result: ", res, err)
		return
	}

	res, err = term.Run("put testfile66 /backup/tmp/foofile66")
	if err != nil || res != "Written file /backup/tmp/foofile66" {
		t.Error(res, err)
		return
	}

	// Check that tmp has become a real directory as it was created in backup

	if res, err := term.Run("tree /backup"); err != nil || (res != `
/backup
-rwxrwx---  10 B   test1
-rw-r--r--  10 B   testfile66
drwxr-xr-x 4.0 KiB tmp

/backup/tmp
-rw-r--r-- 10 B   foofile66
`[1:] && res != `
/backup
-rwxr-x---  10 B   test1
-rw-r--r--  10 B   testfile66
drwxr-xr-x 4.0 KiB tmp

/backup/tmp
-rw-r--r-- 10 B   foofile66
`[1:] && res != `
/backup
-rw-rw-rw- 10 B   test1
-rw-rw-rw- 10 B   testfile66
drwxrwxrwx  0 B   tmp

/backup/tmp
-rw-rw-rw- 10 B   foofile66
`[1:]) {
		t.Error("Unexpected result: ", res, err)
		return
	}

	// The file should have been now written to two files

	if res := dirLocal("./bar"); res != `
test1
testfile66
tmp(dir)
`[1:] {
		t.Error("Unexpected result:", res)
		return
	}

	if res := dirLocal("./bar/tmp"); res != `
foofile66
`[1:] {
		t.Error("Unexpected result:", res)
		return
	}

	if res := dirLocal("./tmp"); res != `
foofile66
`[1:] {
		t.Error("Unexpected result:", res)
		return
	}

	// Delete one of the files

	os.RemoveAll("bar/tmp")

	res, err = term.Run("rm /backup/tmp/foofile66")
	if err != nil || res != "" {
		t.Error(res, err)
		return
	}

	// See that the file was deleted

	if res := dirLocal("./tmp"); res != `
`[1:] {
		t.Error("Unexpected result:", res)
		return
	}

	// Recreate the files

	res, err = term.Run("put testfile66 /backup/tmp/foofile66")
	if err != nil || res != "Written file /backup/tmp/foofile66" {
		t.Error(res, err)
		return
	}

	if res := dirLocal("./bar/tmp"); res != `
foofile66
`[1:] {
		t.Error("Unexpected result:", res)
		return
	}

	if res := dirLocal("./tmp"); res != `
foofile66
`[1:] {
		t.Error("Unexpected result:", res)
		return
	}

	res, err = term.Run("rm /backup/tmp1/")
	if err == nil || err.Error() != "RufsError: Remote error (file does not exist)" {
		t.Error("Unexpected result:", res, err)
		return
	}

	res, err = term.Run("rm /backup/tmp1")
	if err == nil || err.Error() != "RufsError: Remote error (file does not exist)" {
		t.Error("Unexpected result:", res, err)
		return
	}

	res, err = term.Run("rm /backup/*")
	if err != nil || res != "" {
		t.Error(res, err)
		return
	}

	if res, err := term.Run("tree /backup"); err != nil || (res != `
/backup
drwxrwxrwx 0 B   tmp

/backup/tmp
-rw-r--r-- 10 B   foofile66
`[1:] && res != `
/backup
drwxrwxrwx 0 B   tmp

/backup/tmp
-rw-rw-rw- 10 B   foofile66
`[1:]) {
		t.Error("Unexpected result: ", res, err)
		return
	}

	// Now try to delete recursive

	res, err = term.Run("rm /backup/**")
	if err != nil || res != "" {
		t.Error(res, err)
		return
	}

	if res, err := term.Run("tree /backup"); err != nil || (res != `
/backup
drwxrwxrwx 0 B   tmp

/backup/tmp
`[1:] && res != `
/backup
sdrwxrwxrwx 0 KiB tmp

/backup/tmp
`[1:]) {
		t.Error("Unexpected result: ", res, err)
		return
	}

	// Recreate the files

	res, err = term.Run("put testfile66 /backup/tmp/foofile66")
	if err != nil || res != "Written file /backup/tmp/foofile66" {
		t.Error(res, err)
		return
	}

	res, err = term.Run("put testfile66 /backup/foofile66")
	if err != nil || res != "Written file /backup/foofile66" {
		t.Error(res, err)
		return
	}

	// Delete with wildcard

	res, err = term.Run("rm /backup/foo**")
	if err != nil || res != "" {
		t.Error(res, err)
		return
	}

	// Recreate files

	res, err = term.Run("put testfile66 /backup/tmp/foofile66")
	if err != nil || res != "Written file /backup/tmp/foofile66" {
		t.Error(res, err)
		return
	}

	if res, err := term.Run("tree /backup"); err != nil || (res != `
/backup
drwxr-xr-x 4.0 KiB tmp

/backup/tmp
-rw-r--r-- 10 B   foofile66
`[1:] && res != `
/backup
drwxrwxrwx 0 B   tmp

/backup/tmp
-rw-rw-rw- 10 B   foofile66
`[1:]) {
		t.Error("Unexpected result: ", res, err)
		return
	}

	res, err = term.Run("ren /backup/tmp")
	if err == nil || err.Error() != "ren requires a filename and a new filename" {
		t.Error(res, err)
		return
	}

	res, err = term.Run("ren /backup/tmp /tmp/t")
	if err == nil || err.Error() != "new filename must not have a path" {
		t.Error(res, err)
		return
	}

	res, err = term.Run("ren /backup/tmp tmp2")
	if err != nil || res != "" {
		t.Error(res, err)
		return
	}

	res, err = term.Run("ren /backup/tmp2/foofile66/ foofile67")
	if err != nil || res != "" {
		t.Error(res, err)
		return
	}

	res, err = term.Run("mkdir /backup/tmp2/aaa/bbb/")
	if err != nil || res != "" {
		t.Error(res, err)
		return
	}

	res, err = term.Run("cp /backup/tmp2/foofile67 /backup/tmp/")
	if err != nil || res != "Done" {
		t.Error(res, err)
		return
	}

	if buf.String() != "\rCopy /foofile67: 10 B / 10 B (1 of 1)\r                                     \r" {
		t.Errorf("Unexpected buffer: %#v", buf.String())
		return
	}

	res, err = term.Run("cp /backup/tmp2/foofile68 /backup/tmp/foofile67")
	if err == nil || err.Error() != "Cannot stat /backup/tmp2/foofile68: RufsError: Remote error (file does not exist)" {
		t.Error(res, err)
		return
	}

	res, err = term.Run("cp /backup/tmp2/foofile67 /foofile67")
	if err == nil || err.Error() != "Cannot copy /backup/tmp2/foofile67 to /foofile67: All applicable branches for the requested path were mounted as not writable" {
		t.Error(res, err)
		return
	}

	if res, err := term.Run("tree /backup"); err != nil || (res != `
/backup
drwxr-xr-x 4.0 KiB tmp
drwxr-xr-x 4.0 KiB tmp2

/backup/tmp
-rw-r--r-- 10 B   foofile66
-rw-r--r-- 10 B   foofile67

/backup/tmp2
drwxr-xr-x 4.0 KiB aaa
-rw-r--r--  10 B   foofile67

/backup/tmp2/aaa
drwxr-xr-x 4.0 KiB bbb

/backup/tmp2/aaa/bbb
`[1:] && res != `
/backup
drwxrwxrwx 0 B   tmp
drwxrwxrwx 0 B   tmp2

/backup/tmp
-rw-rw-rw- 10 B   foofile66
-rw-rw-rw- 10 B   foofile67

/backup/tmp2
drwxrwxrwx 0  B   aaa
-rw-rw-rw- 10 B   foofile67

/backup/tmp2/aaa
drwxrwxrwx 0 B   bbb

/backup/tmp2/aaa/bbb
`[1:]) {
		t.Error("Unexpected result: ", res, err)
		return
	}

	os.Remove("./tmp/foofile66")
	os.Remove("./tmp/foofile67")

	if res := dirLocal("./tmp"); res != `
`[1:] {
		t.Error("Unexpected result:", res)
		return
	}

	if res := dirLocal("./foo"); res != `
sub1(dir)
test1
test2
`[1:] {
		t.Error("Unexpected result:", res)
		return
	}

	if res := dirLocal("./foo/sub1"); res != `
test3
`[1:] {
		t.Error("Unexpected result:", res)
		return
	}

	os.RemoveAll("./bar/tmp")
	os.RemoveAll("./bar/tmp2")
	ioutil.WriteFile("bar/test1", []byte("Test3 file"), 0770)

	if res := dirLocal("./bar"); res != `
test1
`[1:] {
		t.Error("Unexpected result:", res)
		return
	}

}

/*
dirLocal reads a local directory and returns all found file names as a string.
*/
func dirLocal(dir string) string {
	var buf bytes.Buffer

	fis, err := ioutil.ReadDir(dir)
	errorutil.AssertOk(err)

	for _, fi := range fis {
		buf.WriteString(fi.Name())
		if fi.IsDir() {
			buf.WriteString("(dir)")
		}
		buf.WriteString(fmt.Sprintln())
	}

	return buf.String()
}
