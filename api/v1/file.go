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
	"mime/multipart"
	"net/http"
	"path"
	"time"

	"devt.de/krotik/common/cryptutil"
	"devt.de/krotik/common/datautil"
	"devt.de/krotik/common/errorutil"
	"devt.de/krotik/common/httputil"
	"devt.de/krotik/rufs"
	"devt.de/krotik/rufs/api"
)

// Progress endpoint
// =================

/*
Progress is a persisted data structure which contains the current
progress of an ongoing operation.
*/
type Progress struct {
	Op            string   // Operation which we show progress of
	Subject       string   // Subject on which the operation is performed
	Progress      int64    // Current progress of the ongoing operation (this is reset for each item)
	TotalProgress int64    // Total progress required until current operation is finished
	Item          int64    // Current processing item
	TotalItems    int64    // Total number of items to process
	Errors        []string // Any error messages
}

/*
JSONString returns the progress object as a JSON string.
*/
func (p *Progress) JSONString() []byte {
	ret, err := json.MarshalIndent(map[string]interface{}{
		"operation":      p.Op,
		"subject":        p.Subject,
		"progress":       p.Progress,
		"total_progress": p.TotalProgress,
		"item":           p.Item,
		"total_items":    p.TotalItems,
		"errors":         p.Errors,
	}, "", "    ")
	errorutil.AssertOk(err)
	return ret
}

/*
ProgressMap contains information about copy progress.
*/
var ProgressMap = datautil.NewMapCache(100, 0)

/*
EndpointProgress is the progress endpoint URL (rooted). Handles everything
under progress/...
*/
const EndpointProgress = api.APIRoot + APIv1 + "/progress/"

/*
ProgressEndpointInst creates a new endpoint handler.
*/
func ProgressEndpointInst() api.RestEndpointHandler {
	return &progressEndpoint{}
}

/*
Handler object for progress operations.
*/
type progressEndpoint struct {
	*api.DefaultEndpointHandler
}

