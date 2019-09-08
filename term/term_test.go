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
	"crypto/tls"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"devt.de/krotik/common/cryptutil"
	"devt.de/krotik/common/errorutil"
	"devt.de/krotik/common/fileutil"
	"devt.de/krotik/rufs"
	"devt.de/krotik/rufs/config"
)

func TestStatusUpdate(t *testing.T) {
	var buf bytes.Buffer

	// Build up a tree from one branch

	cfg := map[string]interface{}{
		config.TreeSecret: "123",
	}

	tree, _ := rufs.NewTree(cfg, clientCert)

	term := NewTreeTerm(tree, &buf)

	term.WriteStatus("Test")
	term.WriteStatus("foo")
	term.ClearStatus()

	// Check that we only clear extra characters when overwriting the status

	if buf.String() != "\rTest\rfoo \r   \r" {
		t.Errorf("Unexpected result: %#v", buf.String())
		return
	}
}

func TestSimpleTreeExploring(t *testing.T) {

	// Build up a tree from one branch

	cfg := map[string]interface{}{
		config.TreeSecret: "123",
	}

	tree, _ := rufs.NewTree(cfg, clientCert)

	fooRPC := fmt.Sprintf("%v:%v", branchConfigs["footest"][config.RPCHost], branchConfigs["footest"][config.RPCPort])
	fooFP := footest.SSLFingerprint()

	term := NewTreeTerm(tree, nil)

	term.AddCmd("unittest", "unittest [bla]", "Unit test command", func(*TreeTerm, ...string) (string, error) {
		return "123", nil
	})

	if res, err := term.Run("?"); err != nil || res != `
Available commands:
----
branch [branch name] [rpc] [fingerprint] : List all known branches or add a new branch to the tree
cat <file>                               : Read and print the contents of a file
cd [path]                                : Show or change the current directory
checksum [path] [glob]                   : Show a directory listing and file checksums
cp <src file/dir> <dst dir>              : Copy a file or directory
dir [path] [glob]                        : Show a directory listing
get <src file> [dst local file]          : Retrieve a file and store it locally (in the current directory)
help [cmd]                               : Show general or command specific help
mkdir <dir>                              : Create a new directory
mount [path] [branch name] [ro]          : List all mount points or add a new mount point to the tree
ping <branch name> [rpc]                 : Ping a remote branch
put [src local file] [dst file]          : Read a local file and store it
refresh                                  : Refreshes all known branches and reconnect if possible
ren <file> <newfile>                     : Rename a file or directory
reset [mounts|brances]                   : Remove all mounts or all mounts and all branches
rm <file>                                : Delete a file or directory (* all files; ** all files/recursive)
sync <src dir> <dst dir>                 : Make sure dst has the same files and directories as src
tree [path] [glob]                       : Show the listing of a directory and its subdirectories
unittest [bla]                           : Unit test command
`[1:] {
		t.Error("Unexpected result: ", res, err)
		return
	}

	if res := term.Cmds(); fmt.Sprint(res) != "[? branch cat cd checksum cp dir "+
		"get help ll mkdir mount ping put refresh ren reset rm sync tree unittest]" {
		t.Error("Unexpected result:", res)
		return
	}

	if res, err := term.Run("ll"); err != nil || res != `
/
`[1:] {
		t.Error("Unexpected result: ", res, err)
		return
	}

	if res, err := term.Run("mount"); err != nil || res != `
/: 
`[1:] {
		t.Error("Unexpected result: ", res, err)
		return
	}

	if res, err := term.Run("branch"); err != nil || res != `
`[1:] {
		t.Error("Unexpected result: ", res, err)
		return
	}

	if res, err := term.Run("refresh"); err != nil || res != `
Done`[1:] {
		t.Error("Unexpected result: ", res, err)
		return
	}

	if _, err := term.Run(fmt.Sprintf("branch myfoo %v %v", fooRPC, fooFP)); err == nil ||
		err.Error() != "RufsError: Remote error (Unknown target node)" {
		t.Error("Unexpected result: ", err)
		return
	}

	if res, err := term.Run(fmt.Sprintf("ping footest %v %v", fooRPC, "")); err != nil || res != `
Response ok - fingerprint: `[1:]+fooFP+`
` {
		t.Error("Unexpected result: ", res, err)
		return
	}

	if res, err := term.Run(fmt.Sprintf("branch footest %v %v", fooRPC, "")); err != nil || res != `
footest [`[1:]+fooFP+`]
` {
		t.Error("Unexpected result: ", res, err)
		return
	}

	if res, err := term.Run("branch"); err != nil || res != `
footest [`[1:]+fooFP+`]
` {
		t.Error("Unexpected result: ", res, err)
		return
	}

	if _, err := term.Run("mount / myfoo"); err == nil ||
		err.Error() != "Unknown target node" {
		t.Error("Unexpected result: ", err)
		return
	}

	// Mount the first directory

	if _, err := term.Run("mount / footest"); err != nil {
		t.Error("Unexpected result: ", err)
		return
	}

	// The directory listing should now return something

	if res, err := term.Run("ll"); err != nil || (res != `
/
drwxrwx--- 4.0 KiB sub1
-rwxrwx---  10 B   test1
-rwxrwx---  10 B   test2
`[1:] && res != `
/
drwxr-x--- 4.0 KiB sub1
-rwxr-x---  10 B   test1
-rwxr-x---  10 B   test2
`[1:] && res != `
/
drwxrwxrwx  0 B   sub1
-rw-rw-rw- 10 B   test1
-rw-rw-rw- 10 B   test2
`[1:]) {
		t.Error("Unexpected result: ", res, err)
		return
	}

	if res, err := term.Run("dir sub1"); err != nil || (res != `
/sub1
-rwxrwx--- 17 B   test3
`[1:] && res != `
/sub1
-rwxr-x--- 17 B   test3
`[1:] && res != `
/sub1
-rw-rw-rw- 17 B   test3
`[1:]) {
		t.Error("Unexpected result: ", res, err)
		return
	}

	if res, err := term.Run("dir . {"); err == nil || err.Error() != "Unclosed group at 1 of {" {
		t.Error("Unexpected result: ", res, err)
		return
	}

	if res, err := term.Run("checksum"); err != nil || (res != `
/
drwxrwx--- 4.0 KiB sub1
-rwxrwx---  10 B   test1 [73b8af47]
-rwxrwx---  10 B   test2 [b0c1fadd]
`[1:] && res != `
/
drwxr-x--- 4.0 KiB sub1
-rwxr-x---  10 B   test1 [73b8af47]
-rwxr-x---  10 B   test2 [b0c1fadd]
`[1:] && res != `
/
drwxrwxrwx  0 B   sub1
-rw-rw-rw- 10 B   test1 [73b8af47]
-rw-rw-rw- 10 B   test2 [b0c1fadd]
`[1:]) {
		t.Error("Unexpected result: ", res, err)
		return
	}

	if res, err := term.Run("tree"); err != nil || (res != `
/
drwxrwx--- 4.0 KiB sub1
-rwxrwx---  10 B   test1
-rwxrwx---  10 B   test2

/sub1
-rwxrwx--- 17 B   test3
`[1:] && res != `
/
drwxr-x--- 4.0 KiB sub1
-rwxr-x---  10 B   test1
-rwxr-x---  10 B   test2

/sub1
-rwxr-x--- 17 B   test3
`[1:] && res != `
/
drwxrwxrwx  0 B   sub1
-rw-rw-rw- 10 B   test1
-rw-rw-rw- 10 B   test2

/sub1
-rw-rw-rw- 17 B   test3
`[1:]) {
		t.Error("Unexpected result: ", res, err)
		return
	}

	if res := term.CurrentDir(); res != "/" {
		t.Error("Unexpected result: ", res)
		return
	}

	if res, err := term.Run("cd sub1"); err != nil || res != `
/sub1
`[1:] {
		t.Error("Unexpected result: ", res, err)
		return
	}

	if res := term.CurrentDir(); res != "/sub1" {
		t.Error("Unexpected result: ", res)
		return
	}

	if res, err := term.Run("checksum"); err != nil || (res != `
/sub1
-rwxrwx--- 17 B   test3 [f89782b1]
`[1:] && res != `
/sub1
-rwxr-x--- 17 B   test3 [f89782b1]
`[1:] && res != `
/sub1
-rw-rw-rw- 17 B   test3 [f89782b1]
`[1:]) {
		t.Error("Unexpected result: ", res, err)
		return
	}

	if _, err := term.Run("reset"); err == nil ||
		err.Error() != "Can either reset all [mounts] or all [branches] which includes all mount points" {
		t.Error("Unexpected result: ", err)
		return
	}

	if res, err := term.Run("reset mounts"); err != nil || res != `
Resetting all mounts
`[1:] {
		t.Error("Unexpected result: ", res, err)
		return
	}

	if res, err := term.Run("mount"); err != nil || res != `
/: 
`[1:] {
		t.Error("Unexpected result: ", res, err)
		return
	}

	if res, err := term.Run("branch"); err != nil || res != `
footest [`[1:]+fooFP+`]
` {
		t.Error("Unexpected result: ", res, err)
		return
	}

	if res, err := term.Run("reset branches"); err != nil || res != `
Resetting all branches and mounts
`[1:] {
		t.Error("Unexpected result: ", res, err)
		return
	}

	if res, err := term.Run("branch"); err != nil || res != `
`[1:] {
		t.Error("Unexpected result: ", res, err)
		return
	}

	// Error returns

	if _, err := term.Run("mount x"); err == nil || err.Error() != "mount requires either 2 or no parameters" {
		t.Error("Unexpected result: ", err)
		return
	}

	if _, err := term.Run("branch x"); err == nil || err.Error() != "branch requires either no or at least 2 parameters" {
		t.Error("Unexpected result: ", err)
		return
	}

	if _, err := term.Run("bla"); err == nil || err.Error() != "Unknown command: bla" {
		t.Error("Unexpected result: ", err)
		return
	}
}

