package api

import (
	"testing"

	"devt.de/krotik/rufs/config"
)

func TestAboutEndpoint(t *testing.T) {

	st, _, body, _ := sendTestRequest(testQueryURL+EndpointAbout, "GET", nil)

	if st != "200 OK" || body != `
{
  "product": "RUFS",
  "version": "`[1:]+config.ProductVersion+`"
}` {
		t.Error("Unexpected response:", st, body)
		return
	}
}
