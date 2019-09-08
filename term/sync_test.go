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

	"devt.de/krotik/rufs"
	"devt.de/krotik/rufs/config"
)

func TestSyncOperation(t *testing.T) {
	var buf bytes.Buffer

	// Build up a tree with multiple branches

	cfg := map[string]interface{}{
		config.TreeSecret: "123",
	}

	ioutil.WriteFile("./bar/testfile66", []byte("write test"), 0660)
	defer os.Remove("./bar/testfile66")

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

	tree.AddMapping("/1", footest.Name(), false)
	tree.AddMapping("/1", bartest.Name(), false)
	tree.AddMapping("/2", tmptest.Name(), true)

	term := NewTreeTerm(tree, &buf)

	if res, err := term.Run("tree"); err != nil || (res != `
/
drwxrwxrwx 0 B   1
drwxrwxrwx 0 B   2

/1
drwxrwx--- 4.0 KiB sub1
-rwxrwx---  10 B   test1
-rwxrwx---  10 B   test2
-rw-rw----  10 B   testfile66

/1/sub1
-rwxrwx--- 17 B   test3

/2
`[1:] && res != `
/
drwxrwxrwx 0 B   1
drwxrwxrwx 0 B   2

/1
drwxr-x--- 4.0 KiB sub1
-rwxr-x---  10 B   test1
-rwxr-x---  10 B   test2
-rw-r-----  10 B   testfile66

/1/sub1
-rwxr-x--- 17 B   test3

/2
`[1:] && res != `
/
drwxrwxrwx 0 B   1
drwxrwxrwx 0 B   2

/1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2
-rw-rw-rw-  10 B   testfile66

/1/sub1
-rw-rw-rw- 17 B   test3

/2
`[1:]) {
		t.Error("Unexpected result: ", res, err)
		return
	}

	res, err := term.Run("sync /1 /2")
	if err != nil || res != "Done" {
		t.Error(res, err)
		return
	}

	if buf.String() != "Create directory (1/5)  -> /2/sub1\n\r"+
		"Copy file (2/5) writing: /1/test1 -> /2/test1 10 B / 10 B\r                                                         \r"+
		"Copy file (2/5) /1/test1 -> /2/test1\n\r"+
		"Copy file (3/5) writing: /1/test2 -> /2/test2 10 B / 10 B\r                                                         \r"+
		"Copy file (3/5) /1/test2 -> /2/test2\n\r"+
		"Copy file (4/5) writing: /1/testfile66 -> /2/testfile66 10 B / 10 B\r                                                                   \r"+
		"Copy file (4/5) /1/testfile66 -> /2/testfile66\n\r"+
		"Copy file (5/5) writing: /1/sub1/test3 -> /2/sub1/test3 17 B / 17 B\r                                                                   \r"+
		"Copy file (5/5) /1/sub1/test3 -> /2/sub1/test3\n" {
		t.Errorf("Unexpected buffer: %#v", buf.String())
		return
	}

	if res, err := term.Run("tree"); err != nil || (res != `
/
drwxrwxrwx 0 B   1
drwxrwxrwx 0 B   2

/1
drwxrwx--- 4.0 KiB sub1
-rwxrwx---  10 B   test1
-rwxrwx---  10 B   test2
-rw-rw----  10 B   testfile66

/1/sub1
-rwxrwx--- 17 B   test3

/2
drwxr-xr-x 4.0 KiB sub1
-rw-r--r--  10 B   test1
-rw-r--r--  10 B   test2
-rw-r--r--  10 B   testfile66

/2/sub1
-rw-r--r-- 17 B   test3
`[1:] && res != `
/
drwxrwxrwx 0 B   1
drwxrwxrwx 0 B   2

/1
drwxr-x--- 4.0 KiB sub1
-rwxr-x---  10 B   test1
-rwxr-x---  10 B   test2
-rw-r-----  10 B   testfile66

/1/sub1
-rwxr-x--- 17 B   test3

/2
drwxr-xr-x 4.0 KiB sub1
-rw-r--r--  10 B   test1
-rw-r--r--  10 B   test2
-rw-r--r--  10 B   testfile66

/2/sub1
-rw-r--r-- 17 B   test3
`[1:] && res != `
/
drwxrwxrwx 0 B   1
drwxrwxrwx 0 B   2

/1
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2
-rw-rw-rw-  10 B   testfile66

/1/sub1
-rw-rw-rw- 17 B   test3

/2
drwxrwxrwx 4.0 KiB sub1
-rw-rw-rw-  10 B   test1
-rw-rw-rw-  10 B   test2
-rw-rw-rw-  10 B   testfile66

/2/sub1
-rw-rw-rw- 17 B   test3
`[1:]) {
		t.Error("Unexpected result: ", res, err)
		return
	}
}
