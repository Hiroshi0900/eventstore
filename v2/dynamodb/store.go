// Package dynamodb provides a DynamoDB-backed EventStore implementation.
package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	es "github.com/Hiroshi0900/eventstore/v2"
	"github.com/Hiroshi0900/eventstore/v2/internal/keyresolver"
)

// Client は DynamoDB クライアントの subset interface (テストの mock 用)。
type Client interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
	CreateTable(ctx context.Context, params *dynamodb.CreateTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.CreateTableOutput, error)
	DescribeTable(ctx context.Context, params *dynamodb.DescribeTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error)
}

// Config holds the DynamoDB event store configuration.
type Config struct {
	JournalTableName  string
	SnapshotTableName string
	JournalGSIName    string // GSI for querying by aggregate ID
	SnapshotGSIName   string // GSI for querying by aggregate ID
	ShardCount        uint64
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		JournalTableName:  "journal",
		SnapshotTableName: "snapshot",
		JournalGSIName:    "journal-aid-index",
		SnapshotGSIName:   "snapshot-aid-index",
		ShardCount:        1,
	}
}

// store is the DynamoDB-backed EventStore implementation.
// concrete type は非公開で、外部からは New() が返す es.EventStore interface 経由でのみ操作する。
type store struct {
	client      Client
	keyResolver *keyresolver.Resolver
	config      Config
}

// New は DynamoDB-backed EventStore を生成する。
// CreateTables / WaitForTables を呼びたい場合は型アサーション (`*TableManager`) を経由する必要があるため、
// テーブル管理は別途 NewWithTables を提供する (下記)。
func New(client Client, config Config) es.EventStore {
	return &store{
		client:      client,
		keyResolver: keyresolver.New(keyresolver.Config{ShardCount: config.ShardCount}),
		config:      config,
	}
}

// TableManager は CreateTables / WaitForTables などテーブル管理 API を提供する。
// store の concrete 型を露出させずにこれらの操作を呼べるようにする抽象。
type TableManager interface {
	CreateTables(ctx context.Context) error
	WaitForTables(ctx context.Context) error
}

// NewWithTables は EventStore + TableManager の機能を併せ持つオブジェクトを返す。
// テーブル管理が必要なテスト・デプロイ初期化スクリプト等で使う。
func NewWithTables(client Client, config Config) interface {
	es.EventStore
	TableManager
} {
	return &store{
		client:      client,
		keyResolver: keyresolver.New(keyresolver.Config{ShardCount: config.ShardCount}),
		config:      config,
	}
}

// DynamoDB attribute names.
const (
	colPKey        = "pkey"
	colSKey        = "skey"
	colAid         = "aid"
	colSeqNr       = "seq_nr"
	colPayload     = "payload"
	colOccurred    = "occurred_at"
	colTypeName    = "type_name"
	colEventID     = "event_id"
	colIsCreated   = "is_created"
	colVersion     = "version"
	colTraceparent = "traceparent"
	colTracestate  = "tracestate"
)

// GetLatestSnapshot returns the latest snapshot envelope or nil.
func (s *store) GetLatestSnapshot(ctx context.Context, id es.AggregateID) (*es.SnapshotEnvelope, error) {
	keys := s.keyResolver.ResolveSnapshotKeys(id)
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.config.SnapshotTableName),
		Key: map[string]types.AttributeValue{
			colPKey: &types.AttributeValueMemberS{Value: keys.PartitionKey},
			colSKey: &types.AttributeValueMemberS{Value: keys.SortKey},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get snapshot: %w", err)
	}
	if out.Item == nil {
		return nil, nil
	}
	return s.unmarshalSnapshot(out.Item, id)
}

// GetEventsSince queries the journal GSI for events with SeqNr > seqNr.
func (s *store) GetEventsSince(ctx context.Context, id es.AggregateID, seqNr uint64) ([]*es.EventEnvelope, error) {
	aidKey := s.keyResolver.ResolveAggregateIDKey(id)
	out, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.config.JournalTableName),
		IndexName:              aws.String(s.config.JournalGSIName),
		KeyConditionExpression: aws.String("#aid = :aid AND #seq_nr > :seq_nr"),
		ExpressionAttributeNames: map[string]string{
			"#aid":    colAid,
			"#seq_nr": colSeqNr,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":aid":    &types.AttributeValueMemberS{Value: aidKey},
			":seq_nr": &types.AttributeValueMemberN{Value: strconv.FormatUint(seqNr, 10)},
		},
		ScanIndexForward: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	envs := make([]*es.EventEnvelope, 0, len(out.Items))
	for _, item := range out.Items {
		ev, err := s.unmarshalEvent(item, id)
		if err != nil {
			return nil, err
		}
		envs = append(envs, ev)
	}
	return envs, nil
}

