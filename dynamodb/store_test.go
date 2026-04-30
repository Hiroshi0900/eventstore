package dynamodb_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsdynamo "github.com/aws/aws-sdk-go-v2/service/dynamodb"

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
}
