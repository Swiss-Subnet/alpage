// Package pocketic is a thin REST client for the PocketIC server (v13.x, IC
// release-2026-04-10). It talks the server-native /instances/.../{read,update}
// endpoints: plain JSON, standard-base64 raw-byte fields, no CBOR, no signing.
package pocketic

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const serverMajorVersion = 15

// b64 is STANDARD base64 (base64::encode in the server's rest.rs uses the
// v0.13 top-level API, i.e. the STANDARD alphabet with padding), NOT url-safe.
var b64 = base64.StdEncoding

type Client struct {
	base   string // http://127.0.0.1:<port>
	http   *http.Client
	poll   time.Duration
	server *serverProc
}

// apiResponse models the server's untagged ApiResponse<T>: a 200 carries T
// directly; a 202 carries {state_label, op_id} (Started); a 409 the same
// (Busy). We branch on HTTP status rather than shape.
type startedOrBusy struct {
	StateLabel string `json:"state_label"`
	OpID       string `json:"op_id"`
}

// do issues a POST (or GET when body is nil) and resolves the async protocol:
// 200 -> raw body; 202 -> poll read_graph; 409 -> retry; else error.
func (c *Client) do(method, path string, body any) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		raw, status, err := c.roundtrip(method, path, body)
		if err != nil {
			return nil, err
		}
		switch status {
		case http.StatusOK, http.StatusCreated:
			return raw, nil
		case http.StatusAccepted:
			var sb startedOrBusy
			if err := json.Unmarshal(raw, &sb); err != nil {
				return nil, fmt.Errorf("decode Started: %w", err)
			}
			return c.pollGraph(sb)
		case http.StatusConflict:
			// Busy: instance occupied by another computation; retry original.
			time.Sleep(c.poll)
			continue
		default:
			return nil, fmt.Errorf("%s %s: http %d: %s", method, path, status, string(raw))
		}
	}
}

func (c *Client) roundtrip(method, path string, body any) ([]byte, int, error) {
	var r io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		r = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, c.base+path, r)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	return raw, resp.StatusCode, err
}

// pollGraph polls GET /read_graph/{state_label}/{op_id} on the server root
// until it stops 404-ing, then returns the resolved body.
func (c *Client) pollGraph(sb startedOrBusy) ([]byte, error) {
	path := fmt.Sprintf("/read_graph/%s/%s", sb.StateLabel, sb.OpID)
	for {
		raw, status, err := c.roundtrip(http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}
		switch status {
		case http.StatusOK, http.StatusAccepted:
			// best-effort prune, ignore result
			_, _, _ = c.roundtrip(http.MethodDelete, fmt.Sprintf("/prune_graph/%s/%s", sb.StateLabel, sb.OpID), nil)
			return raw, nil
		case http.StatusNotFound:
			time.Sleep(c.poll)
			continue
		default:
			return nil, fmt.Errorf("read_graph: http %d: %s", status, string(raw))
		}
	}
}