// PersistEvent writes a single event envelope. ConditionExpression は同一 (pkey, skey) の重複保存を拒否し、
// ConditionalCheckFailed を ErrDuplicateAggregate にマッピングする。
func (s *store) PersistEvent(ctx context.Context, ev *es.EventEnvelope, _ uint64) error {
	item := s.marshalEvent(ctx, ev)
	_, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.config.JournalTableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(#pk)"),
		ExpressionAttributeNames: map[string]string{
			"#pk": colPKey,
		},
	})
	if err != nil {
		var ccfe *types.ConditionalCheckFailedException
		if errors.As(err, &ccfe) {
			return es.NewDuplicateAggregateError(ev.AggregateID.AsString())
		}
		return fmt.Errorf("put event: %w", err)
	}
	return nil
}

// PersistEventAndSnapshot writes both event and snapshot atomically using TransactWriteItems.
// snapshot put has a Version condition for optimistic locking.
func (s *store) PersistEventAndSnapshot(ctx context.Context, ev *es.EventEnvelope, snap *es.SnapshotEnvelope) error {
	eventItem := s.marshalEvent(ctx, ev)
	snapshotItem := s.marshalSnapshot(snap)

	expected := snap.Version - 1
	condExpr := "attribute_not_exists(#version) OR #version = :expected"
	if expected == 0 {
		condExpr = "attribute_not_exists(#version)"
	}

	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				Put: &types.Put{
					TableName:           aws.String(s.config.JournalTableName),
					Item:                eventItem,
					ConditionExpression: aws.String("attribute_not_exists(#pk)"),
					ExpressionAttributeNames: map[string]string{
						"#pk": colPKey,
					},
				},
			},
			{
				Put: &types.Put{
					TableName:                 aws.String(s.config.SnapshotTableName),
					Item:                      snapshotItem,
					ConditionExpression:       aws.String(condExpr),
					ExpressionAttributeNames:  map[string]string{"#version": colVersion},
					ExpressionAttributeValues: condValues(expected),
				},
			},
		},
	})
	if err != nil {
		if isConditionalCheckFailureInTransaction(err) {
			return es.NewOptimisticLockError(ev.AggregateID.AsString(), expected, 0)
		}
		return fmt.Errorf("transact write: %w", err)
	}
	return nil
}

func condValues(expected uint64) map[string]types.AttributeValue {
	if expected == 0 {
		return nil
	}
	return map[string]types.AttributeValue{
		":expected": &types.AttributeValueMemberN{Value: strconv.FormatUint(expected, 10)},
	}
}

func isConditionalCheckFailureInTransaction(err error) bool {
	var txErr *types.TransactionCanceledException
	if !errors.As(err, &txErr) || txErr == nil {
		return false
	}
	for _, reason := range txErr.CancellationReasons {
		if reason.Code != nil && *reason.Code == "ConditionalCheckFailed" {
			return true
		}
	}
	return false
}

func isTableAlreadyExistsError(err error) bool {
	var inUse *types.ResourceInUseException
	return errors.As(err, &inUse)
}

// --- attribute marshal / unmarshal ---

func (s *store) marshalEvent(ctx context.Context, ev *es.EventEnvelope) map[string]types.AttributeValue {
	keys := s.keyResolver.ResolveEventKeys(ev.AggregateID, ev.SeqNr)
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)

	item := map[string]types.AttributeValue{
		colPKey:      &types.AttributeValueMemberS{Value: keys.PartitionKey},
		colSKey:      &types.AttributeValueMemberS{Value: keys.SortKey},
		colAid:       &types.AttributeValueMemberS{Value: keys.AggregateIDKey},
		colSeqNr:     &types.AttributeValueMemberN{Value: strconv.FormatUint(ev.SeqNr, 10)},
		colPayload:   &types.AttributeValueMemberB{Value: ev.Payload},
		colOccurred:  &types.AttributeValueMemberN{Value: strconv.FormatInt(ev.OccurredAt.UnixMilli(), 10)},
		colTypeName:  &types.AttributeValueMemberS{Value: ev.EventTypeName},
		colEventID:   &types.AttributeValueMemberS{Value: ev.EventID},
		colIsCreated: &types.AttributeValueMemberBOOL{Value: ev.IsCreated},
	}
	if tp := carrier.Get("traceparent"); tp != "" {
		item[colTraceparent] = &types.AttributeValueMemberS{Value: tp}
	}
	if ts := carrier.Get("tracestate"); ts != "" {
		item[colTracestate] = &types.AttributeValueMemberS{Value: ts}
	}
	return item
}

func (s *store) marshalSnapshot(snap *es.SnapshotEnvelope) map[string]types.AttributeValue {
	keys := s.keyResolver.ResolveSnapshotKeys(snap.AggregateID)
	return map[string]types.AttributeValue{
		colPKey:     &types.AttributeValueMemberS{Value: keys.PartitionKey},
		colSKey:     &types.AttributeValueMemberS{Value: keys.SortKey},
		colAid:      &types.AttributeValueMemberS{Value: keys.AggregateIDKey},
		colSeqNr:    &types.AttributeValueMemberN{Value: strconv.FormatUint(snap.SeqNr, 10)},
		colVersion:  &types.AttributeValueMemberN{Value: strconv.FormatUint(snap.Version, 10)},
		colPayload:  &types.AttributeValueMemberB{Value: snap.Payload},
		colOccurred: &types.AttributeValueMemberN{Value: strconv.FormatInt(snap.OccurredAt.UnixMilli(), 10)},
	}
}

