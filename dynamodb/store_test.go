package dynamodb_test

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsdynamo "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	awsdynamotypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	es "github.com/Hiroshi0900/eventstore"
	v2dynamo "github.com/Hiroshi0900/eventstore/dynamodb"
)

// fakeClient は dynamodb.Client interface を満たす最小実装。
type fakeClient struct{}

func (fakeClient) GetItem(ctx context.Context, params *awsdynamo.GetItemInput, optFns ...func(*awsdynamo.Options)) (*awsdynamo.GetItemOutput, error) {
	return &awsdynamo.GetItemOutput{}, nil
}

func (fakeClient) Query(ctx context.Context, params *awsdynamo.QueryInput, optFns ...func(*awsdynamo.Options)) (*awsdynamo.QueryOutput, error) {
	return &awsdynamo.QueryOutput{}, nil
}

func (fakeClient) PutItem(ctx context.Context, params *awsdynamo.PutItemInput, optFns ...func(*awsdynamo.Options)) (*awsdynamo.PutItemOutput, error) {
	return &awsdynamo.PutItemOutput{}, nil
}

func (fakeClient) TransactWriteItems(ctx context.Context, params *awsdynamo.TransactWriteItemsInput, optFns ...func(*awsdynamo.Options)) (*awsdynamo.TransactWriteItemsOutput, error) {
	return &awsdynamo.TransactWriteItemsOutput{}, nil
}

func (fakeClient) CreateTable(ctx context.Context, params *awsdynamo.CreateTableInput, optFns ...func(*awsdynamo.Options)) (*awsdynamo.CreateTableOutput, error) {
	return &awsdynamo.CreateTableOutput{}, nil
}

func (fakeClient) DescribeTable(ctx context.Context, params *awsdynamo.DescribeTableInput, optFns ...func(*awsdynamo.Options)) (*awsdynamo.DescribeTableOutput, error) {
	return &awsdynamo.DescribeTableOutput{}, nil
}

type capturingClient struct {
	fakeClient
	lastQuery *awsdynamo.QueryInput
}

func (c *capturingClient) Query(ctx context.Context, params *awsdynamo.QueryInput, optFns ...func(*awsdynamo.Options)) (*awsdynamo.QueryOutput, error) {
	c.lastQuery = params
	return &awsdynamo.QueryOutput{}, nil
}

type paginatedClient struct {
	fakeClient
	queryInputs []*awsdynamo.QueryInput
	queryPages  []*awsdynamo.QueryOutput
}

func (c *paginatedClient) Query(ctx context.Context, params *awsdynamo.QueryInput, optFns ...func(*awsdynamo.Options)) (*awsdynamo.QueryOutput, error) {
	copied := *params
	c.queryInputs = append(c.queryInputs, &copied)
	if len(c.queryPages) == 0 {
		return &awsdynamo.QueryOutput{}, nil
	}
	page := c.queryPages[0]
	c.queryPages = c.queryPages[1:]
	return page, nil
}

// stub types for generic instantiation.
type ddbTestAggID struct{ value string }

func (a ddbTestAggID) TypeName() string { return "T" }
func (a ddbTestAggID) Value() string    { return a.value }
func (a ddbTestAggID) AsString() string { return "T-" + a.value }

type ddbStubEvent struct{ aggID ddbTestAggID }

func (e ddbStubEvent) EventTypeName() string       { return "Stub" }
func (e ddbStubEvent) AggregateID() es.AggregateID { return e.aggID }

type ddbStubCommand interface {
	es.Command
	isDDBStubCommand()
}

type ddbCreateStubCommand struct{}

func (ddbCreateStubCommand) CommandTypeName() string { return "CreateStub" }
func (ddbCreateStubCommand) isDDBStubCommand()       {}

type ddbStubAggregate struct{ id ddbTestAggID }

