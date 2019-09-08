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
	"crypto/sha512"
	"crypto/tls"
	"fmt"
	"net"
	"net/rpc"
	"sync"
)

/*
RequestHandler is a function to handle incoming requests. A request has a
control object which contains information on what the data is and how it
should be used and the data itself. The request handler should return
the result or an error.
*/
type RequestHandler func(ctrl map[string]string, data []byte) ([]byte, error)

/*
RufsNode is the management object for a node in the Rufs network.

A RufsNode registers itself to the rpc server which is the global
server object. Each node needs to have a unique name. Communication between nodes
is secured by using a secret string which is never exchanged over the network
and a hash generated token which identifies a member.

Each RufsNode object contains a Client object which can be used to communicate
with other nodes. This object should be used by pure clients - code which should
communicate with the cluster without running an actual member.
*/
type RufsNode struct {
	name        string           // Name of the node
	secret      string           // Network wide secret
	Client      *Client          // RPC client object
	listener    net.Listener     // RPC server listener
	wg          sync.WaitGroup   // RPC server Waitgroup for listener shutdown
	DataHandler RequestHandler   // Handler function for data requests
	cert        *tls.Certificate // Node certificate
}

/*
NewNode create a new RufsNode object.
*/
func NewNode(rpcInterface string, name string, secret string, clientCert *tls.Certificate,
	dataHandler RequestHandler) *RufsNode {

	// Generate node token

	token := &RufsNodeToken{name, fmt.Sprintf("%X", sha512.Sum512_224([]byte(name+secret)))}

	rn := &RufsNode{name, secret, &Client{token, rpcInterface, make(map[string]string),
		make(map[string]*rpc.Client), make(map[string]string), clientCert, &sync.RWMutex{}, false},
		nil, sync.WaitGroup{}, dataHandler, clientCert}

	return rn
}

/*
NewClient create a new Client object.
*/
func NewClient(secret string, clientCert *tls.Certificate) *Client {
	return NewNode("", "", secret, clientCert, nil).Client
}

// General node API
// ================

/*
Name returns the name of the node.
*/
func (rn *RufsNode) Name() string {
	return rn.name
}

/*
SSLFingerprint returns the SSL fingerprint of the node.
*/
func (rn *RufsNode) SSLFingerprint() string {
	var ret string
	if rn.cert != nil && rn.cert.Certificate[0] != nil {
		ret = fingerprint(rn.cert.Certificate[0])
	}
	return ret
}

/*
LogInfo logs a node related message at info level.
*/
func (rn *RufsNode) LogInfo(v ...interface{}) {
	LogInfo(rn.name, ": ", fmt.Sprint(v...))
}

/*
Start starts process for this node.
*/
func (rn *RufsNode) Start(serverCert *tls.Certificate) error {

	if _, ok := rufsServer.nodes[rn.name]; ok {
		return fmt.Errorf("Cannot start node %s twice", rn.name)
	}

	rn.LogInfo("Starting node ", rn.name, " rpc server on: ", rn.Client.rpc)

	l, err := net.Listen("tcp", rn.Client.rpc)
	if err != nil {
		return err
	}

	if serverCert != nil && serverCert.Certificate[0] != nil {
		rn.cert = serverCert

		rn.LogInfo("SSL fingerprint: ", rn.SSLFingerprint())

		// Wrap the listener in a TLS listener

		config := tls.Config{Certificates: []tls.Certificate{*serverCert}}

		l = tls.NewListener(l, &config)
	}

	// Kick of the rpc listener

	go func() {
		rpc.Accept(l)
		rn.wg.Done()
		rn.LogInfo("Connection closed: ", rn.Client.rpc)
	}()

	rn.listener = l

	// Register this node in the global server map

	rufsServer.nodes[rn.name] = rn

	return nil
}

/*
Shutdown shuts the member manager rpc server for this cluster member down.
*/
func (rn *RufsNode) Shutdown() error {
	var err error

	// Close socket

	if rn.listener != nil {
		rn.LogInfo("Shutdown rpc server on: ", rn.Client.rpc)
		rn.wg.Add(1)
		err = rn.listener.Close()
		rn.Client.Shutdown()
		rn.listener = nil
		rn.wg.Wait()
		delete(rufsServer.nodes, rn.name)
	} else {
		LogDebug("Node ", rn.name, " already shut down")
	}

	return err
}
