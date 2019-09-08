/*
 * Rufs - Remote Union File System
 *
 * Copyright 2017 Matthias Ladkau. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the MIT
 * License, If a copy of the MIT License was not distributed with this
 * file, You can obtain one at https://opensource.org/licenses/MIT.
 */

package api

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"devt.de/krotik/rufs"
)

/*
APIVersion is the version of the REST API
*/
const APIVersion = "1.0.0"

/*
APIRoot is the API root directory for the REST API
*/
const APIRoot = "/fs"

/*
APISchemes defines the supported schemes by the REST API
*/
var APISchemes = []string{"https"}

/*
APIHost is the host definition for the REST API
*/
var APIHost = "localhost:9040"

/*
RestEndpointInst models a factory function for REST endpoint handlers.
*/
type RestEndpointInst func() RestEndpointHandler

/*
GeneralEndpointMap is a map of urls to general REST endpoints
*/
var GeneralEndpointMap = map[string]RestEndpointInst{
	EndpointAbout:   AboutEndpointInst,
	EndpointSwagger: SwaggerEndpointInst,
}

/*
RestEndpointHandler models a REST endpoint handler.
*/
type RestEndpointHandler interface {

	/*
		HandleGET handles a GET request.
	*/
	HandleGET(w http.ResponseWriter, r *http.Request, resources []string)

	/*
		HandlePOST handles a POST request.
	*/
	HandlePOST(w http.ResponseWriter, r *http.Request, resources []string)

	/*
		HandlePUT handles a PUT request.
	*/
	HandlePUT(w http.ResponseWriter, r *http.Request, resources []string)

	/*
		HandleDELETE handles a DELETE request.
	*/
	HandleDELETE(w http.ResponseWriter, r *http.Request, resources []string)

	/*
		SwaggerDefs is used to describe the endpoint in swagger.
	*/
	SwaggerDefs(s map[string]interface{})
}

/*
trees is a map of all trees which can be used by the REST API
*/
var trees = make(map[string]*rufs.Tree)
var treesLock = sync.Mutex{}

/*
ResetTrees removes all registered trees.
*/
var ResetTrees = func() {
	treesLock.Lock()
	defer treesLock.Unlock()

	trees = make(map[string]*rufs.Tree)
}

/*
Trees is a getter function which returns a map of all registered trees.
This function can be overwritten by client code to implement access
control.
*/
var Trees = func() (map[string]*rufs.Tree, error) {
	treesLock.Lock()
	defer treesLock.Unlock()

	ret := make(map[string]*rufs.Tree)

	for k, v := range trees {
		ret[k] = v
	}

	return ret, nil
}

/*
GetTree returns a specific tree. This function can be overwritten by
client code to implement access control.
*/
var GetTree = func(id string) (*rufs.Tree, bool, error) {
	treesLock.Lock()
	defer treesLock.Unlock()

	tree, ok := trees[id]

	return tree, ok, nil
}

/*
AddTree adds a new tree. This function can be overwritten by
client code to implement access control.
*/
var AddTree = func(id string, tree *rufs.Tree) error {
	treesLock.Lock()
	defer treesLock.Unlock()

	if _, ok := trees[id]; ok {
		return fmt.Errorf("Tree %v already exists", id)
	}

	trees[id] = tree

	return nil
}

/*
RemoveTree removes a tree. This function can be overwritten by
client code to implement access control.
*/
var RemoveTree = func(id string) error {
	treesLock.Lock()
	defer treesLock.Unlock()

	if _, ok := trees[id]; !ok {
		return fmt.Errorf("Tree %v does not exist", id)
	}

	delete(trees, id)

	return nil
}

/*
TreeConfigTemplate is the configuration which is used for newly created trees.
*/
var TreeConfigTemplate map[string]interface{}

/*
TreeCertTemplate is the certificate which is used for newly created trees.
*/
var TreeCertTemplate *tls.Certificate

/*
Map of all registered endpoint handlers.
*/
var registered = map[string]RestEndpointInst{}

/*
HandleFunc to use for registering handlers
*/
var HandleFunc = http.HandleFunc

/*
RegisterRestEndpoints registers all given REST endpoint handlers.
*/
func RegisterRestEndpoints(endpointInsts map[string]RestEndpointInst) {

	for url, endpointInst := range endpointInsts {
		registered[url] = endpointInst

		HandleFunc(url, func() func(w http.ResponseWriter, r *http.Request) {
			var handlerURL = url
			var handlerInst = endpointInst

			return func(w http.ResponseWriter, r *http.Request) {

				// Create a new handler instance

				handler := handlerInst()

				// Handle request in appropriate method

				res := strings.TrimSpace(r.URL.Path[len(handlerURL):])

				if len(res) > 0 && res[len(res)-1] == '/' {
					res = res[:len(res)-1]
				}

				var resources []string

				if res != "" {
					resources = strings.Split(res, "/")
				}

				switch r.Method {
				case "GET":
					handler.HandleGET(w, r, resources)

				case "POST":
					handler.HandlePOST(w, r, resources)

				case "PUT":
					handler.HandlePUT(w, r, resources)

				case "DELETE":
					handler.HandleDELETE(w, r, resources)

				default:
					http.Error(w, http.StatusText(http.StatusMethodNotAllowed),
						http.StatusMethodNotAllowed)
				}
			}
		}())
	}
}

/*
DefaultEndpointHandler is the default endpoint handler implementation.
*/
type DefaultEndpointHandler struct {
}

/*
HandleGET handles a GET request.
*/
func (de *DefaultEndpointHandler) HandleGET(w http.ResponseWriter, r *http.Request, resources []string) {
	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

/*
HandlePOST handles a POST request.
*/
func (de *DefaultEndpointHandler) HandlePOST(w http.ResponseWriter, r *http.Request, resources []string) {
	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

/*
HandlePUT handles a PUT request.
*/
func (de *DefaultEndpointHandler) HandlePUT(w http.ResponseWriter, r *http.Request, resources []string) {
	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

/*
HandleDELETE handles a DELETE request.
*/
func (de *DefaultEndpointHandler) HandleDELETE(w http.ResponseWriter, r *http.Request, resources []string) {
	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}
