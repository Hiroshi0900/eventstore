package dynamodb_test

import (
	"context"
	"testing"

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

// stub types for generic instantiation.
type ddbTestAggID struct{ value string }

func (a ddbTestAggID) TypeName() string { return "T" }
func (a ddbTestAggID) Value() string    { return a.value }
func (a ddbTestAggID) AsString() string { return "T-" + a.value }

type ddbStubEvent struct{ aggID ddbTestAggID }

func (e ddbStubEvent) EventTypeName() string       { return "Stub" }
func (e ddbStubEvent) AggregateID() es.AggregateID { return e.aggID }

type ddbStubAggregate struct{ id ddbTestAggID }

func (a ddbStubAggregate) AggregateID() es.AggregateID                  { return a.id }
func (a ddbStubAggregate) ApplyCommand(es.Command) (ddbStubEvent, error) { return ddbStubEvent{}, es.ErrUnknownCommand }
func (a ddbStubAggregate) ApplyEvent(ddbStubEvent) es.Aggregate[ddbStubEvent] { return a }

type ddbStubAggSer struct{}

func (ddbStubAggSer) Serialize(ddbStubAggregate) ([]byte, error)   { return nil, nil }
func (ddbStubAggSer) Deserialize([]byte) (ddbStubAggregate, error) { return ddbStubAggregate{}, nil }

type ddbStubEvSer struct{}

func (ddbStubEvSer) Serialize(ddbStubEvent) ([]byte, error)               { return nil, nil }
func (ddbStubEvSer) Deserialize(string, []byte) (ddbStubEvent, error)     { return ddbStubEvent{}, nil }

func TestNew_Construct(t *testing.T) {
	store := v2dynamo.New[ddbStubAggregate, ddbStubEvent](
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
	mgr := v2dynamo.NewWithTables[ddbStubAggregate, ddbStubEvent](
		fakeClient{},
		v2dynamo.DefaultConfig(),
		ddbStubAggSer{},
		ddbStubEvSer{},
	)
	if mgr == nil {
		t.Fatal("expected non-nil TableManager")
	}
}
