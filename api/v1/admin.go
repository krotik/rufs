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
Package v1 contains Rufs REST API Version 1.

Admin control endpoint

/admin

The admin endpoint can be used for various admin tasks such as registering
new branches or mounting known branches.

A GET request to the admin endpoint returns the current tree
configuration; an object of all known branches and the current mapping:

	{
	    branches : [ <known branches> ],
	    tree  : [ <current mapping> ]
	}

A POST request to the admin endpoint creates a new tree. The body of
the request should have the following form:

	"<name>"

/admin/<tree>

A DELETE request to a particular tree will delete the tree.

/admin/<tree>/branch

A new branch can be created in an existing tree by sending a POST request
to the branch endpoint. The body of the request should have the following
form:

	{
	    branch : <Name of the branch>,
	    rpc : <RPC definition of the remote branch (e.g. localhost:9020)>,
	    fingerprint : <Expected SSL fingerprint of the remote branch or an empty string>
	}

/admin/<tree>/mapping

A new mapping can be created in an existing tree by sending a POST request
to the mapping endpoint. The body of the request should have the following
form:

	{
	    branch : <Name of the branch>,
	    dir : <Tree directory of the branch root>,
	    writable : <Flag if the branch should handle write operations>
	}


Dir listing endpoing

/dir/<tree>/<path>

The dir endpoing handles requests for the directory listing of a certain
path. A request url should be of the following form:

/dir/<tree>/<path>?recursive=<flag>&checksums=<flag>

The request can optionally include the flag parameters (value should
be 1 or 0) recursive and checksums. The recursive flag will add all
subdirectories to the listing and the checksums flag will add
checksums for all listed files.


File queries and manipulation

/file/{tree}/{path}

A GET request to a specific file will return its contents. A POST will
upload a new or overwrite an existing file. A DELETE request will delete
an existing file.

New files are expected to be uploaded using a multipart/form-data request.
When uploading a new file the form field for the file should be named
"uploadfile". The form can optionally contain a redirect field which
will issue a redirect once the file has been uploaded.

A PUT request is used to perform a file operation. The request body
should be a JSON object of the form (parameters are operation specific):

	{
	    action : <Action to perform>,
		files : <List of (full path) files which should be copied / renamed>
	    newname : <New name of file (when renaming)>,
		newnames : <List of new file names when renaming multiple files using
					the files parameter>,
	    destination : <Destination file when copying a single file - Destination
						directory when copying multiple files using the files
						parameter or syncing directories>
	}

The action can either be: sync, rename, mkdir or copy. Copy and sync returns a JSON
structure containing a progress id:

	{
	    progress_id : <Id for progress of the copy operation>
	}


Progress information

/progress/<progress id>

A GET request to the progress endpoint returns the current progress of
an ongoing operation. The result should be:

	{
	    "item": <Currently processing item>,
	    "operation": <Name of operation>,
	    "progress": <Current progress>,
	    "subject": <Name of the subject on which the operation is performed>,
	    "total_items": <Total number of items>,
	    "total_progress": <Total progress>
	}


Create zip files

/zip/<tree>

A post to the zip enpoint returns a zip file containing requested files. The
files to include must be given as a list of file name with full path in the body.
The body should be application/x-www-form-urlencoded encoded. The list should
be a JSON encoded string as value of the value files. The body should have the
following form:

	files=[ "<file1>", "<file2>" ]
*/
package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"devt.de/krotik/rufs"
	"devt.de/krotik/rufs/api"
)

/*
EndpointAdmin is the mount endpoint URL (rooted). Handles everything
under admin/...
*/
const EndpointAdmin = api.APIRoot + APIv1 + "/admin/"

/*
AdminEndpointInst creates a new endpoint handler.
*/
func AdminEndpointInst() api.RestEndpointHandler {
	return &adminEndpoint{}
}

/*
Handler object for admin operations.
*/
type adminEndpoint struct {
	*api.DefaultEndpointHandler
}

