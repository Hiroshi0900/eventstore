package eventsourcing_test

import (
	"testing"

	es "github.com/Hiroshi0900/eventstore/v2"
)

func TestDefaultConfig(t *testing.T) {
	c := es.DefaultConfig()
	if c.SnapshotInterval != 5 {
		t.Errorf("SnapshotInterval: got %d, want 5", c.SnapshotInterval)
	}
	if c.JournalTableName != "journal" {
		t.Errorf("JournalTableName: got %q, want %q", c.JournalTableName, "journal")
	}
	if c.SnapshotTableName != "snapshot" {
		t.Errorf("SnapshotTableName: got %q, want %q", c.SnapshotTableName, "snapshot")
	}
	if c.ShardCount != 1 {
		t.Errorf("ShardCount: got %d, want 1", c.ShardCount)
	}
}

func TestConfig_ShouldSnapshot(t *testing.T) {
	tests := []struct {
		name             string
		snapshotInterval uint64
		seqNr            uint64
		want             bool
	}{
		{"interval=0 disables snapshots", 0, 5, false},
		{"seqNr 1 with interval 5", 5, 1, false},
		{"seqNr 5 with interval 5", 5, 5, true},
		{"seqNr 6 with interval 5", 5, 6, false},
		{"seqNr 10 with interval 5", 5, 10, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := es.Config{SnapshotInterval: tt.snapshotInterval}
			if got := c.ShouldSnapshot(tt.seqNr); got != tt.want {
				t.Errorf("ShouldSnapshot(%d) with interval=%d: got %v, want %v",
					tt.seqNr, tt.snapshotInterval, got, tt.want)
			}
		})
	}
}
