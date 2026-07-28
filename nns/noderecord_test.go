package nns

import "testing"

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