/*
HandleGET handles an admin query REST call.
*/
func (a *adminEndpoint) HandleGET(w http.ResponseWriter, r *http.Request, resources []string) {
	data := make(map[string]interface{})

	trees, err := api.Trees()

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	refreshName := r.URL.Query().Get("refresh")

	for k, v := range trees {
		var tree map[string]interface{}

		if refreshName != "" && k == refreshName {
			v.Refresh()
		}

		json.Unmarshal([]byte(v.Config()), &tree)
		data[k] = tree
	}

	// Write data

	w.Header().Set("content-type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(data)
}

/*
HandlePOST handles REST calls to create a new tree.
*/
func (a *adminEndpoint) HandlePOST(w http.ResponseWriter, r *http.Request, resources []string) {
	var tree *rufs.Tree
	var ok bool
	var err error
	var data map[string]interface{}

	if len(resources) == 0 {
		var name string

		if err := json.NewDecoder(r.Body).Decode(&name); err != nil {
			http.Error(w, fmt.Sprintf("Could not decode request body: %v", err.Error()),
				http.StatusBadRequest)
			return
		} else if name == "" {
			http.Error(w, fmt.Sprintf("Body must contain the tree name as a non-empty JSON string"),
				http.StatusBadRequest)
			return
		}

		// Create a new tree

		tree, err := rufs.NewTree(api.TreeConfigTemplate, api.TreeCertTemplate)
		if err != nil {
			http.Error(w, fmt.Sprintf("Could not create new tree: %v", err.Error()),
				http.StatusBadRequest)
			return
		}

		// Store the new tree

		if err := api.AddTree(name, tree); err != nil {
			http.Error(w, fmt.Sprintf("Could not add new tree: %v", err.Error()),
				http.StatusBadRequest)
		}

		return
	}

	if !checkResources(w, resources, 2, 2, "Need a tree name and a section (either branches or mapping)") {
		return
	}

	if tree, ok, err = api.GetTree(resources[0]); err == nil && !ok {
		err = fmt.Errorf("Unknown tree: %v", resources[0])
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, fmt.Sprintf("Could not decode request body: %v", err.Error()),
			http.StatusBadRequest)
		return
	}

	if resources[1] == "branch" {

		// Add a new branch

		if rpc, ok := getMapValue(w, data, "rpc"); ok {
			if branch, ok := getMapValue(w, data, "branch"); ok {
				if fingerprint, ok := getMapValue(w, data, "fingerprint"); ok {

					if err := tree.AddBranch(branch, rpc, fingerprint); err != nil {
						http.Error(w, fmt.Sprintf("Could not add branch: %v", err.Error()),
							http.StatusBadRequest)
					}
				}
			}
		}

	} else if resources[1] == "mapping" {

		// Add a new mapping

		if _, ok := data["dir"]; ok {

			if dir, ok := getMapValue(w, data, "dir"); ok {
				if branch, ok := getMapValue(w, data, "branch"); ok {
					if writeableStr, ok := getMapValue(w, data, "writeable"); ok {

						writeable, err := strconv.ParseBool(writeableStr)

						if err != nil {
							http.Error(w, fmt.Sprintf("Writeable value must be a boolean: %v", err.Error()),
								http.StatusBadRequest)

						} else if err := tree.AddMapping(dir, branch, writeable); err != nil {

							http.Error(w, fmt.Sprintf("Could not add branch: %v", err.Error()),
								http.StatusBadRequest)
						}
					}
				}
			}
		}
	}
}

/*
HandleDELETE handles REST calls to delete an existing tree.
*/
func (a *adminEndpoint) HandleDELETE(w http.ResponseWriter, r *http.Request, resources []string) {

	if !checkResources(w, resources, 1, 1, "Need a tree name") {
		return
	}

	// Delete the tree

	if err := api.RemoveTree(resources[0]); err != nil {
		http.Error(w, fmt.Sprintf("Could not remove tree: %v", err.Error()),
			http.StatusBadRequest)
	}
}

