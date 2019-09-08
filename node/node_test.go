/*
 * Rufs - Remote Union File System
 *
 * Copyright 2017 Matthias Ladkau. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the MIT
 * License, If a copy of the MIT License was not distributed with this
 * file, You can obtain one at https://opensource.org/licenses/MIT.
 */

package node

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"testing"
	"time"

	"devt.de/krotik/common/cryptutil"
	"devt.de/krotik/common/fileutil"
)

var consoleOutput = false
var liveOutput = false

type LogWriter struct {
	w io.Writer
}

func (l LogWriter) Write(p []byte) (n int, err error) {
	if liveOutput {
		fmt.Print(string(p))
	}
	return l.w.Write(p)
}

const certdir = "certs"

func TestMain(m *testing.M) {
	flag.Parse()

	// Create output capture file

	outFile, err := os.Create("out.txt")
	if err != nil {
		panic(err)
	}

	if res, _ := fileutil.PathExists(certdir); res {
		os.RemoveAll(certdir)
	}

	err = os.Mkdir(certdir, 0770)
	if err != nil {
		fmt.Print("Could not create test directory:", err.Error())
		os.Exit(1)
	}

	// Ensure logging is directed to the file

	log.SetOutput(LogWriter{outFile})

	// Run the tests

	res := m.Run()

	log.SetOutput(os.Stderr)

	// Collected output

	outFile.Sync()
	outFile.Close()

	stdout, err := ioutil.ReadFile("out.txt")
	if err != nil {
		panic(err)
	}

	// Handle collected output

	if consoleOutput {
		fmt.Println(string(stdout))
	}

	os.RemoveAll("out.txt")

	err = os.RemoveAll(certdir)
	if err != nil {
		fmt.Print("Could not remove test directory:", err.Error())
	}

	os.Exit(res)
}

/*
createNodeNetwork creates a network of n nodes.
*/
func createNodeNetwork(n int) []*RufsNode {

	var mms []*RufsNode

	for i := 0; i < n; i++ {
		host := fmt.Sprintf("localhost:%v", 9020+i)

		// Generate a certificate and private key

		certFile := fmt.Sprintf("cert-%v.pem", i)
		keyFile := fmt.Sprintf("key-%v.pem", i)

		err := cryptutil.GenCert(certdir, certFile, keyFile, host, "", 365*24*time.Hour, true, 2048, "")
		if err != nil {
			panic(err)
		}

		cert, err := tls.LoadX509KeyPair(path.Join(certdir, certFile), path.Join(certdir, keyFile))
		if err != nil {
			panic(err)
		}

		mms = append(mms, NewNode(host,
			fmt.Sprintf("TestNode-%v", i), "test123", &cert, nil))
	}

	return mms
}

func TestReconnect(t *testing.T) {

	// Debug logging

	// liveOutput = true
	// LogDebug = LogInfo
	// defer func() { liveOutput = false }()

	// Send a simple ping

	nnet2 := createNodeNetwork(2)

	nnet2[0].Start(nnet2[0].Client.cert) // Start the server with the client certificate
	nnet2[1].Start(nnet2[1].Client.cert) // Start the server with the client certificate
	defer nnet2[0].Shutdown()
	defer nnet2[1].Shutdown()

	var ctrlReceived map[string]string
	var dataReceived string

	nnet2[1].DataHandler = func(ctrl map[string]string, data []byte) ([]byte, error) {
		dataReceived = string(data)
		ctrlReceived = ctrl
		return []byte("testack"), nil
	}

	// Register peer 1 on peer 0

	nnet2[0].Client.RegisterPeer(nnet2[1].name, nnet2[1].Client.rpc, nnet2[1].SSLFingerprint())

	// Send data successful

	datares, err := nnet2[0].Client.SendData(nnet2[1].name, map[string]string{
		"foo": "bar",
	}, []byte("testmsg"))

	if dataReceived != "testmsg" || fmt.Sprint(ctrlReceived) != "map[foo:bar]" ||
		string(datares) != "testack" || err != nil {
		t.Error("Unexpected result: ", ctrlReceived, dataReceived, datares, err)
		return
	}

	ctrlReceived = nil
	dataReceived = ""

	// Shutdown peer 1

	nnet2[1].Shutdown()

	datares, err = nnet2[0].Client.SendData(nnet2[1].name, map[string]string{
		"foo": "bar",
	}, []byte("testmsg"))

	if err == nil || err.Error() != "RufsError: Remote error (Unknown target node)" {
		t.Error("Unexpected result: ", ctrlReceived, dataReceived, datares, err)
		return
	}

	ctrlReceived = nil
	dataReceived = ""

	// Start again

	nnet2[1].Start(nnet2[1].Client.cert)

	datares, err = nnet2[0].Client.SendData(nnet2[1].name, map[string]string{
		"foo": "bar",
	}, []byte("testmsg"))

	if dataReceived != "testmsg" || fmt.Sprint(ctrlReceived) != "map[foo:bar]" ||
		string(datares) != "testack" || err != nil {
		t.Error("Unexpected result: ", ctrlReceived, dataReceived, datares, err)
		return
	}
}

