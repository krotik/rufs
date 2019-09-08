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
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"devt.de/krotik/common/cryptutil"
	"devt.de/krotik/common/errorutil"
	"devt.de/krotik/common/fileutil"
	"devt.de/krotik/rufs/config"
)

const certdir = "certs" // Directory for certificates
var portCount = 0       // Port assignment counter for Branch ports

var footest, bartest *Branch                            // Branches
var branchConfigs = map[string]map[string]interface{}{} // All branch configs
var clientCert *tls.Certificate

func TestReadOnlyBranch(t *testing.T) {

	x, err := createBranch("footest3", "foo3", true)
	if err != nil {
		t.Error(err)
		return
	}
	defer os.RemoveAll("foo3")

	if err := x.WriteFileFromBuffer("", nil); err == nil || err.Error() != "Branch footest3 is read-only" {
		t.Error("Unepxected result:", err)
		return
	}

	if _, err := x.WriteFile("", nil, 0); err == nil || err.Error() != "Branch footest3 is read-only" {
		t.Error("Unepxected result:", err)
		return
	}

	if _, err := x.ItemOp("", nil); err == nil || err.Error() != "Branch footest3 is read-only" {
		t.Error("Unepxected result:", err)
		return
	}

	if res, _, err := x.Dir("/", "(", false, false); err == nil || err.Error() != "error parsing regexp: missing closing ): `(`" {
		t.Error("Unepxected result:", res, err)
		return
	}
}

