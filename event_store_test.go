package eventstore

import "testing"

func TestDefaultConfig_Defaults(t *testing.T) {
	c := DefaultConfig()
	if got, want := c.SnapshotInterval, uint64(5); got != want {
		t.Errorf("SnapshotInterval = %d, want %d", got, want)
	}
}

func TestConfig_ShouldSnapshot(t *testing.T) {
	cases := []struct {
		interval uint64
		seqNr    uint64
		want     bool
	}{
		{interval: 5, seqNr: 0, want: true},  // 0 % 5 == 0
		{interval: 5, seqNr: 1, want: false},
		{interval: 5, seqNr: 5, want: true},
		{interval: 5, seqNr: 10, want: true},
		{interval: 0, seqNr: 5, want: false}, // disabled
		{interval: 1, seqNr: 1, want: true},
	}
	for _, tc := range cases {
		c := Config{SnapshotInterval: tc.interval}
		if got := c.ShouldSnapshot(tc.seqNr); got != tc.want {
			t.Errorf("ShouldSnapshot(interval=%d, seq=%d) = %v, want %v",
				tc.interval, tc.seqNr, got, tc.want)
		}
	}
}
