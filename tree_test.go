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
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"devt.de/krotik/common/bitutil"
	"devt.de/krotik/common/errorutil"
	"devt.de/krotik/common/pools"
	"devt.de/krotik/rufs/config"
	"devt.de/krotik/rufs/node"
)

func TestRefresh(t *testing.T) {

	// Build up a tree from one branch

	cfg := map[string]interface{}{
		config.TreeSecret: "123",
	}

	tree, _ := NewTree(cfg, clientCert)

	branchRPC := fmt.Sprintf("%v:%v", branchConfigs["footest"][config.RPCHost], branchConfigs["footest"][config.RPCPort])

	if fp, err := tree.PingBranch("footest", branchRPC); err != nil || fp != footest.SSLFingerprint() {
		t.Error("Unexpected result:", fp, err)
		return
	}

	tree.AddBranch("footest", branchRPC, "")

	tree.AddMapping("/", "footest", true)

	paths, infos, err := tree.Dir("/", "", true, false)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2

/sub1
-rw-rw-rw- 17 B   test3
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	tree.Refresh()

	paths, infos, err = tree.Dir("/", "", true, false)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2

/sub1
-rw-rw-rw- 17 B   test3
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	// Simulate that the branch is no longer reachable

	tree.client.RemovePeer("footest")
	tree.client.RegisterPeer("footest", ":9095", "")

	if _, _, err = tree.Dir("/", "", true, false); err == nil {
		t.Error("Dir call should fail.")
		return
	}

	tree.Refresh()

	if res := tree.NotReachableBranches(); len(res) != 1 {
		t.Error("Unexpected result:", res)
		return
	}

	if res, _ := tree.ActiveBranches(); len(res) != 0 {
		t.Error("Unexpected result:", res)
		return
	}

	if res := tree.KnownBranches(); len(res) != 1 {
		t.Error("Unexpected result:", res)
		return
	}

	// Restore the branch again

	tree.client.RemovePeer("footest")
	tree.client.RegisterPeer("footest", branchRPC, "")

	tree.Refresh()

	paths, infos, err = tree.Dir("/", "", true, false)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2

/sub1
-rw-rw-rw- 17 B   test3
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

}

func TestItemOpWrite(t *testing.T) {

	// Build up a tree from one branch

	cfg := map[string]interface{}{
		config.TreeSecret: "123",
	}

	tree, _ := NewTree(cfg, clientCert)

	branchRPC := fmt.Sprintf("%v:%v", branchConfigs["footest"][config.RPCHost], branchConfigs["footest"][config.RPCPort])

	if fp, err := tree.PingBranch("footest", branchRPC); err != nil || fp != footest.SSLFingerprint() {
		t.Error("Unexpected result:", fp, err)
		return
	}

	tree.AddBranch("footest", branchRPC, "")

	if res := fmt.Sprint(tree.ActiveBranches()); res != "[footest] ["+footest.SSLFingerprint()+"]" {
		t.Error("Unexpected result:", res)
		return
	}

	if err := tree.AddMapping("/", "bla", true); err == nil || err.Error() != "Unknown target node" {
		t.Error("Unexpected result:", err)
		return
	}

	tree.AddMapping("/", "footest", true)
	tree.AddMapping("/sub1", "footest", false)

	paths, infos, err := tree.Dir("/", "", true, false)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2

/sub1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2
-rw-rw-rw-  17 B   test3

/sub1/sub1
-rw-rw-rw- 17 B   test3
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	// Make the byteSlicePool byte slices very small

	oldDefaultReadBufferSize := DefaultReadBufferSize
	defer func() {
		DefaultReadBufferSize = oldDefaultReadBufferSize
		byteSlicePool = pools.NewByteSlicePool(DefaultReadBufferSize)
	}()
	DefaultReadBufferSize = 3
	byteSlicePool = pools.NewByteSlicePool(DefaultReadBufferSize)

	bb := bytes.Buffer{}

	n, err := tree.WriteFile("/test99", []byte("Test"), 5)
	defer func() {

		// Make sure the written files are deleted

		footest.ItemOp("/", map[string]string{
			ItemOpAction: ItemOpActDelete,
			ItemOpName:   "test99",
		})
	}()
	if err != nil || n != 4 {
		t.Error(n, err)
		return
	}

	if footest.ReadFileToBuffer("/test99", &bb); bb.String() !=
		string([]byte{0, 0, 0, 0, 0, 84, 101, 115, 116}) {
		t.Error("Unexpected result:", bb.Bytes())
		return
	}
	bb.Reset()

	n, err = footest.WriteFile("/test99", []byte("oo"), 1)
	if err != nil || n != 2 {
		t.Error(n, err)
		return
	}

	tree.WriteFile("/test99", []byte("oo"), 11)

	if footest.ReadFileToBuffer("/test99", &bb); bb.String() !=
		string([]byte{0, 111, 111, 0, 0, 84, 101, 115, 116, 0, 0, 111, 111}) {
		t.Error("Unexpected result:", bb.Bytes())
		return
	}
	bb.Reset()

	n, err = footest.WriteFile("/test98", []byte("Test"), 0)
	defer func() {

		// Make sure the written files are deleted

		footest.ItemOp("/", map[string]string{
			ItemOpAction: ItemOpActDelete,
			ItemOpName:   "test98",
		})
	}()
	if err != nil || n != 4 {
		t.Error(n, err)
		return
	}

	if footest.ReadFileToBuffer("/test98", &bb); bb.String() != string("Test") {
		t.Error("Unexpected result:", bb.Bytes())
		return
	}
	bb.Reset()

	bb = *bytes.NewBuffer([]byte("Test buffer file"))

	err = footest.WriteFileFromBuffer("/test97", &bb)
	defer func() {

		// Make sure the written files are deleted

		footest.ItemOp("/", map[string]string{
			ItemOpAction: ItemOpActDelete,
			ItemOpName:   "test97",
		})
	}()

	if err != nil {
		t.Error(err)
		return
	}

	if footest.ReadFileToBuffer("/test97", &bb); bb.String() != string("Test buffer file") {
		t.Error("Unexpected result:", bb.Bytes())
		return
	}
	bb.Reset()

	bb = *bytes.NewBuffer([]byte("Test buffer file2"))

	err = tree.WriteFileFromBuffer("/test96", &bb)
	defer func() {

		// Make sure the written files are deleted

		footest.ItemOp("/", map[string]string{
			ItemOpAction: ItemOpActDelete,
			ItemOpName:   "test96",
		})
	}()

	if err != nil {
		t.Error(err)
		return
	}

	if footest.ReadFileToBuffer("/test96", &bb); bb.String() != string("Test buffer file2") {
		t.Error("Unexpected result:", bb.Bytes())
		return
	}
	bb.Reset()

	// Create an empty file

	tree.WriteFileFromBuffer("/test95", &bb)
	defer func() {

		// Make sure the written files are deleted

		footest.ItemOp("/", map[string]string{
			ItemOpAction: ItemOpActDelete,
			ItemOpName:   "test95",
		})
	}()

	if res := dirLocal("./foo"); res != `
sub1(dir)
test1(10)
test2(10)
test95(0)
test96(17)
test97(16)
test98(4)
test99(13)
`[1:] {
		t.Error("Unexpected result:", res)
		return
	}
}

