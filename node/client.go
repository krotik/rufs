/*
 * Rufs - Remote Union File System
 *
 * Copyright 2017 Matthias Ladkau. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the MIT
 * License, If a copy of the MIT License was not distributed with this
 * file, You can obtain one at https://opensource.org/licenses/MIT.
 */

/*
Package node contains the network communication code for Rufs via RPC calls.
*/
package node

import (
	"crypto/tls"
	"encoding/gob"
	"fmt"
	"io"
	"net"
	"net/rpc"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

func init() {

	// Make sure we can use the relevant types in a gob operation

	gob.Register(&RufsNodeToken{})
	gob.Register(map[string]string{})
}

/*
DialTimeout is the dial timeout for RPC connections
*/
var DialTimeout = 10 * time.Second

/*
RufsNodeToken is used to authenticate a node in the network to other nodes
*/
type RufsNodeToken struct {
	NodeName string
	NodeAuth string
}

/*
Client is the client for the RPC API of a node.
*/
type Client struct {
	token        *RufsNodeToken         // Token to be send to other nodes for authentication
	rpc          string                 // This client's rpc network interface (may be empty in case of pure clients)
	peers        map[string]string      // Map of node names to their rpc network interface
	conns        map[string]*rpc.Client // Map of node names to network connections
	fingerprints map[string]string      // Map of expected server certificate fingerprints
	cert         *tls.Certificate       // Client certificate
	maplock      *sync.RWMutex          // Lock for maps
	redial       bool                   // Flag if this client is attempting a redial
}

/*
SSLFingerprint returns the SSL fingerprint of the client.
*/
func (c *Client) SSLFingerprint() string {
	var ret string
	if c.cert != nil && c.cert.Certificate[0] != nil {
		ret = fingerprint(c.cert.Certificate[0])
	}
	return ret
}

/*
Shutdown closes all stored connections.
*/
func (c *Client) Shutdown() {
	c.maplock.Lock()
	defer c.maplock.Unlock()

	for _, c := range c.conns {
		c.Close()
	}
	c.conns = make(map[string]*rpc.Client)
}

/*
RegisterPeer registers a new peer to communicate with. An empty fingerprint
means that the client will accept any certificate from the server.
*/
func (c *Client) RegisterPeer(node string, rpc string, fingerprint string) error {

	if _, ok := c.peers[node]; ok {
		return fmt.Errorf("Peer already registered: %v", node)
	} else if rpc == "" {
		return fmt.Errorf("RPC interface must not be empty")
	}

	c.maplock.Lock()

	c.peers[node] = rpc
	delete(c.conns, node)
	c.fingerprints[node] = fingerprint

	c.maplock.Unlock()

	return nil
}

/*
Peers returns all registered peers and their expected fingerprints.
*/
func (c *Client) Peers() ([]string, []string) {
	ret := make([]string, 0, len(c.peers))
	fps := make([]string, len(c.peers))

	c.maplock.Lock()
	defer c.maplock.Unlock()

	for k := range c.peers {
		ret = append(ret, k)
	}

	sort.Strings(ret)

	for i, node := range ret {
		fps[i] = c.fingerprints[node]
	}

	return ret, fps
}

/*
RemovePeer removes a registered peer.
*/
func (c *Client) RemovePeer(node string) {
	c.maplock.Lock()
	delete(c.peers, node)
	delete(c.conns, node)
	delete(c.fingerprints, node)
	c.maplock.Unlock()
}

/*
SendPing sends a ping to a node and returns the result. Second argument is
optional if the target member is not a known peer. Should be an empty string
in all other cases. Returns the answer, the fingerprint of the presented server
certificate and any errors.
*/
func (c *Client) SendPing(node string, rpc string) ([]string, string, error) {
	var ret []string
	var fp string

	if _, ok := c.peers[node]; !ok && rpc != "" {

		// Add member temporary if it was not registered

		c.maplock.Lock()
		c.peers[node] = rpc
		c.maplock.Unlock()

		defer func() {
			c.maplock.Lock()
			delete(c.peers, node)
			delete(c.conns, node)
			delete(c.fingerprints, node)
			c.maplock.Unlock()
		}()
	}

	res, err := c.SendRequest(node, RPCPing, nil)

	if res != nil && err == nil {
		ret = res.([]string)
		c.maplock.Lock()
		fp = c.fingerprints[node]
		c.maplock.Unlock()
	}

	return ret, fp, err
}

/*
SendData sends a portion of data and some control information to a node and
returns the result.
*/
func (c *Client) SendData(node string, ctrl map[string]string, data []byte) ([]byte, error) {

	if _, ok := c.peers[node]; !ok {
		return nil, fmt.Errorf("Unknown peer: %v", node)
	}

	res, err := c.SendRequest(node, RPCData, map[RequestArgument]interface{}{
		RequestCTRL: ctrl,
		RequestDATA: data,
	})

	if res != nil {
		return res.([]byte), err
	}

	return nil, err
}

/*
SendRequest sends a request to another node.
*/
func (c *Client) SendRequest(node string, remoteCall RPCFunction,
	args map[RequestArgument]interface{}) (interface{}, error) {

	var err error

	// Function to categorize errors

	handleError := func(err error) error {

		if _, ok := err.(net.Error); ok {
			return &Error{ErrNodeComm, err.Error(), false}
		}

		// Wrap remote errors in a proper error object

		if err != nil && !strings.HasPrefix(err.Error(), "RufsError: ") {

			// Check if the error is known to report that a file or directory
			// does not exist.

			err = &Error{ErrRemoteAction, err.Error(), err.Error() == os.ErrNotExist.Error()}
		}

		return err
	}

	c.maplock.Lock()
	laddr, ok := c.peers[node]
	c.maplock.Unlock()

	if ok {

		// Get network connection to the node

		c.maplock.Lock()
		conn, ok := c.conns[node]
		c.maplock.Unlock()

		if !ok {

			// Create a new connection if necessary

			nconn, err := net.DialTimeout("tcp", laddr, DialTimeout)

			if err != nil {
				LogDebug(c.token.NodeName, ": ",
					fmt.Sprintf("- %v.%v (laddr=%v err=%v)",
						node, remoteCall, laddr, err))
				return nil, handleError(err)
			}

			if c.cert != nil && c.cert.Certificate[0] != nil {

				// Wrap the conn in a TLS client

				config := tls.Config{
					Certificates:       []tls.Certificate{*c.cert},
					InsecureSkipVerify: true,
				}

				tlsconn := tls.Client(nconn, &config)

				// Do the handshake and look at the server certificate

				tlsconn.Handshake()
				rfp := fingerprint(tlsconn.ConnectionState().PeerCertificates[0].Raw)

				c.maplock.Lock()
				expected, _ := c.fingerprints[node]
				c.maplock.Unlock()

				if expected == "" {

					// Accept the certificate and store it

					c.maplock.Lock()
					c.fingerprints[node] = rfp
					c.maplock.Unlock()

				} else if expected != rfp {

					// Fingerprint was NOT verified

					LogDebug(c.token.NodeName, ": ",
						fmt.Sprintf("Not trusting %v (laddr=%v) presented fingerprint: %v expected fingerprint: %v", node, laddr, rfp, expected))

					return nil, &Error{ErrUntrustedTarget, node, false}
				}

				LogDebug(c.token.NodeName, ": ",
					fmt.Sprintf("%v (laddr=%v) has SSL fingerprint %v ", node, laddr, rfp))

				nconn = tlsconn
			}

			conn = rpc.NewClient(nconn)

			// Store the connection so it can be reused

			c.maplock.Lock()
			c.conns[node] = conn
			c.maplock.Unlock()
		}

		// Assemble the request

		request := map[RequestArgument]interface{}{
			RequestTARGET: node,
			RequestTOKEN:  c.token,
		}

		if args != nil {
			for k, v := range args {
				request[k] = v
			}
		}

		var response interface{}

		LogDebug(c.token.NodeName, ": ",
			fmt.Sprintf("> %v.%v (laddr=%v)", node, remoteCall, laddr))

		err = conn.Call("RufsServer."+string(remoteCall), request, &response)

		if !c.redial && (err == rpc.ErrShutdown || err == io.EOF || err == io.ErrUnexpectedEOF) {

			// Delete the closed connection and retry the request

			c.maplock.Lock()
			delete(c.conns, node)
			c.redial = true // Set the redial flag to avoid a forever loop
			c.maplock.Unlock()

			return c.SendRequest(node, remoteCall, args)
		}

		// Reset redial flag

		c.maplock.Lock()
		c.redial = false
		c.maplock.Unlock()

		LogDebug(c.token.NodeName, ": ",
			fmt.Sprintf("< %v.%v (err=%v)", node, remoteCall, err))

		return response, handleError(err)
	}

	return nil, &Error{ErrUnknownTarget, node, false}
}
