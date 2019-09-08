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
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"devt.de/krotik/rufs"
	"devt.de/krotik/rufs/api"
)

/*
EndpointZip is the zip endpoint URL (rooted). Handles everything
under zip/...
*/
const EndpointZip = api.APIRoot + APIv1 + "/zip/"

/*
ZipEndpointInst creates a new endpoint handler.
*/
func ZipEndpointInst() api.RestEndpointHandler {
	return &zipEndpoint{}
}

/*
Handler object for zip operations.
*/
type zipEndpoint struct {
	*api.DefaultEndpointHandler
}

/*
HandlePOST handles a zip query REST call.
*/
func (z *zipEndpoint) HandlePOST(w http.ResponseWriter, r *http.Request, resources []string) {
	var tree *rufs.Tree
	var data []string
	var ok bool
	var err error

	if !checkResources(w, resources, 1, 1, "Need a tree name") {
		return
	}

	if tree, ok, err = api.GetTree(resources[0]); err == nil && !ok {
		http.Error(w, fmt.Sprintf("Unknown tree: %v", resources[0]), http.StatusBadRequest)
		return
	}

	if err = r.ParseForm(); err == nil {
		files := r.Form["files"]

		if len(files) == 0 {
			err = fmt.Errorf("Field 'files' should be a list of files as JSON encoded string")
		} else {
			err = json.NewDecoder(bytes.NewBufferString(files[0])).Decode(&data)
		}
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("Could not decode request body: %v", err.Error()),
			http.StatusBadRequest)
		return
	}

	w.Header().Set("content-type", "application/octet-stream")
	w.Header().Set("content-disposition", `attachment; filename="files.zip"`)

	// Go through the list of files and stream the zip file

	zipW := zip.NewWriter(w)

	for _, f := range data {
		writer, _ := zipW.Create(f)
		tree.ReadFileToBuffer(f, writer)
	}

	zipW.Close()
}

/*
SwaggerDefs is used to describe the endpoint in swagger.
*/
func (z *zipEndpoint) SwaggerDefs(s map[string]interface{}) {

	s["paths"].(map[string]interface{})["/v1/zip/{tree}"] = map[string]interface{}{
		"post": map[string]interface{}{
			"summary":     "Create zip file from a list of files.",
			"description": "Combine a list of given files into a single zip file.",
			"produces": []string{
				"text/plain",
			},
			"consumes": []string{
				"application/x-www-form-urlencoded",
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
					"name":        "files",
					"in":          "body",
					"description": "JSON encoded list of (full path) files which should be zipped up",
					"required":    true,
					"schema": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"description": "File (with full path) which should be included in the zip file.",
							"type":        "string",
						},
					},
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Returns the content of the requested file.",
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
}