func TestItemOpRead(t *testing.T) {

	// Build up a tree from two brances

	cfg := map[string]interface{}{
		config.TreeSecret: "123",
	}

	tree, _ := NewTree(cfg, clientCert)

	branchRPC := fmt.Sprintf("%v:%v", branchConfigs["footest"][config.RPCHost], branchConfigs["footest"][config.RPCPort])
	tree.AddBranch("footest", branchRPC, "")

	tree.AddMapping("/1", "footest", false)
	tree.AddMapping("/1/in/in", "footest", true)

	// We should now have the following structure:
	//
	// /1/test1
	// /1/test2
	// /1/sub1/test3
	// /1/in/in/test1
	// /1/in/in/test2
	// /1/in/in/sub1/test3

	paths, infos, err := tree.Dir("/", "", true, false)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 0 B   1

/1
drwxrwxrwx   0 B   in
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2

/1/in
drwxrwxrwx 0 B   in

/1/in/in
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2

/1/in/in/sub1
-rw-rw-rw- 17 B   test3

/1/sub1
-rw-rw-rw- 17 B   test3
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	paths, infos, err = tree.Dir("/1", "", false, false)
	if res := DirResultToString(paths, infos); err != nil || res != `
/1
drwxrwxrwx   0 B   in
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	// Make the byteSlicePool byte slices very small

	oldDefaultReadBufferSize := DefaultReadBufferSize
	defer func() {
		DefaultReadBufferSize = oldDefaultReadBufferSize
		byteSlicePool = pools.NewByteSlicePool(DefaultReadBufferSize)
	}()
	DefaultReadBufferSize = 3
	byteSlicePool = pools.NewByteSlicePool(DefaultReadBufferSize)

	b := make([]byte, 20)

	n, err := tree.ReadFile("/1/test1", b, 0)
	if err != nil {
		t.Error(err)
		return
	}

	// Despite the small buffer size we should have read everything!

	if n != 10 || string(b[:n]) != "Test1 file" {
		t.Error("Unexpected result:", n, string(b[:n]))
		return
	}

	b = make([]byte, 3)

	n, err = tree.ReadFile("/1/test1", b, 0)
	if err != nil {
		t.Error(err)
		return
	}

	if n != 3 || string(b[:n]) != "Tes" {
		t.Error("Unexpected result:", n, string(b))
		return
	}

	// Try to read outside of root path

	_, err = tree.ReadFile("/1/../../test1", b, 0)
	if err == nil || err.Error() != "RufsError: Remote error (Requested path ../../test1 is outside of the branch)" {
		t.Error(err)
		return
	}

	// Do offset reading

	n, err = tree.ReadFile("/1/test1", b, 3)
	if err != nil {
		t.Error(err)
		return
	}

	if n != 3 || string(b[:n]) != "t1 " {
		t.Error("Unexpected result:", n, string(b))
		return
	}

	_, err = tree.ReadFile("/1/test1", b, 10)
	if err.(*node.Error).Detail != io.EOF.Error() {
		t.Error(err)
		return
	}

	byteSlicePool = pools.NewByteSlicePool(DefaultReadBufferSize)

	// Now read the same file using a reader

	bb := bytes.Buffer{}

	if err = tree.ReadFileToBuffer("/1/test2", &bb); err != nil {
		t.Error(err)
		return
	}

	if bb.String() != "Test2 file" {
		t.Error("Unexpected result: ", bb.String())
		return
	}

	bb.Reset()
	if err = footest.ReadFileToBuffer("test2", &bb); err != nil {
		t.Error(err)
		return
	}

	if bb.String() != "Test2 file" {
		t.Error("Unexpected result: ", bb.String())
		return
	}

	// Test error return

	_, err = tree.ReadFile("/1/sub1", []byte{0}, 0)
	if err == nil || err.Error() != "RufsError: Remote error (read /sub1: is a directory)" {
		t.Error("Unexpected result:", err)
		return
	}

	err = tree.ReadFileToBuffer("/1/sub1", &bb)

	if err == nil || err.Error() != "RufsError: Remote error (read /sub1: is a directory)" {
		t.Error("Unexpected result:", err)
		return
	}
}

func TestTreeDir(t *testing.T) {

	// Test the branch output

	paths, infos, err := footest.Dir("/", "", false, false)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	paths, infos, err = footest.Dir("/", "", true, false)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2

/sub1
-rw-rw-rw- 17 B   test3
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	// Build up a tree from one branch

	cfg := map[string]interface{}{
		config.TreeSecret: "123",
	}

	tree, _ := NewTree(cfg, clientCert)

	branchRPC := fmt.Sprintf("%v:%v", branchConfigs["footest"][config.RPCHost], branchConfigs["footest"][config.RPCPort])

	// Test errors for adding branches

	if err := tree.AddBranch("footest", branchRPC, "123"); err == nil ||
		!strings.HasPrefix(err.Error(), "Remote branch has an unexpected fingerprint") {
		t.Error(err)
		return
	}

	if err := tree.AddBranch("my", branchRPC, ""); err == nil ||
		err.Error() != "RufsError: Remote error (Unknown target node)" {
		t.Error(err)
		return
	}

	if err := tree.AddBranch("footest", branchRPC, ""); err != nil {
		t.Error(err)
		return
	}

	tree.AddMapping("/1", "footest", false)
	tree.AddMapping("/1/sub1", "footest", false)

	// We should now have the following structure:
	//
	// /1/test1
	// /1/test2
	// /1/sub1/test3
	// /1/sub1/test1
	// /1/sub1/test2
	// /1/sub1/sub1/test3

	paths, infos, err = tree.Dir("/", "", true, false)

	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 0 B   1

/1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2

/1/sub1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2
-rw-rw-rw-  17 B   test3

/1/sub1/sub1
-rw-rw-rw- 17 B   test3
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	// Add another mapping

	branchRPC = fmt.Sprintf("%v:%v", branchConfigs["bartest"][config.RPCHost], branchConfigs["bartest"][config.RPCPort])
	tree.AddBranch("bartest", branchRPC, "")
	tree.AddMapping("/2/sub2", "bartest", false)

	paths, infos, err = tree.Dir("/", "", true, false)

	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 0 B   1
drwxrwxrwx 0 B   2

/1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2

/1/sub1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2
-rw-rw-rw-  17 B   test3

/1/sub1/sub1
-rw-rw-rw- 17 B   test3

/2
drwxrwxrwx 0 B   sub2

/2/sub2
-rw-rw-rw- 10 B   test1
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	// Now overlay a files - test1 of bartest should be overlaid by the
	// existing test1 from footest

	tree.AddMapping("/1", "bartest", false)

	paths, infos, err = tree.Dir("/", "", true, false)

	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 0 B   1
drwxrwxrwx 0 B   2

/1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2

/1/sub1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2
-rw-rw-rw-  17 B   test3

/1/sub1/sub1
-rw-rw-rw- 17 B   test3

/2
drwxrwxrwx 0 B   sub2

/2/sub2
-rw-rw-rw- 10 B   test1
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	paths, infos, err = tree.Dir("/1", "", false, false)

	if res := DirResultToString(paths, infos); err != nil || res != `
/1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	paths, infos, err = tree.Dir("/1/sub1", "", false, false)

	if res := DirResultToString(paths, infos); err != nil || res != `
/1/sub1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2
-rw-rw-rw-  17 B   test3
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

}

func TestItemOpRename(t *testing.T) {

	// Build up a tree from one branch

	cfg := map[string]interface{}{
		config.TreeSecret: "123",
	}

	tree, _ := NewTree(cfg, clientCert)

	branchRPC := fmt.Sprintf("%v:%v", branchConfigs["footest"][config.RPCHost], branchConfigs["footest"][config.RPCPort])
	tree.AddBranch("footest", branchRPC, "")

	tree.AddMapping("/1", "footest", false)
	tree.AddMapping("/1/sub1", "footest", true)

	// We should now have the following structure:
	//
	// /1/test1
	// /1/test2
	// /1/sub1/test3
	// /1/sub1/test1
	// /1/sub1/test2
	// /1/sub1/sub1/test3

	if res := fmt.Sprint(tree); res != `
/: 
  1/: footest(r)
    sub1/: footest(w)
`[1:] {
		t.Error("Unexpected result:", res)
		return
	}

	paths, infos, err := footest.Dir("/", "", true, false)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2

/sub1
-rw-rw-rw- 17 B   test3
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	paths, infos, err = tree.Dir("/", "", true, false)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 0 B   1

/1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2

/1/sub1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2
-rw-rw-rw-  17 B   test3

/1/sub1/sub1
-rw-rw-rw- 17 B   test3
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	// Rename test1 to test111

	tree.ItemOp("/1/sub1", map[string]string{
		ItemOpAction:  ItemOpActRename,
		ItemOpName:    "test1",
		ItemOpNewName: "test111",
	})

	tree.ItemOp("/1/sub1", map[string]string{
		ItemOpAction:  ItemOpActRename,
		ItemOpName:    "sub1",
		ItemOpNewName: "sub99",
	})

	paths, infos, err = footest.Dir("/", "", true, false)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 4.0 KiB sub99
-rw-rw-rw-  10 B   test111
-rw-rw-rw-  10 B   test2

/sub99
-rw-rw-rw- 17 B   test3
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	paths, infos, err = tree.Dir("/", "", true, false)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 0 B   1

/1
drwxrwxrwx   0 B   sub1
drwxrwxrwx 4.0 KiB sub99
-rw-rw-rw-  10 B   test111
-rw-rw-rw-  10 B   test2

/1/sub1
drwxrwxrwx 4.0 KiB sub99
-rw-rw-rw-  10 B   test111
-rw-rw-rw-  10 B   test2

/1/sub1/sub99
-rw-rw-rw- 17 B   test3

/1/sub99
-rw-rw-rw- 17 B   test3
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	// Rename test111 back to test1

	ok, err := tree.ItemOp("/1/sub1", map[string]string{
		ItemOpAction:  ItemOpActRename,
		ItemOpName:    "test111",
		ItemOpNewName: "test1",
	})

	if !ok || err != nil {
		t.Error(ok, err)
		return
	}

	ok, err = tree.ItemOp("/1/sub1", map[string]string{
		ItemOpAction:  ItemOpActRename,
		ItemOpName:    "sub99",
		ItemOpNewName: "sub1",
	})

	if !ok || err != nil {
		t.Error(ok, err)
		return
	}

	// Rename non existing file

	ok, err = tree.ItemOp("/1/sub1", map[string]string{
		ItemOpAction:  ItemOpActRename,
		ItemOpName:    "foobar",
		ItemOpNewName: "test99",
	})

	if ok || err == nil ||
		(err.Error() != "RufsError: Remote error (rename /foobar /test99: no such file or directory)" &&
			err.Error() != "RufsError: Remote error (rename \\foobar \\test99: The system cannot find the file specified.)") {
		t.Error(ok, err)
		return
	}

	paths, infos, err = footest.Dir("/", "", true, false)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2

/sub1
-rw-rw-rw- 17 B   test3
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	paths, infos, err = tree.Dir("/", "", true, false)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 0 B   1

/1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2

/1/sub1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2
-rw-rw-rw-  17 B   test3

/1/sub1/sub1
-rw-rw-rw- 17 B   test3
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}
}

func TestItemOpDelete(t *testing.T) {

	// Build up a tree from one branch

	cfg := map[string]interface{}{
		config.TreeSecret: "123",
	}

	tree, _ := NewTree(cfg, clientCert)

	branchRPC := fmt.Sprintf("%v:%v", branchConfigs["footest"][config.RPCHost], branchConfigs["footest"][config.RPCPort])
	tree.AddBranch("footest", branchRPC, "")
	branchRPC = fmt.Sprintf("%v:%v", branchConfigs["bartest"][config.RPCHost], branchConfigs["bartest"][config.RPCPort])
	tree.AddBranch("bartest", branchRPC, "")

	tree.AddMapping("/1", "footest", false)
	tree.AddMapping("/1/sub1", "footest", true)
	tree.AddMapping("///1///sub1", "bartest", false)

	conf := tree.Config()
	if conf != `{
  "branches": [
    {
      "branch": "footest",
      "fingerprint": "`+footest.SSLFingerprint()+`",
      "rpc": "localhost:9021"
    },
    {
      "branch": "bartest",
      "fingerprint": "`+bartest.SSLFingerprint()+`",
      "rpc": "localhost:9022"
    }
  ],
  "tree": [
    {
      "branch": "footest",
      "path": "/1",
      "writeable": false
    },
    {
      "branch": "footest",
      "path": "/1/sub1",
      "writeable": true
    },
    {
      "branch": "bartest",
      "path": "///1///sub1",
      "writeable": false
    }
  ]
}` {
		t.Error("Unexpected config:", conf)
		return
	}

	if err := tree.SetMapping(conf); err != nil {
		t.Error(err)
		return
	}

	// We should now have the following structure:
	//
	// /1/test1
	// /1/test2
	// /1/sub1/test3
	// /1/sub1/test1
	// /1/sub1/test2
	// /1/sub1/sub1/test3

	if res := fmt.Sprint(tree); res != `
/: 
  1/: footest(r)
    sub1/: footest(w), bartest(r)
`[1:] {
		t.Error("Unexpected result:", res)
		return
	}

	// Create a new file

	ioutil.WriteFile("foo/test_to_delete", []byte("Test1 file"), 0770)

	paths, infos, err := tree.Dir("/", "", true, true)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 0 B   1

/1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1 [73b8af47]
-rw-rw-rw-  10 B   test2 [b0c1fadd]
-rw-rw-rw-  10 B   test_to_delete [73b8af47]

/1/sub1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1 [73b8af47]
-rw-rw-rw-  10 B   test2 [b0c1fadd]
-rw-rw-rw-  17 B   test3 [f89782b1]
-rw-rw-rw-  10 B   test_to_delete [73b8af47]

/1/sub1/sub1
-rw-rw-rw- 17 B   test3 [f89782b1]
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	// Delete file

	ok, err := tree.ItemOp("/1", map[string]string{
		ItemOpAction: ItemOpActDelete,
		ItemOpName:   "test_to_delete",
	})

	if ok || err == nil || err.Error() != "All applicable branches for the requested path were mounted as not writable" {
		t.Error(ok, err)
		return
	}

	_, err = tree.WriteFile("/1/bla", []byte("test"), 0)

	if err == nil || err.Error() != "All applicable branches for the requested path were mounted as not writable" {
		t.Error(ok, err)
		return
	}

	ok, err = tree.ItemOp("/1/sub1", map[string]string{
		ItemOpAction: ItemOpActDelete,
		ItemOpName:   "test_to_delete",
	})

	if !ok || err != nil {
		t.Error(ok, err)
		return
	}

	paths, infos, err = tree.Dir("/", "", true, true)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 0 B   1

/1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1 [73b8af47]
-rw-rw-rw-  10 B   test2 [b0c1fadd]

/1/sub1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1 [73b8af47]
-rw-rw-rw-  10 B   test2 [b0c1fadd]
-rw-rw-rw-  17 B   test3 [f89782b1]

/1/sub1/sub1
-rw-rw-rw- 17 B   test3 [f89782b1]
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	tree.Reset(false) // Just reset mappings not known branches

	tree.AddMapping("/1", "footest", false)
	tree.AddMapping("/1/sub1", "footest", true)
	tree.AddMapping("/1/sub1", "bartest", true)

	// Create new files to delete

	ioutil.WriteFile("foo/test_to_delete1", []byte("Test1 file"), 0770)
	ioutil.WriteFile("bar/test_to_delete2", []byte("Test1 file"), 0770)

	// Delete non existing file

	ok, err = tree.ItemOp("/1/sub1", map[string]string{
		ItemOpAction: ItemOpActDelete,
		ItemOpName:   "test_to_delete3",
	})

	if err == nil || err.Error() != "RufsError: Remote error (file does not exist)" || ok {
		t.Error("Unexpected result:", ok, err)
		return
	}

	ok, err = tree.ItemOp("/1/sub1", map[string]string{
		ItemOpAction: ItemOpActDelete,
		ItemOpName:   "",
	})

	if err == nil || err.Error() != "RufsError: Remote error (This operation requires a specific file or directory)" || ok {
		t.Error("Unexpected result:", ok, err)
		return
	}

	// Do a wildcard delete

	ok, err = tree.ItemOp("/1/sub1", map[string]string{
		ItemOpAction: ItemOpActDelete,
		ItemOpName:   "test_to_delete*",
	})

	if !ok || err != nil {
		t.Error(ok, err)
		return
	}

	paths, infos, err = tree.Dir("/", "", true, true)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 0 B   1

/1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1 [73b8af47]
-rw-rw-rw-  10 B   test2 [b0c1fadd]

/1/sub1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1 [73b8af47]
-rw-rw-rw-  10 B   test2 [b0c1fadd]
-rw-rw-rw-  17 B   test3 [f89782b1]

/1/sub1/sub1
-rw-rw-rw- 17 B   test3 [f89782b1]
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	tree.Reset(true)

	paths, infos, err = tree.Dir("/", "", true, true)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

}

func TestItemOpMkdir(t *testing.T) {

	// Build up a tree from one branch

	cfg := map[string]interface{}{
		config.TreeSecret: "123",
	}

	tree, _ := NewTree(cfg, clientCert)

	branchRPC := fmt.Sprintf("%v:%v", branchConfigs["footest"][config.RPCHost], branchConfigs["footest"][config.RPCPort])
	tree.AddBranch("footest", branchRPC, "")

	tree.AddMapping("/1", "footest", true)

	paths, infos, err := tree.Dir("/", "", true, true)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 0 B   1

/1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1 [73b8af47]
-rw-rw-rw-  10 B   test2 [b0c1fadd]

/1/sub1
-rw-rw-rw- 17 B   test3 [f89782b1]
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	ok, err := tree.ItemOp("/1/sub1", map[string]string{
		ItemOpAction: ItemOpActMkDir,
		ItemOpName:   "aaa/bbb",
	})

	if !ok || err != nil {
		t.Error(ok, err)
		return
	}

	paths, infos, err = tree.Dir("/", "", true, true)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 0 B   1

/1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1 [73b8af47]
-rw-rw-rw-  10 B   test2 [b0c1fadd]

/1/sub1
drwxrwxrwx 4.0 KiB bbb
-rw-rw-rw-  17 B   test3 [f89782b1]

/1/sub1/bbb
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	errorutil.AssertOk(os.RemoveAll("./foo/sub1/bbb"))
}

func TestRelPath(t *testing.T) {

	if res := relPath("/1/", ""); res != "/1" {
		t.Error("Unexpected result:", res)
		return
	}

	if res := relPath("/1/", "/1/"); res != "/" {
		t.Error("Unexpected result:", res)
		return
	}

	if res := relPath("/1/test", "/1/"); res != "/test" {
		t.Error("Unexpected result:", res)
		return
	}

	if res := relPath("/1/test/", "/1"); res != "/test" {
		t.Error("Unexpected result:", res)
		return
	}
}

func TestSync(t *testing.T) {
	var buf bytes.Buffer

	// Build up a tree from one branch

	cfg := map[string]interface{}{
		config.TreeSecret: "123",
	}

	tree, _ := NewTree(cfg, clientCert)

	fooRPC := fmt.Sprintf("%v:%v", branchConfigs["footest"][config.RPCHost], branchConfigs["footest"][config.RPCPort])
	barRPC := fmt.Sprintf("%v:%v", branchConfigs["bartest"][config.RPCHost], branchConfigs["bartest"][config.RPCPort])

	errorutil.AssertOk(tree.AddBranch("footest", fooRPC, ""))
	errorutil.AssertOk(tree.AddBranch("bartest", barRPC, ""))

	errorutil.AssertOk(tree.AddMapping("/2", "footest", false))
	errorutil.AssertOk(tree.AddMapping("/3", "bartest", true))

	paths, infos, err := tree.Dir("/", "", true, true)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 0 B   2
drwxrwxrwx 0 B   3

/2
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1 [73b8af47]
-rw-rw-rw-  10 B   test2 [b0c1fadd]

/2/sub1
-rw-rw-rw- 17 B   test3 [f89782b1]

/3
-rw-rw-rw- 10 B   test1 [5b62da0f]
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	ioutil.WriteFile("./bar/test1", []byte("Test1 file"), 0770)
	ioutil.WriteFile("./bar/test2", []byte("Testx file"), 0770)
	ioutil.WriteFile("./bar/test5", []byte("Testx file"), 0770)
	os.Mkdir("./bar/sub2", 0755)
	ioutil.WriteFile("./bar/sub2/test2", []byte("Testx file"), 0770)
	ioutil.WriteFile("./foo/test3", []byte("Testx file"), 0770)
	ioutil.WriteFile("./foo/testempty", nil, 0770)

	paths, infos, err = tree.Dir("/", "", true, true)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 0 B   2
drwxrwxrwx 0 B   3

/2
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1 [73b8af47]
-rw-rw-rw-  10 B   test2 [b0c1fadd]
-rw-rw-rw-  10 B   test3 [91767c28]
-rw-rw-rw-   0 B   testempty

/2/sub1
-rw-rw-rw- 17 B   test3 [f89782b1]

/3
drwxrwxrwx 4.0 KiB sub2
-rw-rw-rw-  10 B   test1 [73b8af47]
-rw-rw-rw-  10 B   test2 [91767c28]
-rw-rw-rw-  10 B   test5 [91767c28]

/3/sub2
-rw-rw-rw- 10 B   test2 [91767c28]
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	updFunc := func(op, srcFile, dstFile string, writtenBytes, totalBytes, currentFile, totalFiles int64) {
		buf.WriteString(fmt.Sprintf("%v %v -> %v", op, srcFile, dstFile))
		if writtenBytes > 0 {
			buf.WriteString(" " + bitutil.ByteSizeString(writtenBytes, false))
			buf.WriteString("/" + bitutil.ByteSizeString(totalBytes, false))
		} else if writtenBytes == -1 {
			buf.WriteString(" finished")
		}
		buf.WriteString("\n")
	}

	if err := tree.Sync("/2", "/3", false, updFunc); err != nil {
		t.Error(buf.String(), err)
		return
	}

	// Check log

	if buf.String() != `
Create directory  -> /3/sub1
Copy file /2/test2 -> /3/test2 10 B/10 B
Copy file /2/test2 -> /3/test2 finished
Copy file /2/test3 -> /3/test3 10 B/10 B
Copy file /2/test3 -> /3/test3 finished
Copy file /2/testempty -> /3/testempty
Remove directory  -> /3/sub2
Remove file  -> /3/test5
`[1:] {
		t.Error("Unexpected log:", buf.String())
		return
	}

	// Check new directory structure - Almost but not quite - sub1 in /3
	// is empty as the call was not recursive

	paths, infos, err = tree.Dir("/", "", true, true)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 0 B   2
drwxrwxrwx 0 B   3

/2
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1 [73b8af47]
-rw-rw-rw-  10 B   test2 [b0c1fadd]
-rw-rw-rw-  10 B   test3 [91767c28]
-rw-rw-rw-   0 B   testempty

/2/sub1
-rw-rw-rw- 17 B   test3 [f89782b1]

/3
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1 [73b8af47]
-rw-rw-rw-  10 B   test2 [b0c1fadd]
-rw-rw-rw-  10 B   test3 [91767c28]
-rw-rw-rw-   0 B   testempty

/3/sub1
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	// Now reset the whole thing and do it recursive this time!

	os.RemoveAll("./bar/test3")
	os.RemoveAll("./bar/sub1")
	os.Mkdir("./bar/sub2", 0755)
	ioutil.WriteFile("./bar/sub2/test2", []byte("Testx file"), 0770)
	ioutil.WriteFile("./bar/test2", []byte("Testx file"), 0770)
	buf.Reset()

	paths, infos, err = tree.Dir("/", "", true, true)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 0 B   2
drwxrwxrwx 0 B   3

/2
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1 [73b8af47]
-rw-rw-rw-  10 B   test2 [b0c1fadd]
-rw-rw-rw-  10 B   test3 [91767c28]
-rw-rw-rw-   0 B   testempty

/2/sub1
-rw-rw-rw- 17 B   test3 [f89782b1]

/3
drwxrwxrwx 4.0 KiB sub2
-rw-rw-rw-  10 B   test1 [73b8af47]
-rw-rw-rw-  10 B   test2 [91767c28]
-rw-rw-rw-   0 B   testempty

/3/sub2
-rw-rw-rw- 10 B   test2 [91767c28]
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	if err := tree.Sync("/2", "/3", true, updFunc); err != nil {
		t.Error(buf.String(), err)
		return
	}

	if buf.String() != `
Create directory  -> /3/sub1
Copy file /2/test2 -> /3/test2 10 B/10 B
Copy file /2/test2 -> /3/test2 finished
Copy file /2/test3 -> /3/test3 10 B/10 B
Copy file /2/test3 -> /3/test3 finished
Remove directory  -> /3/sub2
Copy file /2/sub1/test3 -> /3/sub1/test3 17 B/17 B
Copy file /2/sub1/test3 -> /3/sub1/test3 finished
`[1:] {
		t.Error("Unexpected log:", buf.String())
		return
	}

	// Check new directory structure - Now sub1 in /3
	// is not empty as the call was recursive

	paths, infos, err = tree.Dir("/", "", true, true)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 0 B   2
drwxrwxrwx 0 B   3

/2
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1 [73b8af47]
-rw-rw-rw-  10 B   test2 [b0c1fadd]
-rw-rw-rw-  10 B   test3 [91767c28]
-rw-rw-rw-   0 B   testempty

/2/sub1
-rw-rw-rw- 17 B   test3 [f89782b1]

/3
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1 [73b8af47]
-rw-rw-rw-  10 B   test2 [b0c1fadd]
-rw-rw-rw-  10 B   test3 [91767c28]
-rw-rw-rw-   0 B   testempty

/3/sub1
-rw-rw-rw- 17 B   test3 [f89782b1]
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	// Reset everything

	ioutil.WriteFile("bar/test1", []byte("Test3 file"), 0770)
	os.RemoveAll("./bar/test2")
	os.RemoveAll("./bar/test3")
	os.RemoveAll("./bar/sub1")
	os.RemoveAll("./bar/testempty")
	os.RemoveAll("./foo/test3")
	os.RemoveAll("./foo/testempty")

	// Test error reporting

	buf.Reset()

	if err := tree.Sync("/3", "/2", true, updFunc); err == nil || err.Error() != "All applicable branches for the requested path were mounted as not writable" {
		t.Error(buf.String(), err)
		return
	}

	// Make sure we "bomb out" after the first write attempt

	if buf.String() != `
Copy file /3/test1 -> /2/test1
`[1:] {
		t.Error("Unexpected log:", buf.String())
		return
	}

	// Make sure everything is in the state it should be

	if err := tree.CopyFile("/bla", "/3/xxx", nil); err == nil ||
		err.Error() != "RufsError: Remote error (file does not exist)" {

		t.Error("Unexpected result:", err)
		return
	}

	paths, infos, err = tree.Dir("/", "", true, true)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 0 B   2
drwxrwxrwx 0 B   3

/2
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1 [73b8af47]
-rw-rw-rw-  10 B   test2 [b0c1fadd]

/2/sub1
-rw-rw-rw- 17 B   test3 [f89782b1]

/3
-rw-rw-rw- 10 B   test1 [5b62da0f]
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}
}

func TestDirPattern(t *testing.T) {

	// Build up a tree from one branch

	cfg := map[string]interface{}{
		config.TreeSecret: "123",
	}

	tree, _ := NewTree(cfg, clientCert)

	fooRPC := fmt.Sprintf("%v:%v", branchConfigs["footest"][config.RPCHost], branchConfigs["footest"][config.RPCPort])
	barRPC := fmt.Sprintf("%v:%v", branchConfigs["bartest"][config.RPCHost], branchConfigs["bartest"][config.RPCPort])

	errorutil.AssertOk(tree.AddBranch("footest", fooRPC, ""))
	errorutil.AssertOk(tree.AddBranch("bartest", barRPC, ""))

	errorutil.AssertOk(tree.AddMapping("/2", "footest", false))
	errorutil.AssertOk(tree.AddMapping("/3", "bartest", true))

	paths, infos, err := tree.Dir("/", "", true, true)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 0 B   2
drwxrwxrwx 0 B   3

/2
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1 [73b8af47]
-rw-rw-rw-  10 B   test2 [b0c1fadd]

/2/sub1
-rw-rw-rw- 17 B   test3 [f89782b1]

/3
-rw-rw-rw- 10 B   test1 [5b62da0f]
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	paths, infos, err = tree.Dir("/", "2", true, true)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 0 B   2

/2
-rw-rw-rw- 10 B   test2 [b0c1fadd]

/2/sub1

/3
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	paths, infos, err = tree.Dir("/", "1", true, true)
	if res := DirResultToString(paths, infos); err != nil || res != `
/

/2
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1 [73b8af47]

/2/sub1

/3
-rw-rw-rw- 10 B   test1 [5b62da0f]
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	// Test error case

	if _, _, err := tree.Dir("/", "(", true, true); err == nil || err.Error() != "error parsing regexp: missing closing ): `(`" {
		t.Error("Unexpected result:", err)
		return
	}

}