func (s *store) unmarshalEvent(item map[string]types.AttributeValue, fallbackID es.AggregateID) (*es.EventEnvelope, error) {
	seqNr, err := getN(item, colSeqNr)
	if err != nil {
		return nil, err
	}
	occurred, err := getN(item, colOccurred)
	if err != nil {
		return nil, err
	}
	payload, _ := getB(item, colPayload)
	typeName, _ := getS(item, colTypeName)
	eventID, _ := getS(item, colEventID)
	isCreated, _ := getBool(item, colIsCreated)
	traceparent, _ := getS(item, colTraceparent)
	tracestate, _ := getS(item, colTracestate)

	return &es.EventEnvelope{
		EventID:       eventID,
		EventTypeName: typeName,
		AggregateID:   fallbackID,
		SeqNr:         seqNr,
		IsCreated:     isCreated,
		// #nosec G115 -- UnixMilli は実用範囲で int64 に収まる
		OccurredAt:  time.UnixMilli(int64(occurred)).UTC(),
		Payload:     payload,
		TraceParent: traceparent,
		TraceState:  tracestate,
	}, nil
}

func (s *store) unmarshalSnapshot(item map[string]types.AttributeValue, fallbackID es.AggregateID) (*es.SnapshotEnvelope, error) {
	seqNr, err := getN(item, colSeqNr)
	if err != nil {
		return nil, err
	}
	version, err := getN(item, colVersion)
	if err != nil {
		return nil, err
	}
	payload, _ := getB(item, colPayload)
	occurred, _ := getN(item, colOccurred)

	return &es.SnapshotEnvelope{
		AggregateID: fallbackID,
		SeqNr:       seqNr,
		Version:     version,
		Payload:     payload,
		// #nosec G115
		OccurredAt: time.UnixMilli(int64(occurred)).UTC(),
	}, nil
}

func getS(item map[string]types.AttributeValue, key string) (string, bool) {
	v, ok := item[key].(*types.AttributeValueMemberS)
	if !ok {
		return "", false
	}
	return v.Value, true
}
func getB(item map[string]types.AttributeValue, key string) ([]byte, bool) {
	v, ok := item[key].(*types.AttributeValueMemberB)
	if !ok {
		return nil, false
	}
	return v.Value, true
}
func getBool(item map[string]types.AttributeValue, key string) (bool, bool) {
	v, ok := item[key].(*types.AttributeValueMemberBOOL)
	if !ok {
		return false, false
	}
	return v.Value, true
}
func getN(item map[string]types.AttributeValue, key string) (uint64, error) {
	v, ok := item[key].(*types.AttributeValueMemberN)
	if !ok {
		return 0, fmt.Errorf("attribute %s missing or wrong type", key)
	}
	n, err := strconv.ParseUint(v.Value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("attribute %s parse: %w", key, err)
	}
	return n, nil
}

// CreateTables creates the journal and snapshot tables (for testing/setup).
func (s *store) CreateTables(ctx context.Context) error {
	_, err := s.client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(s.config.JournalTableName),
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String(colPKey), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String(colSKey), KeyType: types.KeyTypeRange},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String(colPKey), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String(colSKey), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String(colAid), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String(colSeqNr), AttributeType: types.ScalarAttributeTypeN},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String(s.config.JournalGSIName),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String(colAid), KeyType: types.KeyTypeHash},
					{AttributeName: aws.String(colSeqNr), KeyType: types.KeyTypeRange},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil && !isTableAlreadyExistsError(err) {
		return fmt.Errorf("failed to create journal table: %w", err)
	}

	_, err = s.client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(s.config.SnapshotTableName),
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String(colPKey), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String(colSKey), KeyType: types.KeyTypeRange},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String(colPKey), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String(colSKey), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String(colAid), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String(colSeqNr), AttributeType: types.ScalarAttributeTypeN},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String(s.config.SnapshotGSIName),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String(colAid), KeyType: types.KeyTypeHash},
					{AttributeName: aws.String(colSeqNr), KeyType: types.KeyTypeRange},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil && !isTableAlreadyExistsError(err) {
		return fmt.Errorf("failed to create snapshot table: %w", err)
	}
	return nil
}

// WaitForTables waits until both tables are active.
func (s *store) WaitForTables(ctx context.Context) error {
	waiter := dynamodb.NewTableExistsWaiter(s.client)
	maxWaitTime := 5 * time.Minute

	if err := waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(s.config.JournalTableName),
	}, maxWaitTime); err != nil {
		return fmt.Errorf("failed to wait for journal table: %w", err)
	}

	if err := waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(s.config.SnapshotTableName),
	}, maxWaitTime); err != nil {
		return fmt.Errorf("failed to wait for snapshot table: %w", err)
	}
	return nil
}