/*
HandleGET handles a progress query REST call.
*/
func (f *progressEndpoint) HandleGET(w http.ResponseWriter, r *http.Request, resources []string) {
	var ok bool
	var err error

	if len(resources) < 2 {
		http.Error(w, "Need a tree name and a progress ID",
			http.StatusBadRequest)
		return
	}

	if _, ok, err = api.GetTree(resources[0]); err == nil && !ok {
		err = fmt.Errorf("Unknown tree: %v", resources[0])
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	p, ok := ProgressMap.Get(resources[0] + "#" + resources[1])

	if !ok {
		http.Error(w, fmt.Sprintf("Unknown progress ID: %v", resources[1]),
			http.StatusBadRequest)
		return
	}

	w.Header().Set("content-type", "application/octet-stream")
	w.Write(p.(*Progress).JSONString())
}

/*
SwaggerDefs is used to describe the endpoint in swagger.
*/
func (f *progressEndpoint) SwaggerDefs(s map[string]interface{}) {

	s["paths"].(map[string]interface{})["/v1/progress/{tree}/{progress_id}"] = map[string]interface{}{
		"get": map[string]interface{}{
			"summary":     "Request progress update.",
			"description": "Return a progress object showing the progress of an ongoing operation.",
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
					"name":        "progress_id",
					"in":          "path",
					"description": "Id of progress object.",
					"required":    true,
					"type":        "string",
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Returns the requested progress object.",
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

// File endpoint
// =============

/*
EndpointFile is the file endpoint URL (rooted). Handles everything
under file/...
*/
const EndpointFile = api.APIRoot + APIv1 + "/file/"

/*
FileEndpointInst creates a new endpoint handler.
*/
func FileEndpointInst() api.RestEndpointHandler {
	return &fileEndpoint{}
}

/*
Handler object for file operations.
*/
type fileEndpoint struct {
	*api.DefaultEndpointHandler
}

/*
HandleGET handles a file query REST call.
*/
func (f *fileEndpoint) HandleGET(w http.ResponseWriter, r *http.Request, resources []string) {
	var tree *rufs.Tree
	var ok bool
	var err error

	if len(resources) < 2 {
		http.Error(w, "Need a tree name and a file path",
			http.StatusBadRequest)
		return
	}

	if tree, ok, err = api.GetTree(resources[0]); err == nil && !ok {
		err = fmt.Errorf("Unknown tree: %v", resources[0])
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("content-type", "application/octet-stream")

	if err := tree.ReadFileToBuffer(path.Join(resources[1:]...), w); err != nil {
		http.Error(w, fmt.Sprintf("Could not read file %v: %v", path.Join(resources[1:]...), err.Error()),
			http.StatusBadRequest)
		return
	}
}

/*
HandlePUT handles REST calls to modify / copy existing files.
*/
func (f *fileEndpoint) HandlePUT(w http.ResponseWriter, r *http.Request, resources []string) {
	f.handleFileOp("PUT", w, r, resources)
}

/*
HandleDELETE handles REST calls to delete existing files.
*/
func (f *fileEndpoint) HandleDELETE(w http.ResponseWriter, r *http.Request, resources []string) {
	f.handleFileOp("DELETE", w, r, resources)
}

func (f *fileEndpoint) handleFileOp(requestType string, w http.ResponseWriter, r *http.Request, resources []string) {
	var action string
	var data, ret map[string]interface{}
	var tree *rufs.Tree
	var ok bool
	var err error
	var files []string

	if len(resources) < 1 {
		http.Error(w, "Need a tree name and a file path",
			http.StatusBadRequest)
		return
	} else if len(resources) == 1 {
		resources = append(resources, "/")
	}

	if tree, ok, err = api.GetTree(resources[0]); err == nil && !ok {
		err = fmt.Errorf("Unknown tree: %v", resources[0])
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ret = make(map[string]interface{})

	if requestType == "DELETE" {

		// See if the request contains a body with a list of files

		err = json.NewDecoder(r.Body).Decode(&files)

	} else {

		// Unless it is a delete request we need an action command

		err = json.NewDecoder(r.Body).Decode(&data)

		if err != nil {
			http.Error(w, fmt.Sprintf("Could not decode request body: %v", err.Error()),
				http.StatusBadRequest)
			return
		}

		actionObj, ok := data["action"]

		if !ok {
			http.Error(w, fmt.Sprintf("Action command is missing from request body"),
				http.StatusBadRequest)
			return
		}

		action = fmt.Sprint(actionObj)
	}

	fullPath := path.Join(resources[1:]...)
	if fullPath != "/" {
		fullPath = "/" + fullPath
	}

	dir, file := path.Split(fullPath)

	if requestType == "DELETE" {

		if len(files) == 0 {
			_, err = tree.ItemOp(dir, map[string]string{
				rufs.ItemOpAction: rufs.ItemOpActDelete,
				rufs.ItemOpName:   file,
			})

		} else {

			// Delete the files given in the body

			for _, f := range files {
				dir, file := path.Split(f)

				if err == nil {
					_, err = tree.ItemOp(dir, map[string]string{
						rufs.ItemOpAction: rufs.ItemOpActDelete,
						rufs.ItemOpName:   file,
					})
				}
			}
		}

	} else if action == "rename" {

		if newNamesParam, ok := data["newnames"]; ok {

			if newNames, ok := newNamesParam.([]interface{}); !ok {
				err = fmt.Errorf("Parameter newnames must be a list of filenames")
			} else {

				if filesParam, ok := data["files"]; !ok {
					err = fmt.Errorf("Parameter files is missing from request body")
				} else {

					if filesList, ok := filesParam.([]interface{}); !ok {
						err = fmt.Errorf("Parameter files must be a list of files")
					} else {

						for i, f := range filesList {
							dir, file := path.Split(fmt.Sprint(f))

							if err == nil {
								_, err = tree.ItemOp(dir, map[string]string{
									rufs.ItemOpAction:  rufs.ItemOpActRename,
									rufs.ItemOpName:    file,
									rufs.ItemOpNewName: fmt.Sprint(newNames[i]),
								})
							}
						}
					}
				}
			}

		} else {

			newName, ok := data["newname"]

			if !ok {
				err = fmt.Errorf("Parameter newname is missing from request body")

			} else {

				_, err = tree.ItemOp(dir, map[string]string{
					rufs.ItemOpAction:  rufs.ItemOpActRename,
					rufs.ItemOpName:    file,
					rufs.ItemOpNewName: fmt.Sprint(newName),
				})
			}
		}

	} else if action == "mkdir" {

		_, err = tree.ItemOp(dir, map[string]string{
			rufs.ItemOpAction: rufs.ItemOpActMkDir,
			rufs.ItemOpName:   file,
		})

	} else if action == "copy" {

		dest, ok := data["destination"]

		if !ok {
			err = fmt.Errorf("Parameter destination is missing from request body")

		} else {

			// Create file list

			filesParam, hasFilesParam := data["files"]

			if hasFilesParam {

				if lf, ok := filesParam.([]interface{}); !ok {
					err = fmt.Errorf("Parameter files must be a list of files")

				} else {
					files = make([]string, len(lf))

					for i, f := range lf {
						files[i] = fmt.Sprint(f)
					}
				}

			} else {

				files = []string{fullPath}
			}

			if err == nil {

				// Create progress object

				uuid := fmt.Sprintf("%x", cryptutil.GenerateUUID())
				ret["progress_id"] = uuid

				mapLookup := resources[0] + "#" + uuid

				ProgressMap.Put(mapLookup, &Progress{
					Op:            "Copy",
					Subject:       "",
					Progress:      0,
					TotalProgress: 0,
					Item:          0,
					TotalItems:    int64(len(files)),
					Errors:        []string{},
				})

				go func() {

					err = tree.Copy(files, fmt.Sprint(dest),
						func(file string, writtenBytes, totalBytes, currentFile, totalFiles int64) {

							if p, ok := ProgressMap.Get(mapLookup); ok && writtenBytes > 0 {
								p.(*Progress).Subject = file
								p.(*Progress).Progress = writtenBytes
								p.(*Progress).TotalProgress = totalBytes
								p.(*Progress).Item = currentFile
								p.(*Progress).TotalItems = totalFiles
							}
						})

					if err != nil {
						if p, ok := ProgressMap.Get(mapLookup); ok {
							p.(*Progress).Errors = append(p.(*Progress).Errors, err.Error())
						}
					}
				}()

				// Wait a little bit so immediate errors are directly reported

				time.Sleep(10 * time.Millisecond)
			}
		}

	} else if action == "sync" {

		dest, ok := data["destination"]

		if !ok {
			err = fmt.Errorf("Parameter destination is missing from request body")

		} else {

			uuid := fmt.Sprintf("%x", cryptutil.GenerateUUID())
			ret["progress_id"] = uuid

			mapLookup := resources[0] + "#" + uuid

			ProgressMap.Put(mapLookup, &Progress{
				Op:            "Sync",
				Subject:       "",
				Progress:      0,
				TotalProgress: -1,
				Item:          0,
				TotalItems:    -1,
				Errors:        []string{},
			})

			go func() {

				err = tree.Sync(fullPath, fmt.Sprint(dest), true,
					func(op, srcFile, dstFile string, writtenBytes, totalBytes, currentFile, totalFiles int64) {

						if p, ok := ProgressMap.Get(mapLookup); ok && writtenBytes > 0 {

							p.(*Progress).Op = op
							p.(*Progress).Subject = srcFile
							p.(*Progress).Progress = writtenBytes
							p.(*Progress).TotalProgress = totalBytes
							p.(*Progress).Item = currentFile
							p.(*Progress).TotalItems = totalFiles
						}
					})

				if err != nil {
					if p, ok := ProgressMap.Get(mapLookup); ok {
						p.(*Progress).Errors = append(p.(*Progress).Errors, err.Error())
					}
				}

			}()

			// Wait a little bit so immediate errors are directly reported

			time.Sleep(10 * time.Millisecond)
		}

	} else {

		err = fmt.Errorf("Unknown action: %v", action)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Write data

	w.Header().Set("content-type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(ret)
}

/*
HandlePOST handles REST calls to create or overwrite a new file.
*/
func (f *fileEndpoint) HandlePOST(w http.ResponseWriter, r *http.Request, resources []string) {
	var err error
	var tree *rufs.Tree
	var ok bool

	if len(resources) < 1 {
		http.Error(w, "Need a tree name and a file path",
			http.StatusBadRequest)
		return
	}

	if tree, ok, err = api.GetTree(resources[0]); err == nil && !ok {
		err = fmt.Errorf("Unknown tree: %v", resources[0])
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check we have the right request type

	if r.MultipartForm == nil {
		if err = r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, fmt.Sprintf("Could not read request body: %v", err.Error()),
				http.StatusBadRequest)
			return
		}
	}

	if r.MultipartForm != nil && r.MultipartForm.File != nil {

		// Check the files are in the form field uploadfile

		files, ok := r.MultipartForm.File["uploadfile"]
		if !ok {
			http.Error(w, "Could not find 'uploadfile' form field",
				http.StatusBadRequest)
			return
		}

		for _, file := range files {
			var f multipart.File

			// Write out all send files

			if f, err = file.Open(); err == nil {
				err = tree.WriteFileFromBuffer(path.Join(path.Join(resources[1:]...), file.Filename), f)
			}

			if err != nil {
				http.Error(w, fmt.Sprintf("Could not write file %v: %v", path.Join(resources[1:]...)+file.Filename, err.Error()),
					http.StatusBadRequest)
				return
			}
		}
	}

	if redirect := r.PostFormValue("redirect"); redirect != "" {

		// Do the redirect - make sure it is a local redirect

		if err = httputil.CheckLocalRedirect(redirect); err != nil {
			http.Error(w, fmt.Sprintf("Could not redirect: %v", err.Error()),
				http.StatusBadRequest)
			return
		}

		http.Redirect(w, r, redirect, http.StatusFound)
	}
}

/*
SwaggerDefs is used to describe the endpoint in swagger.
*/
func (f *fileEndpoint) SwaggerDefs(s map[string]interface{}) {

	s["paths"].(map[string]interface{})["/v1/file/{tree}/{path}"] = map[string]interface{}{
		"get": map[string]interface{}{
			"summary":     "Read a file.",
			"description": "Return the contents of a file.",
			"produces": []string{
				"text/plain",
				"application/octet-stream",
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
					"description": "File path.",
					"required":    true,
					"type":        "string",
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
		"put": map[string]interface{}{
			"summary":     "Perform a file operation.",
			"description": "Perform a file operation like rename or copy.",
			"consumes": []string{
				"application/json",
			},
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
					"description": "File path.",
					"required":    true,
					"type":        "string",
				},

				{
					"name":        "operation",
					"in":          "body",
					"description": "Operation which should be executes",
					"required":    true,
					"schema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"action": map[string]interface{}{
								"description": "Action to perform.",
								"type":        "string",
								"enum": []string{
									"rename",
									"mkdir",
									"copy",
									"sync",
								},
							},
							"newname": map[string]interface{}{
								"description": "New filename when renaming a single file.",
								"type":        "string",
							},
							"newnames": map[string]interface{}{
								"description": "List of new file names when renaming multiple files using the files parameter.",
								"type":        "array",
								"items": map[string]interface{}{
									"description": "New filename.",
									"type":        "string",
								},
							},
							"destination": map[string]interface{}{
								"description": "Destination directory when copying files.",
								"type":        "string",
							},
							"files": map[string]interface{}{
								"description": "List of (full path) files which should be copied / renamed.",
								"type":        "array",
								"items": map[string]interface{}{
									"description": "File (with full path) which should be copied / renamed.",
									"type":        "string",
								},
							},
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
		"post": map[string]interface{}{
			"summary":     "Upload a file.",
			"description": "Upload or overwrite a file.",
			"produces": []string{
				"text/plain",
			},
			"consumes": []string{
				"multipart/form-data",
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
					"description": "File path.",
					"required":    true,
					"type":        "string",
				},
				{
					"name":        "redirect",
					"in":          "formData",
					"description": "Page to redirect to after processing the request.",
					"required":    false,
					"type":        "string",
				},
				{
					"name":        "uploadfile",
					"in":          "formData",
					"description": "File(s) to create / overwrite.",
					"required":    true,
					"type":        "file",
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Successful upload no redirect parameter given.",
				},
				"302": map[string]interface{}{
					"description": "Successful upload - redirect according to the given redirect parameter.",
				},
				"default": map[string]interface{}{
					"description": "Error response",
					"schema": map[string]interface{}{
						"$ref": "#/definitions/Error",
					},
				},
			},
		},
		"delete": map[string]interface{}{
			"summary":     "Delete a file or directory.",
			"description": "Delete a file or directory.",
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
					"name":        "path",
					"in":          "path",
					"description": "File or directory path.",
					"required":    true,
					"type":        "string",
				},
				{
					"name":        "filelist",
					"in":          "body",
					"description": "List of (full path) files which should be deleted",
					"required":    false,
					"schema": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"description": "File (with full path) which should be deleted.",
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

	// Add generic error object to definition

	s["definitions"].(map[string]interface{})["Error"] = map[string]interface{}{
		"description": "A human readable error mesage.",
		"type":        "string",
	}
}
