package nns

import (
	"encoding/json"
	"testing"
)

func TestNodeRegistrationFromRecords(t *testing.T) {
	op := "hsi7b-rl4wt-lum3m-ophfi-oxgx5-u2q7r-ak7ag-nyiik-c4gam-epo2r-3qe"
	tests := []struct {
		name    string
		records []nodeRecord // newest-first, as the endpoint returns them
		wantReg bool
		wantOp  string
	}{
		{"empty is deregistered", nil, false, ""},
		{
			"latest null value is deregistered",
			[]nodeRecord{
				{Version: 62340, Value: nil},
				{Version: 57928, Value: &nodeRecordValue{NodeOperatorID: op}},
			},
			false, "",
		},
		{
			"latest with value is registered",
			[]nodeRecord{
				{Version: 57879, Value: &nodeRecordValue{NodeOperatorID: op}},
				{Version: 57340, Value: &nodeRecordValue{NodeOperatorID: op}},
			},
			true, op,
		},
		{
			"out-of-order records still pick highest version",
			[]nodeRecord{
				{Version: 100, Value: &nodeRecordValue{NodeOperatorID: op}},
				{Version: 200, Value: nil},
			},
			false, "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st, _ := nodeRegistration(tc.records)
			if st.Registered != tc.wantReg {
				t.Errorf("registered = %v, want %v", st.Registered, tc.wantReg)
			}
			if st.OperatorID != tc.wantOp {
				t.Errorf("operator = %q, want %q", st.OperatorID, tc.wantOp)
			}
		})
	}
}

func TestNodeRegistrationChipID(t *testing.T) {
	op := "hsi7b-rl4wt-lum3m-ophfi-oxgx5-u2q7r-ak7ag-nyiik-c4gam-epo2r-3qe"
	chip := "MVaYAK9/jdl0Hliy+H5txLjkTx9zWctFquxDm3pRxGB0FfmHoomyIZ3F9yGL5a3kDjUD3qoAgbUca0Q8ZIdX4w=="
	other := "tJo5fS/HXEl4UYEjhqAeHWpbj+rVVR3+jAYTDWLeoOeUzl7Jp6XOtQ03EwUX6C4QPEeDmSJG3hqHsQZOlOgZ7g=="
	tests := []struct {
		name    string
		records []nodeRecord
		want    string
	}{
		{
			"chip_id is carried through verbatim",
			[]nodeRecord{{Version: 57880, Value: &nodeRecordValue{NodeOperatorID: op, ChipID: chip}}},
			chip,
		},
		{
			"absent chip_id is empty",
			[]nodeRecord{{Version: 57848, Value: &nodeRecordValue{NodeOperatorID: op}}},
			"",
		},
		{
			"only the latest record decides",
			[]nodeRecord{
				{Version: 100, Value: &nodeRecordValue{NodeOperatorID: op, ChipID: chip}},
				{Version: 200, Value: &nodeRecordValue{NodeOperatorID: op, ChipID: other}},
			},
			other,
		},
		{
			"deregistered node carries no chip",
			[]nodeRecord{{Version: 200, Value: nil}},
			"",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st, _ := nodeRegistration(tc.records)
			if st.ChipID != tc.want {
				t.Errorf("chip id = %q, want %q", st.ChipID, tc.want)
			}
		})
	}
}

// The explorer emits chip_id as a base64 string alongside the other node-record
// fields; a node without SEV omits the key entirely.
func TestNodeRecordDecodesChipID(t *testing.T) {
	body := `[{"key":"node_record_x","version":57880,"value":{"http":{"ip_addr":"2001:db8::1","port":8080},"chip_id":"MVaYAK9/jdl0Hliy+H5txLjkTx9zWctFquxDm3pRxGB0FfmHoomyIZ3F9yGL5a3kDjUD3qoAgbUca0Q8ZIdX4w==","node_operator_id":"hoqyg-qkf6b-ulmyi-zfk6z-fuvlo-pekr6-goigz-qweij-xhghd-vibjl-jqe"}},
	           {"key":"node_record_x","version":57341,"value":{"http":{"ip_addr":"2001:db8::1","port":8080},"node_operator_id":"hoqyg-qkf6b-ulmyi-zfk6z-fuvlo-pekr6-goigz-qweij-xhghd-vibjl-jqe"}}]`
	var records []nodeRecord
	if err := json.Unmarshal([]byte(body), &records); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if records[0].Value.ChipID == "" {
		t.Error("chip_id did not decode onto the newest record")
	}
	if records[1].Value.ChipID != "" {
		t.Error("record without chip_id decoded a non-empty ChipID")
	}
	st, _ := nodeRegistration(records)
	if st.ChipID != records[0].Value.ChipID {
		t.Errorf("chip id = %q, want the newest record's", st.ChipID)
	}
}
