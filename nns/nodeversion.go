package nns

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fxamacker/cbor/v2"
)

// statusResponse is the subset of /api/v2/status we read. The endpoint also
// carries root_key, impl_hash, replica_health_status and certified_height; only
// impl_version is needed here. Pointer so an absent field is distinguishable
// from an empty one.
type statusResponse struct {
	ImplVersion *string `cbor:"impl_version"`
}

// FetchNodeVersion reads the GuestOS/replica version a node reports running from
// its own /api/v2/status endpoint, as impl_version.
//
// This is deliberately not a registry read: the registry stores a replica version
// per subnet, not per node, so the only source for what a specific node is
// actually running is the node itself. Note that boundary nodes blank the field
// (ic_boundary serves impl_version: None), so base must address a replica
// directly, not a public gateway.
//
// Reaching a replica requires IPv6; on a v4-only host every call fails to dial.
func FetchNodeVersion(base string) (string, error) {
	u := strings.TrimRight(base, "/") + "/api/v2/status"
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return "", fmt.Errorf("get status: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status: unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read status: %w", err)
	}
	var st statusResponse
	if err := cbor.Unmarshal(body, &st); err != nil {
		return "", fmt.Errorf("decode status: %w", err)
	}
	if st.ImplVersion == nil || *st.ImplVersion == "" {
		return "", fmt.Errorf("status carries no impl_version (a boundary node rather than a replica?)")
	}
	return *st.ImplVersion, nil
}

// NodeStatusURL builds the status base URL for a node's registry http endpoint.
// Registry endpoints carry a bare IPv6 address, which must be bracketed.
func NodeStatusURL(ipAddr string, port uint32) string {
	if port == 0 {
		port = 8080
	}
	return fmt.Sprintf("https://%s", net.JoinHostPort(ipAddr, fmt.Sprint(port)))
}

// DefaultDashboardAPI is the public dashboard API used as a fallback when a node
// cannot be reached directly.
const DefaultDashboardAPI = "https://ic-api.internetcomputer.org"

type dashboardNode struct {
	GuestosVersion string `json:"guestos_version"`
}

// FetchNodeVersionFromDashboard reads a node's GuestOS version from the public
// dashboard API, which is reachable over IPv4.
//
// This is a fallback for when the node itself cannot be reached (see
// FetchNodeVersion): the dashboard polls nodes on its own schedule, so the value
// reflects whenever it last succeeded, not the node's state right now. It is also
// a trusted third party rather than a direct read, the same caveat
// FetchNodeStatus carries for the registry explorer.
func FetchNodeVersionFromDashboard(base, nodeID string) (string, error) {
	u := fmt.Sprintf("%s/api/v3/nodes/%s", strings.TrimRight(base, "/"), url.PathEscape(nodeID))
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return "", fmt.Errorf("get dashboard node: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("dashboard node %s: unexpected status %d", nodeID, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read dashboard node %s: %w", nodeID, err)
	}
	var dn dashboardNode
	if err := json.Unmarshal(body, &dn); err != nil {
		return "", fmt.Errorf("decode dashboard node %s: %w", nodeID, err)
	}
	if dn.GuestosVersion == "" {
		return "", fmt.Errorf("dashboard node %s carries no guestos_version", nodeID)
	}
	return dn.GuestosVersion, nil
}