func TestCopy(t *testing.T) {
	var buf bytes.Buffer

	// Build up a tree from one branch

	cfg := map[string]interface{}{
		config.TreeSecret: "123",
	}

	tree, _ := NewTree(cfg, clientCert)

	fooRPC := fmt.Sprintf("%v:%v", branchConfigs["footest"][config.RPCHost], branchConfigs["footest"][config.RPCPort])
	barRPC := fmt.Sprintf("%v:%v", branchConfigs["bartest"][config.RPCHost], branchConfigs["bartest"][config.RPCPort])

	errorutil.AssertOk(tree.AddBranch("footest", fooRPC, ""))
	errorutil.AssertOk(tree.AddBranch("bartest", barRPC, ""))

	errorutil.AssertOk(tree.AddMapping("/2", "footest", false))
	errorutil.AssertOk(tree.AddMapping("/3", "bartest", true))

	paths, infos, err := tree.Dir("/", "", true, true)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 0 B   2
drwxrwxrwx 0 B   3

/2
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1 [73b8af47]
-rw-rw-rw-  10 B   test2 [b0c1fadd]

/2/sub1
-rw-rw-rw- 17 B   test3 [f89782b1]

/3
-rw-rw-rw- 10 B   test1 [5b62da0f]
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	// Test stat

	if fi, err := tree.Stat("/2/test1"); err != nil || fi.Name() != "test1" {
		t.Error("Unexpected result:", fi, err)
		return
	}

	if fi, err := tree.Stat("/2/test0"); err == nil || err.Error() != "RufsError: Remote error (file does not exist)" {
		t.Error("Unexpected result:", fi, err)
		return
	}

	updFunc := func(file string, writtenBytes, totalBytes, currentFile, totalFiles int64) {
		buf.WriteString(fmt.Sprintf("%v %v/%v (%v of %v)\n", file, writtenBytes, totalBytes, currentFile, totalFiles))
	}

	if err := tree.Copy([]string{"/2", "/2/sub1", "/2/test1"}, "/3/4/5/6", updFunc); err != nil {
		t.Error(buf.String(), err)
		return
	}

	// Check the result

	paths, infos, err = tree.Dir("/", "", true, true)
	if res := DirResultToString(paths, infos); err != nil || res != `
/
drwxrwxrwx 0 B   2
drwxrwxrwx 0 B   3

/2
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1 [73b8af47]
-rw-rw-rw-  10 B   test2 [b0c1fadd]

/2/sub1
-rw-rw-rw- 17 B   test3 [f89782b1]

/3
drwxrwxrwx 4.0 KiB 4
-rw-rw-rw-  10 B   test1 [5b62da0f]

/3/4
drwxrwxrwx 4.0 KiB 5

/3/4/5
drwxrwxrwx 4.0 KiB 6

/3/4/5/6
drwxrwxrwx 4.0 KiB 2
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1 [73b8af47]

/3/4/5/6/2
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1 [73b8af47]
-rw-rw-rw-  10 B   test2 [b0c1fadd]

/3/4/5/6/2/sub1
-rw-rw-rw- 17 B   test3 [f89782b1]

/3/4/5/6/sub1
-rw-rw-rw- 17 B   test3 [f89782b1]
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	// Test error case

	if err := tree.Copy([]string{"/2", "/2/test5"}, "/", updFunc); err == nil ||
		err.Error() != "Cannot stat /2/test5: RufsError: Remote error (file does not exist)" {
		t.Error(buf.String(), err)
		return
	}

	if err := tree.Copy([]string{"/2", "/2/test1"}, "/", updFunc); err == nil ||
		err.Error() != "Cannot copy /2/test1 to /: All applicable branches for the requested path were mounted as not writable" {
		t.Error(buf.String(), err)
		return
	}
}
