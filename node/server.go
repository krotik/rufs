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
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"net/rpc"

	"devt.de/krotik/common/errorutil"
)

func init() {

	// Create singleton Server instance.

	rufsServer = &RufsServer{make(map[string]*RufsNode)}

	// Register the cluster API as RPC server

	errorutil.AssertOk(rpc.Register(rufsServer))
}

/*
RPCFunction is used to identify the called function in a RPC call
*/
type RPCFunction string

/*
List of all possible RPC functions. The list includes all RPC callable functions
in this file.
*/
const (

	// General functions

	RPCPing RPCFunction = "Ping"
	RPCData RPCFunction = "Data"
)

/*
RequestArgument is used to identify arguments in a RPC call
*/
type RequestArgument int

/*
List of all possible arguments in a RPC request. There are usually no checks which
give back an error if a required argument is missing. The RPC API is an internal
API and might change without backwards compatibility.
*/
const (

	// General arguments

	RequestTARGET RequestArgument = iota // Required argument which identifies the target node
	RequestTOKEN                         // Client token which is used for authorization checks
	RequestCTRL                          // Control object (i.e. what to do with the data)
	RequestDATA                          // Data object
)

/*
rufsServer is the Server instance which serves rpc calls
*/
var rufsServer *RufsServer

/*
RufsServer is the RPC exposed Rufs API of a machine. Server is a singleton and will
route incoming (authenticated) requests to registered RufsNodes. The calling
node is referred to as source node and the called node is referred to as
target node.
*/
type RufsServer struct {
	nodes map[string]*RufsNode // Map of local RufsNodes
}

// General functions
// =================

/*
Ping answers with a Pong if the given client token was verified and the local
node exists.
*/
func (s *RufsServer) Ping(request map[RequestArgument]interface{},
	response *interface{}) error {

	// Verify the given token and retrieve the target member

	if _, err := s.checkToken(request); err != nil {
		return err
	}

	// Send a simple response

	res := []string{"Pong"}

	*response = res

	return nil
}

/*
Data handles data requests.
*/
func (s *RufsServer) Data(request map[RequestArgument]interface{},
	response *interface{}) error {

	// Verify the given token and retrieve the target member

	node, err := s.checkToken(request)

	if err != nil || node.DataHandler == nil {
		return err
	}

	// Forward to the registered data handler

	res, err := node.DataHandler(request[RequestCTRL].(map[string]string),
		request[RequestDATA].([]byte))

	if err == nil {
		*response = res
	}

	return err
}

// Helper functions
// ================

/*
checkToken checks the member token in a given request.
*/
func (s *RufsServer) checkToken(request map[RequestArgument]interface{}) (*RufsNode, error) {
	err := ErrUnknownTarget

	// Get the target member

	target := request[RequestTARGET].(string)
	token := request[RequestTOKEN].(*RufsNodeToken)

	if node, ok := s.nodes[target]; ok {
		err = ErrInvalidToken

		// Generate expected auth from given requesting node name in token and secret of target

		expectedAuth := fmt.Sprintf("%X", sha512.Sum512_224([]byte(token.NodeName+node.secret)))

		if token.NodeAuth == expectedAuth {
			return node, nil
		}
	}

	return nil, err
}

/*
fingerprint converts a given set of bytes to a fingerprint.
*/
func fingerprint(b []byte) string {
	var buf bytes.Buffer

	hs := fmt.Sprintf("%x", sha256.Sum256(b))

	for i, c := range hs {
		buf.WriteByte(byte(c))
		if (i+1)%2 == 0 && i != len(hs)-1 {
			buf.WriteByte(byte(':'))
		}
	}

	return buf.String()
}
