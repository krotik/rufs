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
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"strconv"

	"devt.de/krotik/common/stringutil"
	"devt.de/krotik/rufs"
	"devt.de/krotik/rufs/api"
)

/*
EndpointDir is the dir endpoint URL (rooted). Handles everything
under dir/...
*/
const EndpointDir = api.APIRoot + APIv1 + "/dir/"

/*
DirEndpointInst creates a new endpoint handler.
*/
func DirEndpointInst() api.RestEndpointHandler {
	return &dirEndpoint{}
}

/*
Handler object for dir operations.
*/
type dirEndpoint struct {
	*api.DefaultEndpointHandler
}

/*
HandleGET handles a dir query REST call.
*/
func (d *dirEndpoint) HandleGET(w http.ResponseWriter, r *http.Request, resources []string) {
	var tree *rufs.Tree
	var ok, checksums bool
	var err error
	var dirs []string
	var fis [][]os.FileInfo

	if len(resources) == 0 {
		http.Error(w, "Need at least a tree name",
			http.StatusBadRequest)
		return
	}

	if tree, ok, err = api.GetTree(resources[0]); err == nil && !ok {
		err = fmt.Errorf("Unknown tree: %v", resources[0])
	}

	if err == nil {
		var rex string

		glob := r.URL.Query().Get("glob")
		recursive, _ := strconv.ParseBool(r.URL.Query().Get("recursive"))
		checksums, _ = strconv.ParseBool(r.URL.Query().Get("checksums"))

		if rex, err = stringutil.GlobToRegex(glob); err == nil {

			dirs, fis, err = tree.Dir(path.Join(resources[1:]...), rex, recursive, checksums)
		}
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	data := make(map[string]interface{})

	for i, d := range dirs {
		var flist []map[string]interface{}

		fi := fis[i]

		for _, f := range fi {
			toAdd := map[string]interface{}{
				"name":  f.Name(),
				"size":  f.Size(),
				"isdir": f.IsDir(),
			}

			if checksums {
				toAdd["checksum"] = f.(*rufs.FileInfo).Checksum()
			}

			flist = append(flist, toAdd)
		}

		data[d] = flist
	}

	// Write data

	w.Header().Set("content-type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(data)
}

/*
SwaggerDefs is used to describe the endpoint in swagger.
*/
func (d *dirEndpoint) SwaggerDefs(s map[string]interface{}) {

	s["paths"].(map[string]interface{})["/v1/dir/{tree}/{path}"] = map[string]interface{}{
		"get": map[string]interface{}{
			"summary":     "Read a directory.",
			"description": "List the contents of a directory.",
			"produces": []string{
				"text/plain",
				"application/json",
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
					"name":        "path",
					"in":          "path",
					"description": "Directory path.",
					"required":    true,
					"type":        "string",
				},
				{
					"name":        "recursive",
					"in":          "query",
					"description": "Add listings of subdirectories.",
					"required":    false,
					"type":        "boolean",
				},
				{
					"name":        "checksums",
					"in":          "query",
					"description": "Include file checksums.",
					"required":    false,
					"type":        "boolean",
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Returns a map of directories with a list of files as values.",
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
