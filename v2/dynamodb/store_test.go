package dynamodb_test

import (
	"context"
	"testing"

	awsdynamo "github.com/aws/aws-sdk-go-v2/service/dynamodb"

	es "github.com/Hiroshi0900/eventstore/v2"
	v2dynamo "github.com/Hiroshi0900/eventstore/v2/dynamodb"
)

// fakeClient は dynamodb.Client interface を満たす最小実装。
// 各メソッドはゼロ値を返すのみ（構築テスト用）。
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

// stubAgg は generic Store[T] 構築テスト用の最小 Aggregate 実装。
type stubAgg struct {
	id      es.AggregateID
	seqNr   uint64
	version uint64
}

func (a stubAgg) AggregateID() es.AggregateID                  { return a.id }
func (a stubAgg) SeqNr() uint64                                { return a.seqNr }
func (a stubAgg) Version() uint64                              { return a.version }
func (a stubAgg) WithSeqNr(s uint64) es.Aggregate              { a.seqNr = s; return a }
func (a stubAgg) WithVersion(v uint64) es.Aggregate            { a.version = v; return a }
func (a stubAgg) ApplyCommand(es.Command) (es.Event, error)    { return nil, es.ErrUnknownCommand }
func (a stubAgg) ApplyEvent(es.Event) es.Aggregate             { return a }

type stubSerializer struct{}

func (stubSerializer) Serialize(stubAgg) ([]byte, error)   { return nil, nil }
func (stubSerializer) Deserialize([]byte) (stubAgg, error) { return stubAgg{}, nil }

func TestNew_Construct(t *testing.T) {
	store := v2dynamo.New[stubAgg](fakeClient{}, v2dynamo.DefaultConfig(), stubSerializer{})
	if store == nil {
		t.Fatal("expected non-nil Store")
	}
}
