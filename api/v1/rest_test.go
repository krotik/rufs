/*
 * Rufs - Remote Union File System
 *
 * Copyright 2017 Matthias Ladkau. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the MIT
 * License, If a copy of the MIT License was not distributed with this
 * file, You can obtain one at https://opensource.org/licenses/MIT.
 */

package v1

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"devt.de/krotik/common/cryptutil"
	"devt.de/krotik/common/errorutil"
	"devt.de/krotik/common/fileutil"
	"devt.de/krotik/common/httputil"
	"devt.de/krotik/rufs"
	"devt.de/krotik/rufs/api"
	"devt.de/krotik/rufs/config"
)

const TESTPORT = ":9040"

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

	// Start the server

	hs, wg := startServer()
	if hs == nil {
		return
	}

	// Register endpoints for version 1
	api.RegisterRestEndpoints(api.GeneralEndpointMap)
	api.RegisterRestEndpoints(V1EndpointMap)

	// Run the tests

	res := m.Run()

	// Teardown

	stopServer(hs, wg)

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

func TestSwaggerDefs(t *testing.T) {

	// Test we can build swagger defs from the endpoint

	data := map[string]interface{}{
		"paths":       map[string]interface{}{},
		"definitions": map[string]interface{}{},
	}

	for _, inst := range V1EndpointMap {
		inst().SwaggerDefs(data)
	}

	// Show swagger output

	/*
		queryURL := "http://localhost" + TESTPORT + api.EndpointSwagger
		_, _, res := sendTestRequest(queryURL, "GET", nil)
		fmt.Println(res)
	*/
}

/*
Send a request to a HTTP test server
*/
func sendTestRequest(url string, method string, content []byte) (string, http.Header, string) {
	var req *http.Request
	var err error

	ct := "application/json"

	if method == "FORMPOST" {
		method = "POST"
		ct = "application/x-www-form-urlencoded"
	}

	if content != nil {
		req, err = http.NewRequest(method, url, bytes.NewBuffer(content))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		panic(err)
	}

	req.Header.Set("Content-Type", ct)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	bodyStr := strings.Trim(string(body), " \n")

	// Try json decoding first

	out := bytes.Buffer{}
	err = json.Indent(&out, []byte(bodyStr), "", "  ")
	if err == nil {
		return resp.Status, resp.Header, out.String()
	}

	// Just return the body

	return resp.Status, resp.Header, bodyStr
}

/*
Start a HTTP test server.
*/
func startServer() (*httputil.HTTPServer, *sync.WaitGroup) {
	hs := &httputil.HTTPServer{}

	var wg sync.WaitGroup
	wg.Add(1)

	go hs.RunHTTPServer(TESTPORT, &wg)

	wg.Wait()

	// Server is started

	if hs.LastError != nil {
		panic(hs.LastError)
	}

	return hs, &wg
}

/*
Stop a started HTTP test server.
*/
func stopServer(hs *httputil.HTTPServer, wg *sync.WaitGroup) {

	if hs.Running == true {

		wg.Add(1)

		// Server is shut down

		hs.Shutdown()

		wg.Wait()

	} else {

		panic("Server was not running as expected")
	}
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

	cfg := map[string]interface{}{
		config.BranchName:     name,
		config.BranchSecret:   "123",
		config.EnableReadOnly: false,
		config.RPCHost:        "localhost",
		config.RPCPort:        fmt.Sprint(9020 + portCount),
		config.LocalFolder:    dir,
	}

	branchConfigs[name] = cfg

	return rufs.NewBranch(cfg, &cert)
}