/*
SwaggerDefs is used to describe the endpoint in swagger.
*/
func (a *adminEndpoint) SwaggerDefs(s map[string]interface{}) {

	s["paths"].(map[string]interface{})["/v1/admin"] = map[string]interface{}{
		"get": map[string]interface{}{
			"summary":     "Return all current tree configurations.",
			"description": "All current tree configurations; each object has a list of all known branches and the current mapping.",
			"produces": []string{
				"text/plain",
				"application/json",
			},
			"parameters": []map[string]interface{}{
				{
					"name":        "refresh",
					"in":          "query",
					"description": "Refresh a particular tree (reload branches and mappings).",
					"required":    false,
					"type":        "string",
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "A key-value map of tree name to tree configuration",
				},
				"default": map[string]interface{}{
					"description": "Error response",
					"schema": map[string]interface{}{
						"$ref": "#/definitions/Error",
					},
				},
			},
		},
		"post": map[string]interface{}{
			"summary":     "Create a new tree.",
			"description": "Create a new named tree.",
			"consumes": []string{
				"application/json",
			},
			"produces": []string{
				"text/plain",
			},
			"parameters": []map[string]interface{}{
				{
					"name":        "data",
					"in":          "body",
					"description": "Name of the new tree.",
					"required":    true,
					"schema": map[string]interface{}{
						"type": "string",
					},
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Returns an empty body if successful.",
				},
				"default": map[string]interface{}{
					"description": "Error response",
					"schema": map[string]interface{}{
						"$ref": "#/definitions/Error",
					},
				},
			},
		},
	}

	s["paths"].(map[string]interface{})["/v1/admin/{tree}"] = map[string]interface{}{
		"delete": map[string]interface{}{
			"summary":     "Delete a tree.",
			"description": "Delete a named tree.",
			"produces": []string{
				"text/plain",
			},
			"parameters": []map[string]interface{}{
				{
					"name":        "tree",
					"in":          "path",
					"description": "Name of the tree.",
					"required":    true,
					"type":        "string",
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Returns an empty body if successful.",
				},
				"default": map[string]interface{}{
					"description": "Error response",
					"schema": map[string]interface{}{
						"$ref": "#/definitions/Error",
					},
				},
			},
		},
	}

	s["paths"].(map[string]interface{})["/v1/admin/{tree}/branch"] = map[string]interface{}{
		"post": map[string]interface{}{
			"summary":     "Add a new branch.",
			"description": "Add a new remote branch to the tree.",
			"consumes": []string{
				"application/json",
			},
			"produces": []string{
				"text/plain",
			},
			"parameters": []map[string]interface{}{
				{
					"name":        "tree",
					"in":          "path",
					"description": "Name of the tree.",
					"required":    true,
					"type":        "string",
				},
				{
					"name":        "data",
					"in":          "body",
					"description": "Definition of the new branch.",
					"required":    true,
					"schema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"branch": map[string]interface{}{
								"description": "Name of the remote branch (must match on the remote branch).",
								"type":        "string",
							},
							"rpc": map[string]interface{}{
								"description": "RPC definition of the remote branch (e.g. localhost:9020).",
								"type":        "string",
							},
							"fingerprint": map[string]interface{}{
								"description": "Expected SSL fingerprint of the remote branch (shown during startup) or an empty string.",
								"type":        "string",
							},
						},
					},
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Returns an empty body if successful.",
				},
				"default": map[string]interface{}{
					"description": "Error response",
					"schema": map[string]interface{}{
						"$ref": "#/definitions/Error",
					},
				},
			},
		},
	}

	s["paths"].(map[string]interface{})["/v1/admin/{tree}/mapping"] = map[string]interface{}{
		"post": map[string]interface{}{
			"summary":     "Add a new mapping.",
			"description": "Add a new mapping to the tree.",
			"consumes": []string{
				"application/json",
			},
			"produces": []string{
				"text/plain",
			},
			"parameters": []map[string]interface{}{
				{
					"name":        "tree",
					"in":          "path",
					"description": "Name of the tree.",
					"required":    true,
					"type":        "string",
				},
				{
					"name":        "data",
					"in":          "body",
					"description": "Definition of the new branch.",
					"required":    true,
					"schema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"branch": map[string]interface{}{
								"description": "Name of the known remote branch.",
								"type":        "string",
							},
							"dir": map[string]interface{}{
								"description": "Tree directory which should hold the branch root.",
								"type":        "string",
							},
							"writable": map[string]interface{}{
								"description": "Flag if the branch should be mapped as writable.",
								"type":        "string",
							},
						},
					},
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Returns an empty body if successful.",
				},
				"default": map[string]interface{}{
					"description": "Error response",
					"schema": map[string]interface{}{
						"$ref": "#/definitions/Error",
					},
				},
			},
		},
	}

	// Add generic error object to definition

	s["definitions"].(map[string]interface{})["Error"] = map[string]interface{}{
		"description": "A human readable error mesage.",
		"type":        "string",
	}
}