func (a ddbStubAggregate) AggregateID() es.AggregateID { return a.id }
func (a ddbStubAggregate) ApplyCommand(ddbStubCommand) (ddbStubEvent, error) {
	return ddbStubEvent{}, es.ErrUnknownCommand
}
func (a ddbStubAggregate) ApplyEvent(ddbStubEvent) es.Aggregate[ddbStubCommand, ddbStubEvent] {
	return a
}

type ddbStubAggSer struct{}

func (ddbStubAggSer) Serialize(ddbStubAggregate) ([]byte, error)   { return nil, nil }
func (ddbStubAggSer) Deserialize([]byte) (ddbStubAggregate, error) { return ddbStubAggregate{}, nil }

type ddbStubEvSer struct{}

func (ddbStubEvSer) Serialize(ddbStubEvent) ([]byte, error)               { return nil, nil }
func (ddbStubEvSer) Deserialize(string, []byte) (ddbStubEvent, error)     { return ddbStubEvent{}, nil }

func TestNew_Construct(t *testing.T) {
	store := v2dynamo.New[ddbStubAggregate, ddbStubCommand, ddbStubEvent](
		fakeClient{},
		v2dynamo.DefaultConfig(),
		ddbStubAggSer{},
		ddbStubEvSer{},
	)
	if store == nil {
		t.Fatal("expected non-nil EventStore")
	}
}

func TestNewWithTables_Construct(t *testing.T) {
	mgr := v2dynamo.NewWithTables[ddbStubAggregate, ddbStubCommand, ddbStubEvent](
		fakeClient{},
		v2dynamo.DefaultConfig(),
		ddbStubAggSer{},
		ddbStubEvSer{},
	)
	if mgr == nil {
		t.Fatal("expected non-nil TableManager")
	}
}

func TestLoadStreamAfter_UsesPrimaryTableConsistentRead(t *testing.T) {
	client := &capturingClient{}
	store := v2dynamo.New[ddbStubAggregate, ddbStubCommand, ddbStubEvent](
		client,
		v2dynamo.DefaultConfig(),
		ddbStubAggSer{},
		ddbStubEvSer{},
	)

	_, err := store.LoadStreamAfter(context.Background(), ddbTestAggID{value: "1"}, 0)
	if err != nil {
		t.Fatalf("LoadStreamAfter: %v", err)
	}
	if client.lastQuery == nil {
		t.Fatal("expected Query call")
	}
	if client.lastQuery.IndexName != nil {
		t.Fatalf("IndexName = %q, want nil", aws.ToString(client.lastQuery.IndexName))
	}
	if client.lastQuery.ConsistentRead == nil || !aws.ToBool(client.lastQuery.ConsistentRead) {
		t.Fatal("ConsistentRead = false, want true")
	}
	if got := aws.ToString(client.lastQuery.KeyConditionExpression); got != "#pk = :pk AND #sk > :sk" {
		t.Fatalf("KeyConditionExpression = %q, want primary-table lower-bound query", got)
	}
	if got := aws.ToString(client.lastQuery.FilterExpression); got != "#aid = :aid" {
		t.Fatalf("FilterExpression = %q, want exact aggregate filter", got)
	}
	from, ok := client.lastQuery.ExpressionAttributeValues[":sk"]
	if !ok {
		t.Fatal("expected :sk attribute value")
	}
	fromS, ok := from.(*awsdynamotypes.AttributeValueMemberS)
	if !ok {
		t.Fatalf(":sk type = %T, want string", from)
	}
	if fromS.Value != "T-1-00000000000000000000" {
		t.Fatalf(":sk = %q, want %q", fromS.Value, "T-1-00000000000000000000")
	}
	aid, ok := client.lastQuery.ExpressionAttributeValues[":aid"]
	if !ok {
		t.Fatal("expected :aid attribute value")
	}
	aidS, ok := aid.(*awsdynamotypes.AttributeValueMemberS)
	if !ok {
		t.Fatalf(":aid type = %T, want string", aid)
	}
	if aidS.Value != "T-1" {
		t.Fatalf(":aid = %q, want %q", aidS.Value, "T-1")
	}
}