func TestTreeTraversal(t *testing.T) {

	// Test create and shutdown

	x, err := createBranch("footest2", "foo2", false)
	if err != nil {
		t.Error(err)
		return
	}

	if footest.SSLFingerprint() == "" {
		t.Error("Branch should have a SSL fingerprint")
		return
	}

	if res := footest.Name(); res != "footest" {
		t.Error("Unexpected result:", res)
		return
	}

	x.Shutdown()

	os.RemoveAll("foo2")

	// Build up a tree from one branch

	cfg := map[string]interface{}{
		config.TreeSecret:     "123",
		config.EnableReadOnly: false,
	}

	tree, _ := NewTree(cfg, clientCert)

	branchRPC := fmt.Sprintf("%v:%v", branchConfigs["footest"][config.RPCHost], branchConfigs["footest"][config.RPCPort])

	if err := tree.AddBranch("footest", branchRPC, ""); err != nil {
		t.Error(err)
		return
	}

	if err := tree.AddMapping("/1", "footest", false); err != nil {
		t.Error(err)
		return
	}
	if err := tree.AddMapping("/1/sub1", "footest", false); err != nil {
		t.Error(err)
		return
	}
	if err := tree.AddMapping("/2/3/4", "footest", false); err != nil {
		t.Error(err)
		return
	}
	if err := tree.AddMapping("/2/3", "footest", false); err != nil {
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
	// /2/3/test1
	// /2/3/test2
	// /2/3/sub1/test3
	// /2/3/4/test1
	// /2/3/4/test2
	// /2/3/4/sub1/test3

	if res := fmt.Sprint(tree); res != `
/: 
  1/: footest(r)
    sub1/: footest(r)
  2/: 
    3/: footest(r)
      4/: footest(r)
`[1:] {
		t.Error("Unexpected result:", res)
		return
	}

	// Test tree traversal (non recursive)

	var res string

	treeVisitor := func(item *treeItem, treePath string, branchPath []string, branches []string, writable []bool) {
		// treePath is used for the result (to present to the user)
		// branchPath is send to the branch
		// branches are the branches on the current level
		res += fmt.Sprintf("(%v) /%v: %v\n", treePath, strings.Join(branchPath, "/"), branches)
	}

	res = ""
	tree.root.findPathBranches("/", createMappingPath("/"), false, treeVisitor)
	if res != `(/) /: []
` {
		t.Error("Unexpected result:", res)
		return
	}

	res = ""
	tree.root.findPathBranches("/", createMappingPath("/1"), false, treeVisitor)
	if res != `(/) /1: []
(/1) /: [footest]
` {
		t.Error("Unexpected result:", res)
		return
	}

	res = ""
	tree.root.findPathBranches("/", createMappingPath("/1/sub1"), false, treeVisitor)
	if res != `(/) /1/sub1: []
(/1) /sub1: [footest]
(/1/sub1) /: [footest]
` {
		t.Error("Unexpected result:", res)
		return
	}

	res = ""
	tree.root.findPathBranches("/", createMappingPath("/2/"), false, treeVisitor)
	if res != `(/) /2: []
(/2) /: []
` {
		t.Error("Unexpected result:", res)
		return
	}

	res = ""
	tree.root.findPathBranches("/", createMappingPath("/2/3/4/5/6"), false, treeVisitor)
	if res != `(/) /2/3/4/5/6: []
(/2) /3/4/5/6: []
(/2/3) /4/5/6: [footest]
(/2/3/4) /5/6: [footest]
` {
		t.Error("Unexpected result:", res)
		return
	}

	// Test tree traversal (recursive)

	res = ""
	tree.root.findPathBranches("/", createMappingPath("/1"), true, treeVisitor)
	if res != `(/) /1: []
(/1) /: [footest]
(/1/sub1) /: [footest]
` {
		t.Error("Unexpected result:", res)
		return
	}
}

func TestMain(m *testing.M) {
	flag.Parse()

	unitTestModes = true // Unit tests will have standard file modes
	defer func() {
		unitTestModes = false
	}()

	// Create a ssl certificate directory

	if res, _ := fileutil.PathExists(certdir); res {
		os.RemoveAll(certdir)
	}

	err := os.Mkdir(certdir, 0770)
	if err != nil {
		fmt.Print("Could not create test directory:", err.Error())
		os.Exit(1)
	}

	// Create client certificate

	certFile := fmt.Sprintf("cert-client.pem")
	keyFile := fmt.Sprintf("key-client.pem")
	host := "localhost"

	err = cryptutil.GenCert(certdir, certFile, keyFile, host, "", 365*24*time.Hour, true, 2048, "")
	if err != nil {
		panic(err)
	}

	cert, err := tls.LoadX509KeyPair(path.Join(certdir, certFile), path.Join(certdir, keyFile))
	if err != nil {
		panic(err)
	}

	clientCert = &cert

	// Ensure logging is discarded

	log.SetOutput(ioutil.Discard)

	// Set up test branches

	b1, err := createBranch("footest", "foo", false)
	errorutil.AssertOk(err)

	b2, err := createBranch("bartest", "bar", false)
	errorutil.AssertOk(err)

	footest = b1
	bartest = b2

	// Create some test files

	ioutil.WriteFile("foo/test1", []byte("Test1 file"), 0770)
	ioutil.WriteFile("foo/test2", []byte("Test2 file"), 0770)

	os.Mkdir("foo/sub1", 0770)
	ioutil.WriteFile("foo/sub1/test3", []byte("Sub dir test file"), 0770)

	ioutil.WriteFile("bar/test1", []byte("Test3 file"), 0770)

	// Run the tests

	res := m.Run()

	// Shutdown the branches

	errorutil.AssertOk(b1.Shutdown())
	errorutil.AssertOk(b2.Shutdown())

	// Remove all directories again

	if err = os.RemoveAll(certdir); err != nil {
		fmt.Print("Could not remove test directory:", err.Error())
	}
	if err = os.RemoveAll("foo"); err != nil {
		fmt.Print("Could not remove test directory:", err.Error())
	}
	if err = os.RemoveAll("bar"); err != nil {
		fmt.Print("Could not remove test directory:", err.Error())
	}

	os.Exit(res)
}

/*
createBranch creates a new branch.
*/
func createBranch(name, dir string, readionly bool) (*Branch, error) {

	// Create the path directory

	if res, _ := fileutil.PathExists(dir); res {
		os.RemoveAll(dir)
	}

	err := os.Mkdir(dir, 0770)
	if err != nil {
		fmt.Print("Could not create test directory:", err.Error())
		os.Exit(1)
	}

	// Create the certificate

	portCount++
	host := fmt.Sprintf("localhost:%v", 9020+portCount)

	// Generate a certificate and private key

	certFile := fmt.Sprintf("cert-%v.pem", portCount)
	keyFile := fmt.Sprintf("key-%v.pem", portCount)

	err = cryptutil.GenCert(certdir, certFile, keyFile, host, "", 365*24*time.Hour, true, 2048, "")
	if err != nil {
		panic(err)
	}

	cert, err := tls.LoadX509KeyPair(filepath.Join(certdir, certFile), filepath.Join(certdir, keyFile))
	if err != nil {
		panic(err)
	}

	// Create the Branch

	config := map[string]interface{}{
		config.BranchName:     name,
		config.BranchSecret:   "123",
		config.EnableReadOnly: readionly,
		config.RPCHost:        "localhost",
		config.RPCPort:        fmt.Sprint(9020 + portCount),
		config.LocalFolder:    dir,
	}

	branchConfigs[name] = config

	return NewBranch(config, &cert)
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
		} else {
			buf.WriteString(fmt.Sprintf("(%v)", fi.Size()))
		}
		buf.WriteString(fmt.Sprintln())
	}

	return buf.String()
}
