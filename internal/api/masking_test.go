package api

import "testing"

func TestMaskPhone(t *testing.T) {
	cases := []struct {
		in   *string
		want *string
	}{
		{nil, nil},
		{strp("+15551234567"), strp("***-***-4567")},
		{strp("123"), strp("***")},
	}
	for _, tc := range cases {
		got := maskPhone(tc.in)
		switch {
		case tc.want == nil && got != nil:
			t.Errorf("maskPhone(%v) = %v, want nil", tc.in, *got)
		case tc.want != nil && got == nil:
			t.Errorf("maskPhone(%v) = nil, want %v", *tc.in, *tc.want)
		case tc.want != nil && got != nil && *got != *tc.want:
			t.Errorf("maskPhone(%v) = %v, want %v", *tc.in, *got, *tc.want)
		}
	}
}