func TestLoadStreamAfter_UsesSeqNrAsLowerBound(t *testing.T) {
	tests := []struct {
		name     string
		seqNr    uint64
		wantSk   string
	}{
		{name: "seq0", seqNr: 0, wantSk: "T-1-00000000000000000000"},
		{name: "seq1", seqNr: 1, wantSk: "T-1-00000000000000000001"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &capturingClient{}
			store := v2dynamo.New[ddbStubAggregate, ddbStubCommand, ddbStubEvent](
				client,
				v2dynamo.DefaultConfig(),
				ddbStubAggSer{},
				ddbStubEvSer{},
			)

			_, err := store.LoadStreamAfter(context.Background(), ddbTestAggID{value: "1"}, tc.seqNr)
			if err != nil {
				t.Fatalf("LoadStreamAfter: %v", err)
			}
			from, ok := client.lastQuery.ExpressionAttributeValues[":sk"]
			if !ok {
				t.Fatal("expected :sk attribute value")
			}
			fromS, ok := from.(*awsdynamotypes.AttributeValueMemberS)
			if !ok {
				t.Fatalf(":sk type = %T, want string", from)
			}
			if fromS.Value != tc.wantSk {
				t.Fatalf(":sk = %q, want %q", fromS.Value, tc.wantSk)
			}
		})
	}
}

func TestLoadStreamAfter_PaginatesPrimaryTableQuery(t *testing.T) {
	client := &paginatedClient{
		queryPages: []*awsdynamo.QueryOutput{
			{
				LastEvaluatedKey: map[string]awsdynamotypes.AttributeValue{
					"pkey": &awsdynamotypes.AttributeValueMemberS{Value: "T-0"},
					"skey": &awsdynamotypes.AttributeValueMemberS{Value: "T-other-00000000000000000099"},
				},
			},
			{
				Items: []map[string]awsdynamotypes.AttributeValue{
					{
						"seq_nr":      &awsdynamotypes.AttributeValueMemberN{Value: "2"},
						"occurred_at": &awsdynamotypes.AttributeValueMemberN{Value: "1714521600000"},
						"payload":     &awsdynamotypes.AttributeValueMemberB{Value: []byte("payload")},
						"type_name":   &awsdynamotypes.AttributeValueMemberS{Value: "Stub"},
						"event_id":    &awsdynamotypes.AttributeValueMemberS{Value: "evt-2"},
						"is_created":  &awsdynamotypes.AttributeValueMemberBOOL{Value: false},
					},
				},
			},
		},
	}
	store := v2dynamo.New[ddbStubAggregate, ddbStubCommand, ddbStubEvent](
		client,
		v2dynamo.DefaultConfig(),
		ddbStubAggSer{},
		ddbStubEvSer{},
	)

	events, err := store.LoadStreamAfter(context.Background(), ddbTestAggID{value: "1"}, 1)
	if err != nil {
		t.Fatalf("LoadStreamAfter: %v", err)
	}
	if len(client.queryInputs) != 2 {
		t.Fatalf("Query calls = %d, want 2", len(client.queryInputs))
	}
	if client.queryInputs[0].ExclusiveStartKey != nil {
		t.Fatal("first query ExclusiveStartKey = non-nil, want nil")
	}
	if client.queryInputs[1].ExclusiveStartKey == nil {
		t.Fatal("second query ExclusiveStartKey = nil, want LastEvaluatedKey from previous page")
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if events[0].SeqNr != 2 {
		t.Fatalf("events[0].SeqNr = %d, want 2", events[0].SeqNr)
	}
	if events[0].EventID != "evt-2" {
		t.Fatalf("events[0].EventID = %q, want %q", events[0].EventID, "evt-2")
	}
	if !events[0].OccurredAt.Equal(time.UnixMilli(1714521600000).UTC()) {
		t.Fatalf("events[0].OccurredAt = %s, want %s", events[0].OccurredAt, time.UnixMilli(1714521600000).UTC())
	}
}
