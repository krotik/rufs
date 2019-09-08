package rumble

import (
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
	"devt.de/krotik/common/defs/rumble"
	"devt.de/krotik/common/errorutil"
	"devt.de/krotik/common/fileutil"
	"devt.de/krotik/rufs"
	"devt.de/krotik/rufs/api"
	"devt.de/krotik/rufs/config"
)

type mockRuntime struct {
}

func (mr *mockRuntime) NewRuntimeError(t error, d string) rumble.RuntimeError {
	return fmt.Errorf("%v %v", t, d)
}

func TestDir(t *testing.T) {

	// Setup a tree

	defer func() {

		// Make sure all trees are removed

		api.ResetTrees()
	}()

	tree, err := rufs.NewTree(api.TreeConfigTemplate, api.TreeCertTemplate)
	errorutil.AssertOk(err)

	api.AddTree("Hans1", tree)

	fooRPC := fmt.Sprintf("%v:%v", branchConfigs["footest"][config.RPCHost], branchConfigs["footest"][config.RPCPort])
	fooFP := footest.SSLFingerprint()

	err = tree.AddBranch("footest", fooRPC, fooFP)
	errorutil.AssertOk(err)

	err = tree.AddMapping("/", "footest", false)
	errorutil.AssertOk(err)

	paths, infos, err := tree.Dir("/", "", true, true)
	if res := rufs.DirResultToString(paths, infos); err != nil || res != `
/
drwxrwx--- 4.0 KiB sub1
-rwxrwx---  10 B   test1 [73b8af47]
-rwxrwx---  10 B   test2 [b0c1fadd]

/sub1
-rwxrwx--- 17 B   test3 [f89782b1]
`[1:] && res != `
/
drwxr-x--- 4.0 KiB sub1
-rwxr-x---  10 B   test1 [73b8af47]
-rwxr-x---  10 B   test2 [b0c1fadd]

/sub1
-rwxr-x--- 17 B   test3 [f89782b1]
`[1:] {
		t.Error("Unexpected result:", res, err)
		return
	}

	df := &DirFunc{}
	mr := &mockRuntime{}

	if df.Name() != "fs.dir" {
		t.Error("Unexpected result:", df.Name())
		return
	}

	if err := df.Validate(1, mr); err == nil || err.Error() !=
		"Invalid construct Function dir requires 3 or 4 parameters: tree, a path, a glob expression and optionally a recursive flag" {
		t.Error(err)
		return
	}

	if err := df.Validate(3, mr); err != nil {
		t.Error(err)
		return
	}

	if err := df.Validate(4, mr); err != nil {
		t.Error(err)
		return
	}

	_, err = df.Execute([]interface{}{"Hans1", "/", "*.mp3", true}, nil, mr)
	if err != nil {
		t.Error(err)
		return
	}

	res, err := df.Execute([]interface{}{"Hans1", "/", "", true}, nil, mr)
	if err != nil {
		t.Error(err)
		return
	}

	if fmt.Sprint(res.([]interface{})[0].([]interface{})[0]) != "/" {
		t.Error("Unexpected result:", fmt.Sprint(res.([]interface{})[0].([]interface{})[0]))
		return
	}

	if fmt.Sprint(res.([]interface{})[0].([]interface{})[1]) != "/sub1" {
		t.Error("Unexpected result:", fmt.Sprint(res.([]interface{})[0].([]interface{})[1]))
		return
	}

	// Test error messages

	_, err = df.Execute([]interface{}{"Hans2", "/", "", true}, nil, mr)
	if err == nil || err.Error() != "Invalid state Cannot list files: Unknown tree: Hans2" {
		t.Error(err)
		return
	}
}

// Main function for all tests in this package

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

	// Set the default client certificate and configuration for the REST API

	api.TreeConfigTemplate = map[string]interface{}{
		config.TreeSecret: "123",
	}

	api.TreeCertTemplate = &cert

	// Ensure logging is discarded

	log.SetOutput(ioutil.Discard)

	// Set up test branches

	b1, err := createBranch("footest", "foo")
	errorutil.AssertOk(err)

	b2, err := createBranch("bartest", "bar")
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

const certdir = "certs" // Directory for certificates
var portCount = 0       // Port assignment counter for Branch ports

var footest, bartest *rufs.Branch                       // Branches
var branchConfigs = map[string]map[string]interface{}{} // All branch configs

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
