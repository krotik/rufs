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
	"fmt"
	"net/http"
	"strings"

	"devt.de/krotik/rufs/api"
)

/*
APIv1 is the directory for version 1 of the API
*/
const APIv1 = "/v1"

/*
V1EndpointMap is a map of urls to endpoints for version 1 of the API
*/
var V1EndpointMap = map[string]api.RestEndpointInst{
	EndpointAdmin:    AdminEndpointInst,
	EndpointDir:      DirEndpointInst,
	EndpointFile:     FileEndpointInst,
	EndpointProgress: ProgressEndpointInst,
	EndpointZip:      ZipEndpointInst,
}

// Helper functions
// ================

/*
checkResources check given resources for a GET request.
*/
func checkResources(w http.ResponseWriter, resources []string, requiredMin int, requiredMax int, errorMsg string) bool {
	if len(resources) < requiredMin {
		http.Error(w, errorMsg, http.StatusBadRequest)
		return false
	} else if len(resources) > requiredMax {
		http.Error(w, "Invalid resource specification: "+strings.Join(resources[1:], "/"), http.StatusBadRequest)
		return false
	}
	return true
}

/*
getMapValue extracts a value from a given map.
*/
func getMapValue(w http.ResponseWriter, data map[string]interface{}, key string) (string, bool) {

	if val, ok := data[key]; ok && val != "" {
		return fmt.Sprint(val), true
	}

	http.Error(w, fmt.Sprintf("Value for %v is missing in posted data", key), http.StatusBadRequest)

	return "", false
}