func Test2NodeNetwork(t *testing.T) {

	// Debug logging

	// liveOutput = true
	// LogDebug = LogInfo
	// defer func() { liveOutput = false }()

	// Send a simple ping

	nnet2 := createNodeNetwork(2)

	nnet2[0].Start(nnet2[0].Client.cert) // Start the server with the client certificate
	nnet2[1].Start(nnet2[1].Client.cert) // Start the server with the client certificate
	defer nnet2[0].Shutdown()
	defer nnet2[1].Shutdown()

	// Check we are using the same certificates

	if nnet2[0].Client.SSLFingerprint() != nnet2[0].SSLFingerprint() {
		t.Errorf("Unexpected result:\n#%v\n#%v", nnet2[0].Client.SSLFingerprint(), nnet2[0].SSLFingerprint())
		return
	}

	res, rfp, err := nnet2[0].Client.SendPing(nnet2[1].name, nnet2[1].Client.rpc)

	if fmt.Sprint(res) != "[Pong]" || rfp != nnet2[1].SSLFingerprint() || err != nil {
		t.Error("Unexpected result:", res, err)
		return
	}

	if res := nnet2[1].Name(); res != "TestNode-1" {
		t.Error("Unexpected result:", res)
		return
	}

	// Send a data request

	var ctrlReceived map[string]string
	var dataReceived string
	var datares []byte

	nnet2[1].DataHandler = func(ctrl map[string]string, data []byte) ([]byte, error) {
		dataReceived = string(data)
		ctrlReceived = ctrl
		return []byte("testack"), nil
	}

	_, err = nnet2[0].Client.SendData(nnet2[1].name, map[string]string{
		"foo": "bar",
	}, []byte("testmsg"))

	if err == nil || err.Error() != "Unknown peer: TestNode-1" {
		t.Error("Unexpected result: ", err)
		return
	}

	// Register the peer

	if err := nnet2[0].Client.RegisterPeer(nnet2[1].name, "", ""); err.Error() != "RPC interface must not be empty" {
		t.Error("Unexpected result: ", err)
		return
	}

	nnet2[0].Client.RegisterPeer(nnet2[1].name, nnet2[1].Client.rpc, nnet2[1].SSLFingerprint())

	if err := nnet2[0].Client.RegisterPeer(nnet2[1].name, nnet2[1].Client.rpc, ""); err.Error() != "Peer already registered: TestNode-1" {
		t.Error("Unexpected result: ", err)
		return
	}

	peers, peerFps := nnet2[0].Client.Peers()
	if res := fmt.Sprint(peers); res != "[TestNode-1]" && peerFps[0] == nnet2[1].SSLFingerprint() {
		t.Error("Unexpected result: ", res)
		return
	}

	// Send the data request

	datares, err = nnet2[0].Client.SendData(nnet2[1].name, map[string]string{
		"foo": "bar",
	}, []byte("testmsg"))

	if dataReceived != "testmsg" || fmt.Sprint(ctrlReceived) != "map[foo:bar]" ||
		string(datares) != "testack" || err != nil {
		t.Error("Unexpected result: ", ctrlReceived, dataReceived, datares, err)
		return
	}

	// Close connection and send data again (connection should be automatically reconnected)

	nnet2[0].Client.conns[nnet2[1].name].Close()

	datares, err = nnet2[0].Client.SendData(nnet2[1].name, map[string]string{
		"foo": "bar",
	}, []byte("testmsg"))

	if dataReceived != "testmsg" || fmt.Sprint(ctrlReceived) != "map[foo:bar]" ||
		string(datares) != "testack" || err != nil {
		t.Error("Unexpected result: ", ctrlReceived, dataReceived, datares, err)
		return
	}

	nnet2[0].Client.RemovePeer(nnet2[1].name)
	nnet2[0].Client.RegisterPeer(nnet2[1].name, nnet2[1].Client.rpc, "123")

	res, rfp, err = nnet2[0].Client.SendPing(nnet2[1].name, nnet2[1].Client.rpc)

	if fmt.Sprint(res) != "[]" || rfp != "" || err == nil ||
		err.Error() != "RufsError: Unexpected SSL certificate from target node (TestNode-1)" {
		t.Error("Unexpected result:", res, rfp, err)
		return
	}

	// Test error response

	nnet2[0].Client.RemovePeer(nnet2[1].name)

	_, err = nnet2[0].Client.SendData(nnet2[1].name, map[string]string{
		"foo": "bar",
	}, []byte("testmsg"))

	if err == nil || err.Error() != "Unknown peer: TestNode-1" {
		t.Error("Unexpected result: ", err)
		return
	}

	nnet2[0].Client.RegisterPeer(nnet2[1].name, nnet2[1].Client.rpc, "")

	res, rfp, err = nnet2[0].Client.SendPing("", "localhost")

	if err == nil || rfp != "" {
		t.Error("Unexpected result:", res, rfp, err)
		return
	}

	rres, err := nnet2[0].Client.SendRequest("", RPCPing, nil)

	if err == nil || err.Error() != "RufsError: Unknown target node" {
		t.Error("Unexpected result:", rres, err)
		return
	}

	rres, err = nnet2[0].Client.SendRequest(nnet2[1].name, "foo", nil)

	if err == nil || err.Error() != "RufsError: Remote error (rpc: can't find method RufsServer.foo)" {
		t.Error("Unexpected result:", rres, err)
		return
	}

	nnet2[1].DataHandler = func(ctrl map[string]string, data []byte) ([]byte, error) {
		return nil, fmt.Errorf("Testerror")
	}

	datares, err = nnet2[0].Client.SendData(nnet2[1].name, nil, []byte("testmsg"))

	if err == nil || err.Error() != "RufsError: Remote error (Testerror)" {
		t.Error("Unexpected result: ", dataReceived, datares, err)
		return
	}

	nnet2[1].DataHandler = func(ctrl map[string]string, data []byte) ([]byte, error) {
		return nil, &Error{ErrNodeComm, "Testerror2", false}
	}

	datares, err = nnet2[0].Client.SendData(nnet2[1].name, nil, []byte("testmsg"))

	if err == nil || err.Error() != "RufsError: Network error (Testerror2)" {
		t.Error("Unexpected result: ", dataReceived, datares, err)
		return
	}
}

