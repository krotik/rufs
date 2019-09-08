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
Package api contains the REST API for RUFS.

/about

Endpoint which returns an object with version information.

{
    api_versions : List of available API versions e.g. [ "v1" ]
    product      : Name of the API provider (RUFS)
    version:     : Version of the API provider
}
*/
package api

import (
	"encoding/json"
	"net/http"

	"devt.de/krotik/rufs/config"
)

/*
EndpointAbout is the about endpoint definition (rooted). Handles about/
*/
const EndpointAbout = APIRoot + "/about/"

/*
AboutEndpointInst creates a new endpoint handler.
*/
func AboutEndpointInst() RestEndpointHandler {
	return &aboutEndpoint{}
}

/*
aboutEndpoint is the handler object for about operations.
*/
type aboutEndpoint struct {
	*DefaultEndpointHandler
}

/*
HandleGET returns about data for the REST API.
*/
func (a *aboutEndpoint) HandleGET(w http.ResponseWriter, r *http.Request, resources []string) {

	data := map[string]interface{}{
		"product": "RUFS",
		"version": config.ProductVersion,
	}

	// Write data

	w.Header().Set("content-type", "application/json; charset=utf-8")

	ret := json.NewEncoder(w)
	ret.Encode(data)
}
