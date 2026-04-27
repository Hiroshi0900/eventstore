package dynamodb_test

import (
	"context"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	ddb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	es "github.com/Hiroshi0900/eventstore/v2"
	v2ddb "github.com/Hiroshi0900/eventstore/v2/dynamodb"
)

// fakeClient records the last received PutItem / TransactWriteItems call and
// returns canned responses for GetItem / Query.
type fakeClient struct {
	lastPut   *ddb.PutItemInput
	lastTrans *ddb.TransactWriteItemsInput
	getResp   *ddb.GetItemOutput
	queryResp *ddb.QueryOutput
	putErr    error
	transErr  error
}

func (f *fakeClient) GetItem(_ context.Context, _ *ddb.GetItemInput, _ ...func(*ddb.Options)) (*ddb.GetItemOutput, error) {
	if f.getResp == nil {
		return &ddb.GetItemOutput{}, nil
	}
	return f.getResp, nil
}

func (f *fakeClient) Query(_ context.Context, _ *ddb.QueryInput, _ ...func(*ddb.Options)) (*ddb.QueryOutput, error) {
	if f.queryResp == nil {
		return &ddb.QueryOutput{}, nil
	}
	return f.queryResp, nil
}

func (f *fakeClient) PutItem(_ context.Context, in *ddb.PutItemInput, _ ...func(*ddb.Options)) (*ddb.PutItemOutput, error) {
	f.lastPut = in
	return &ddb.PutItemOutput{}, f.putErr
}

func (f *fakeClient) TransactWriteItems(_ context.Context, in *ddb.TransactWriteItemsInput, _ ...func(*ddb.Options)) (*ddb.TransactWriteItemsOutput, error) {
	f.lastTrans = in
	return &ddb.TransactWriteItemsOutput{}, f.transErr
}

func TestStore_PersistEvent_writesPutItem(t *testing.T) {
	c := &fakeClient{}
	store := v2ddb.New(c, v2ddb.DefaultConfig())
	id := es.NewAggregateID("Visit", "x")

	ev := &es.EventEnvelope{
		EventID:       "ev-1",
		EventTypeName: "Test",
		AggregateID:   id,
		SeqNr:         1,
		IsCreated:     true,
		OccurredAt:    time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC),
		Payload:       []byte(`{"k":"v"}`),
	}

	if err := store.PersistEvent(context.Background(), ev, 0); err != nil {
		t.Fatalf("PersistEvent: %v", err)
	}
	if c.lastPut == nil {
		t.Fatalf("PutItem not called")
	}
	if got := awssdk.ToString(c.lastPut.TableName); got != "journal" {
		t.Errorf("TableName: got %q, want %q", got, "journal")
	}
	if _, ok := c.lastPut.Item["pkey"].(*types.AttributeValueMemberS); !ok {
		t.Errorf("pkey attribute missing or wrong type")
	}
	if _, ok := c.lastPut.Item["skey"].(*types.AttributeValueMemberS); !ok {
		t.Errorf("skey attribute missing or wrong type")
	}
	if v, ok := c.lastPut.Item["seq_nr"].(*types.AttributeValueMemberN); !ok || v.Value != "1" {
		t.Errorf("seq_nr attribute mismatch: %+v", c.lastPut.Item["seq_nr"])
	}
}

func TestStore_PersistEventAndSnapshot_initialWriteUsesAttributeNotExists(t *testing.T) {
	c := &fakeClient{}
	store := v2ddb.New(c, v2ddb.DefaultConfig())
	id := es.NewAggregateID("Visit", "x")
	ev := &es.EventEnvelope{
		EventID:       "ev-2",
		EventTypeName: "Test",
		AggregateID:   id,
		SeqNr:         5,
		IsCreated:     false,
		OccurredAt:    time.Now().UTC(),
		Payload:       []byte(`{}`),
	}
	snap := &es.SnapshotEnvelope{
		AggregateID: id,
		SeqNr:       5,
		Version:     1,
		Payload:     []byte(`{"state":"x"}`),
		OccurredAt:  time.Now().UTC(),
	}

	if err := store.PersistEventAndSnapshot(context.Background(), ev, snap); err != nil {
		t.Fatalf("PersistEventAndSnapshot: %v", err)
	}
	if c.lastTrans == nil {
		t.Fatalf("TransactWriteItems not called")
	}
	if len(c.lastTrans.TransactItems) != 2 {
		t.Errorf("TransactItems count: got %d, want 2", len(c.lastTrans.TransactItems))
	}
	snapPut := c.lastTrans.TransactItems[1].Put
	if snapPut == nil {
		t.Fatalf("snapshot put is nil")
	}
	if got := awssdk.ToString(snapPut.ConditionExpression); got != "attribute_not_exists(#version)" {
		t.Errorf("initial-write ConditionExpression: got %q, want %q",
			got, "attribute_not_exists(#version)")
	}
}

