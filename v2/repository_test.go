package eventstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	es "github.com/Hiroshi0900/eventstore/v2"
	"github.com/Hiroshi0900/eventstore/v2/memory"
)

// --- テスト用ミニ集約: 単純なカウンタ ---

type counter struct {
	id      es.AggregateID
	value   int
	seqNr   uint64
	version uint64
}

func newCounter(id es.AggregateID) counter { return counter{id: id} }

func (c counter) AggregateID() es.AggregateID       { return c.id }
func (c counter) SeqNr() uint64                     { return c.seqNr }
func (c counter) Version() uint64                   { return c.version }
func (c counter) WithVersion(v uint64) es.Aggregate { c.version = v; return c }
func (c counter) WithSeqNr(s uint64) es.Aggregate   { c.seqNr = s; return c }

func (c counter) ApplyCommand(cmd es.Command) (es.Event, error) {
	switch cmd.(type) {
	case incrementCmd:
		return es.NewEvent("evt-"+cmd.AggregateID().AsString(), "Incremented", cmd.AggregateID(), nil,
			es.WithSeqNr(c.seqNr+1), es.WithIsCreated(c.seqNr == 0)), nil
	default:
		return nil, es.ErrUnknownCommand
	}
}

func (c counter) ApplyEvent(ev es.Event) es.Aggregate {
	switch ev.EventTypeName() {
	case "Incremented":
		c.value++
		c.seqNr = ev.SeqNr()
	}
	return c
}

type incrementCmd struct{ id es.AggregateID }

func (c incrementCmd) CommandTypeName() string     { return "Increment" }
func (c incrementCmd) AggregateID() es.AggregateID { return c.id }

// --- AggregateSerializer ---

type counterSnapshot struct {
	Value   int    `json:"value"`
	SeqNr   uint64 `json:"seq_nr"`
	Version uint64 `json:"version"`
	IDType  string `json:"id_type"`
	IDValue string `json:"id_value"`
}

type counterSerializer struct{}

func (counterSerializer) Serialize(c counter) ([]byte, error) {
	return json.Marshal(counterSnapshot{
		Value: c.value, SeqNr: c.seqNr, Version: c.version,
		IDType: c.id.TypeName(), IDValue: c.id.Value(),
	})
}

func (counterSerializer) Deserialize(data []byte) (counter, error) {
	var s counterSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return counter{}, err
	}
	return counter{
		id:      es.NewAggregateID(s.IDType, s.IDValue),
		value:   s.Value,
		seqNr:   s.SeqNr,
		version: s.Version,
	}, nil
}

// --- 共通ヘルパ ---

func newTestRepo() *es.Repository[counter] {
	return es.NewRepository[counter](
		memory.New(),
		func(id es.AggregateID) counter { return newCounter(id) },
		counterSerializer{},
		es.DefaultConfig(),
	)
}

func TestRepository_Construct(t *testing.T) {
	r := newTestRepo()
	if r == nil {
		t.Fatal("nil repo")
	}
	_ = context.Background
	_ = errors.Is
}