const certdir = "certs" // Directory for certificates
var portCount = 0       // Port assignment counter for Branch ports

var footest, bartest, tmptest *rufs.Branch              // Branches
var branchConfigs = map[string]map[string]interface{}{} // All branch configs
var clientCert *tls.Certificate

func TestMain(m *testing.M) {
	flag.Parse()

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

	b1, err := createBranch("footest", "foo")
	errorutil.AssertOk(err)

	b2, err := createBranch("bartest", "bar")
	errorutil.AssertOk(err)

	b3, err := createBranch("tmptest", "tmp")
	errorutil.AssertOk(err)

	footest = b1
	bartest = b2
	tmptest = b3

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
	errorutil.AssertOk(b3.Shutdown())

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
	if err = os.RemoveAll("tmp"); err != nil {
		fmt.Print("Could not remove test directory:", err.Error())
	}

	os.Exit(res)
}

/*
createBranch creates a new branch.
*/
func createBranch(name, dir string) (*rufs.Branch, error) {

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
		config.EnableReadOnly: false,
		config.RPCHost:        "localhost",
		config.RPCPort:        fmt.Sprint(9020 + portCount),
		config.LocalFolder:    dir,
	}

	branchConfigs[name] = config

	return rufs.NewBranch(config, &cert)
}
