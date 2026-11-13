package httpclient

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"sync"
)

// Boxes are now called projects. The CLI calls the /api/v1/projects routes;
// a server from before the rename only has the /api/v1/boxes ones. When a
// projects path comes back as a 404 that is not an API error (the server has
// no such route), the request is retried with the box path, and later
// requests to that server use the box paths straight away.
//
// Remove this once every server routes /api/v1/projects.

var projectPath = regexp.MustCompile(`^(/?api/v1/(?:(?:teams|agents)/[^/?]+/)?)projects([/?]|$)`)

var oldServers sync.Map // base URL => true when it only routes /api/v1/boxes

// BoxPath returns path with its projects segment named boxes, and whether
// path is a projects path at all.
func BoxPath(path string) (string, bool) {
	if !projectPath.MatchString(path) {
		return path, false
	}
	return projectPath.ReplaceAllString(path, "${1}boxes${2}"), true
}

func (c Client) routesOnlyBoxes() bool {
	_, old := oldServers.Load(c.baseURL())
	return old
}

func (c Client) rememberRoutesOnlyBoxes() {
	oldServers.Store(c.baseURL(), true)
}

func (c Client) baseURL() string {
	return strings.TrimRight(c.cfg.BaseURL, "/")
}

// missingRoute reports whether a response is the server saying it has no
// such route, rather than an API error such as an unknown project id (which
// always carries the {"error": {...}} envelope).
func missingRoute(statusCode int, body []byte) bool {
	if statusCode != http.StatusNotFound {
		return false
	}
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return true
	}
	return len(envelope.Error) == 0 || envelope.Error[0] != '{'
}