func TestStore_PersistEventAndSnapshot_subsequentWriteUsesVersionCondition(t *testing.T) {
	c := &fakeClient{}
	store := v2ddb.New(c, v2ddb.DefaultConfig())
	id := es.NewAggregateID("Visit", "x")
	ev := &es.EventEnvelope{
		EventID:     "ev-3",
		AggregateID: id,
		SeqNr:       10,
		OccurredAt:  time.Now().UTC(),
		Payload:     []byte(`{}`),
	}
	snap := &es.SnapshotEnvelope{
		AggregateID: id,
		SeqNr:       10,
		Version:     3, // expected = 2
		Payload:     []byte(`{}`),
		OccurredAt:  time.Now().UTC(),
	}

	if err := store.PersistEventAndSnapshot(context.Background(), ev, snap); err != nil {
		t.Fatalf("PersistEventAndSnapshot: %v", err)
	}
	snapPut := c.lastTrans.TransactItems[1].Put
	got := awssdk.ToString(snapPut.ConditionExpression)
	want := "attribute_not_exists(#version) OR #version = :expected"
	if got != want {
		t.Errorf("subsequent-write ConditionExpression: got %q, want %q", got, want)
	}
	expected, ok := snapPut.ExpressionAttributeValues[":expected"].(*types.AttributeValueMemberN)
	if !ok || expected.Value != "2" {
		t.Errorf(":expected value: got %+v, want N=2", snapPut.ExpressionAttributeValues[":expected"])
	}
}

func TestStore_GetEventsSince_unmarshalsCorrectly(t *testing.T) {
	c := &fakeClient{
		queryResp: &ddb.QueryOutput{
			Items: []map[string]types.AttributeValue{
				{
					"pkey":        &types.AttributeValueMemberS{Value: "Visit-0"},
					"skey":        &types.AttributeValueMemberS{Value: "Visit-x-00000000000000000001"},
					"aid":         &types.AttributeValueMemberS{Value: "Visit-x"},
					"seq_nr":      &types.AttributeValueMemberN{Value: "1"},
					"payload":     &types.AttributeValueMemberB{Value: []byte(`{"k":"v"}`)},
					"occurred_at": &types.AttributeValueMemberN{Value: "1700000000000"},
					"type_name":   &types.AttributeValueMemberS{Value: "VisitScheduled"},
					"event_id":    &types.AttributeValueMemberS{Value: "ev-1"},
					"is_created":  &types.AttributeValueMemberBOOL{Value: true},
				},
			},
		},
	}
	store := v2ddb.New(c, v2ddb.DefaultConfig())
	id := es.NewAggregateID("Visit", "x")

	envs, err := store.GetEventsSince(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("GetEventsSince: %v", err)
	}
	if len(envs) != 1 {
		t.Fatalf("len: got %d, want 1", len(envs))
	}
	got := envs[0]
	if got.SeqNr != 1 || got.EventID != "ev-1" || got.EventTypeName != "VisitScheduled" || !got.IsCreated {
		t.Errorf("envelope mismatch: %+v", got)
	}
	if got.AggregateID.AsString() != "Visit-x" {
		t.Errorf("AggregateID: got %q, want Visit-x", got.AggregateID.AsString())
	}
	if string(got.Payload) != `{"k":"v"}` {
		t.Errorf("Payload: got %q, want %q", got.Payload, `{"k":"v"}`)
	}
}
