package httpclient

import "strings"

func (c Client) baseURL() string {
	return strings.TrimRight(c.cfg.BaseURL, "/")
}
