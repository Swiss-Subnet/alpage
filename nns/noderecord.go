package nns

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// nodeRecord is one versioned entry of a node_record_<id> key, as served by the
// registry explorer's /api/records/nodes/<id> endpoint. A nil Value is a
// deletion mutation: the node was deregistered at that version.
type nodeRecord struct {
	Key     string           `json:"key"`
	Version uint64           `json:"version"`
	Value   *nodeRecordValue `json:"value"`
}

type nodeRecordValue struct {
	NodeOperatorID string `json:"node_operator_id"`
}

// nodeRegistration reduces a node's record history to its current state: the
// node is registered iff the highest-version entry carries a value; that
// entry's operator id is returned. Pure; the caller supplies the records.
func nodeRegistration(records []nodeRecord) (registered bool, operatorID string) {
	var latest *nodeRecord
	for i := range records {
		if latest == nil || records[i].Version > latest.Version {
			latest = &records[i]
		}
	}
	if latest == nil || latest.Value == nil {
		return false, ""
	}
	return true, latest.Value.NodeOperatorID
}

// NodeStatus is a node's current registry state.
type NodeStatus struct {
	Registered bool
	OperatorID string
}

// FetchNodeStatus reads a node's record history from the registry explorer HTTP
// API and reduces it to its current registration state.
//
// TODO: this trusts an HTTP service in front of the registry. Replace with a
// direct, trustless read of the registry canister KV store (get_changes_since;
// node_record_<id> present vs deletion mutation), the way nodao/regexp does it
// in-process. Kept as HTTP for now because that read path is not yet vendored
// into alpage.
func FetchNodeStatus(explorerBase, nodeID string) (NodeStatus, error) {
	base := strings.TrimRight(explorerBase, "/")
	u := fmt.Sprintf("%s/api/records/nodes/%s", base, url.PathEscape(nodeID))
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return NodeStatus{}, fmt.Errorf("get node record %s: %w", nodeID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return NodeStatus{Registered: false}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return NodeStatus{}, fmt.Errorf("node record %s: unexpected status %d", nodeID, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return NodeStatus{}, fmt.Errorf("read node record %s: %w", nodeID, err)
	}
	var records []nodeRecord
	if err := json.Unmarshal(body, &records); err != nil {
		return NodeStatus{}, fmt.Errorf("decode node record %s: %w", nodeID, err)
	}
	reg, op := nodeRegistration(records)
	return NodeStatus{Registered: reg, OperatorID: op}, nil
}

// FetchOperatorNodes returns the node ids the registry records as owned by an
// operator, via the explorer's /api/records/node-operators/<id>/nodes index. An
// unknown operator (404) yields an empty slice. Same HTTP-trust caveat as
// FetchNodeStatus: there is no typed canister query for operator->nodes, so
// enumeration goes through the explorer until the get_changes_since read path
// is vendored.
func FetchOperatorNodes(explorerBase, operatorID string) ([]string, error) {
	base := strings.TrimRight(explorerBase, "/")
	u := fmt.Sprintf("%s/api/records/node-operators/%s/nodes", base, url.PathEscape(operatorID))
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return nil, fmt.Errorf("get operator nodes %s: %w", operatorID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("operator nodes %s: unexpected status %d", operatorID, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read operator nodes %s: %w", operatorID, err)
	}
	var ids []string
	if err := json.Unmarshal(body, &ids); err != nil {
		return nil, fmt.Errorf("decode operator nodes %s: %w", operatorID, err)
	}
	return ids, nil
}

// DefaultRegistryExplorer is the registry explorer this build queries for
// node-record history. See FetchNodeStatus's TODO on trust.
const DefaultRegistryExplorer = "https://registry.aviatelabs.co"
