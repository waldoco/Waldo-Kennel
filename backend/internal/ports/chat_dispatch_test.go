package ports

import "testing"

func TestChatTurnDispatchValidate(t *testing.T) {
	written := ChatTurnDispatch{TransportRequestID: 7, TransportSHA256: "abc", TransportBytes: 42, TransportSequence: 3}
	cases := []struct {
		name  string
		value ChatTurnDispatch
		valid bool
	}{
		{"not sent", ChatTurnDispatch{Acceptance: ChatTurnNotSent}, true},
		{"not sent with write", func() ChatTurnDispatch { d := written; d.Acceptance = ChatTurnNotSent; return d }(), false},
		{"rejected", func() ChatTurnDispatch { d := written; d.Acceptance = ChatTurnRejected; return d }(), true},
		{"unknown", func() ChatTurnDispatch { d := written; d.Acceptance = ChatTurnDeliveryUnknown; return d }(), true},
		{"unknown with provider id", func() ChatTurnDispatch {
			d := written
			d.Acceptance = ChatTurnDeliveryUnknown
			d.Ref.ProviderTurnID = "turn"
			return d
		}(), false},
		{"acknowledged", func() ChatTurnDispatch {
			d := written
			d.Acceptance = ChatTurnAcknowledged
			d.Ref.ProviderTurnID = "turn"
			return d
		}(), true},
		{"acknowledged missing provider id", func() ChatTurnDispatch { d := written; d.Acceptance = ChatTurnAcknowledged; return d }(), false},
		{"zero acceptance", ChatTurnDispatch{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.value.Validate()
			if (err == nil) != tc.valid {
				t.Fatalf("Validate()=%v valid=%v value=%+v", err, tc.valid, tc.value)
			}
		})
	}
}