func TestNodeErrors(t *testing.T) {
	// Debug logging

	//liveOutput = true
	//LogDebug = LogInfo
	// defer func() { liveOutput = false }()

	n := NewNode(fmt.Sprintf("localhost:%v", 9019), "TestNode", "test123", nil, nil)

	n.Start(nil)

	if err := n.Start(nil); err.Error() != "Cannot start node TestNode twice" {
		t.Error("Unexpected result: ", err)
		return
	}

	cl := NewClient("test123", nil)
	res, rfp, err := cl.SendPing("TestNode", fmt.Sprintf("localhost:%v", 9019))

	if fmt.Sprint(res) != "[Pong]" || err != nil || rfp != "" {
		t.Error("Unexpected result:", res, rfp, err)
		return
	}

	// Now corrupt the token of the Client

	cl.token.NodeAuth = "123"

	_, _, err = cl.SendPing("TestNode", fmt.Sprintf("localhost:%v", 9019))

	if err == nil || err.Error() != "RufsError: Remote error (Invalid node token)" {
		t.Error("Unexpected result: ", err)
		return
	}

	cl.RegisterPeer("TestNode", fmt.Sprintf("localhost:%v", 9019), "")

	_, err = cl.SendData("TestNode", nil, nil)

	if err == nil || err.Error() != "RufsError: Remote error (Invalid node token)" {
		t.Error("Unexpected result: ", err)
		return
	}

	cl.Shutdown()

	n.Shutdown()

	if err := n.Shutdown(); err != nil {
		t.Error("Unexpected result: ", err)
		return
	}

	n = NewNode(fmt.Sprintf("fff"), "TestNode", "test123", nil, nil)

	if err := n.Start(nil); err == nil {
		t.Error("Unexpected result: ", err)
		return
	}
}
