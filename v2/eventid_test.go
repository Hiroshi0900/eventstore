package eventstore

import (
	"regexp"
	"testing"
)

func TestGenerateEventID_format(t *testing.T) {
	id, err := generateEventID()
	if err != nil {
		t.Fatalf("generateEventID(): %v", err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(id) {
		t.Errorf("id format unexpected: %q", id)
	}
}

func TestGenerateEventID_unique(t *testing.T) {
	const n = 1000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id, err := generateEventID()
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id at iteration %d: %s", i, id)
		}
		seen[id] = struct{}{}
	}
}
