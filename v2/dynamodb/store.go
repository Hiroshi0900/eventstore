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

// Client is the interface for the DynamoDB client.
// Used to enable mocking in tests.
type Client interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
	CreateTable(ctx context.Context, params *dynamodb.CreateTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.CreateTableOutput, error)
	DescribeTable(ctx context.Context, params *dynamodb.DescribeTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error)
}

// Store[T] is the DynamoDB-backed EventStore[T] implementation.
//
// Aggregate (T) の (de)serialize は構築時に注入される AggregateSerializer[T] が担当します。
// snapshot column 上は payload (bytes) として保存し、SeqNr / Version は別 column に展開します。
type Store[T es.Aggregate] struct {
	client      Client
	keyResolver *keyresolver.Resolver
	config      Config
	serializer  es.AggregateSerializer[T]
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

// New creates a new DynamoDB event store for aggregate type T.
// serializer は snapshot payload の (de)serialize に使われます。
func New[T es.Aggregate](client Client, config Config, serializer es.AggregateSerializer[T]) *Store[T] {
	return &Store[T]{
		client:      client,
		keyResolver: keyresolver.New(keyresolver.Config{ShardCount: config.ShardCount}),
		config:      config,
		serializer:  serializer,
	}
}

// DynamoDB table column names.
const (
	colPKey        = "pkey"
	colSKey        = "skey"
	colAid         = "aid"
	colSeqNr       = "seq_nr"
	colPayload     = "payload"
	colOccurred    = "occurred_at"
	colTypeName    = "type_name"
	colEventId     = "event_id"
	colIsCreated   = "is_created"
	colVersion     = "version"
	colTraceparent = "traceparent"
	colTracestate  = "tracestate"
)

// GetLatestSnapshotByID retrieves and deserializes the latest snapshot for the given aggregate ID.
// 第 2 戻り値 found=false なら snapshot 未存在 (エラーではない)。
func (s *Store[T]) GetLatestSnapshotByID(ctx context.Context, id es.AggregateID) (T, bool, error) {
	var zero T

	keys := s.keyResolver.ResolveSnapshotKeys(id)

	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.config.SnapshotTableName),
		Key: map[string]types.AttributeValue{
			colPKey: &types.AttributeValueMemberS{Value: keys.PartitionKey},
			colSKey: &types.AttributeValueMemberS{Value: keys.SortKey},
		},
	})
	if err != nil {
		return zero, false, fmt.Errorf("failed to get snapshot: %w", err)
	}

	if result.Item == nil {
		return zero, false, nil
	}

	var payload []byte
	var seqNr, version uint64
	if v, ok := result.Item[colPayload].(*types.AttributeValueMemberB); ok {
		payload = v.Value
	}
	if v, ok := result.Item[colSeqNr].(*types.AttributeValueMemberN); ok {
		seqNr, _ = strconv.ParseUint(v.Value, 10, 64)
	}
	if v, ok := result.Item[colVersion].(*types.AttributeValueMemberN); ok {
		version, _ = strconv.ParseUint(v.Value, 10, 64)
	}

	agg, err := s.serializer.Deserialize(payload)
	if err != nil {
		return zero, false, err
	}
	// DynamoDB column 上の SeqNr / Version を権威として集約に反映。
	agg = agg.WithSeqNr(seqNr).(T)
	agg = agg.WithVersion(version).(T)
	return agg, true, nil
}

// GetEventsByIDSinceSeqNr retrieves all events since the given sequence number.
func (s *Store[T]) GetEventsByIDSinceSeqNr(ctx context.Context, id es.AggregateID, seqNr uint64) ([]es.Event, error) {
	aidKey := s.keyResolver.ResolveAggregateIDKey(id)

	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
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
		return nil, fmt.Errorf("failed to query events: %w", err)
	}

	events := make([]es.Event, 0, len(result.Items))
	for _, item := range result.Items {
		event, err := s.unmarshalEvent(item, id)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	return events, nil
}

// PersistEvent persists a single event without a snapshot.
func (s *Store[T]) PersistEvent(ctx context.Context, event es.Event, version uint64) error {
	keys := s.keyResolver.ResolveEventKeys(event.AggregateID(), event.SeqNr())

	eventItem, err := s.marshalEvent(ctx, event, keys)
	if err != nil {
		return err
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.config.JournalTableName),
		Item:                eventItem,
		ConditionExpression: aws.String("attribute_not_exists(#pk)"),
		ExpressionAttributeNames: map[string]string{
			"#pk": colPKey,
		},
	})
	if err != nil {
		var ccfe *types.ConditionalCheckFailedException
		if errors.As(err, &ccfe) {
			return es.NewDuplicateAggregateError(event.AggregateID().AsString())
		}
		return fmt.Errorf("failed to persist event: %w", err)
	}

	return nil
}

// PersistEventAndSnapshot atomically persists an event and a snapshot.
//
// 楽観ロックは aggregate.Version() を起点にする。期待 version は aggregate.Version() - 1 で、
// aggregate.Version() <= 1 の場合は初回作成扱い (attribute_not_exists 条件)。
// snapshot payload は serializer.Serialize(aggregate) で生成する。
func (s *Store[T]) PersistEventAndSnapshot(ctx context.Context, event es.Event, aggregate T) error {
	eventKeys := s.keyResolver.ResolveEventKeys(event.AggregateID(), event.SeqNr())
	snapshotKeys := s.keyResolver.ResolveSnapshotKeys(event.AggregateID())

	eventItem, err := s.marshalEvent(ctx, event, eventKeys)
	if err != nil {
		return err
	}

	payload, err := s.serializer.Serialize(aggregate)
	if err != nil {
		return es.NewSerializationError("aggregate", err)
	}

	snapshotItem := s.marshalSnapshot(payload, aggregate.SeqNr(), aggregate.Version(), snapshotKeys)

	isFirstSnapshot := aggregate.Version() <= 1
	var expectedVersion uint64
	if !isFirstSnapshot {
		expectedVersion = aggregate.Version() - 1
	}

	transactItems := []types.TransactWriteItem{
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
	}

	if isFirstSnapshot {
		transactItems = append(transactItems, types.TransactWriteItem{
			Put: &types.Put{
				TableName:           aws.String(s.config.SnapshotTableName),
				Item:                snapshotItem,
				ConditionExpression: aws.String("attribute_not_exists(#pk)"),
				ExpressionAttributeNames: map[string]string{
					"#pk": colPKey,
				},
			},
		})
	} else {
		transactItems = append(transactItems, types.TransactWriteItem{
			Put: &types.Put{
				TableName:           aws.String(s.config.SnapshotTableName),
				Item:                snapshotItem,
				ConditionExpression: aws.String("#version = :expected_version"),
				ExpressionAttributeNames: map[string]string{
					"#version": colVersion,
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":expected_version": &types.AttributeValueMemberN{Value: strconv.FormatUint(expectedVersion, 10)},
				},
			},
		})
	}

	_, err = s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: transactItems,
	})
	if err != nil {
		if isTransactionCanceledDueToCondition(err) {
			return es.NewOptimisticLockError(
				event.AggregateID().AsString(),
				expectedVersion,
				0, // actual version is unknown
			)
		}
		return fmt.Errorf("failed to persist event and snapshot: %w", err)
	}

	return nil
}

// marshalEvent marshals an event into a DynamoDB item.
func (s *Store[T]) marshalEvent(ctx context.Context, event es.Event, keys keyresolver.KeyComponents) (map[string]types.AttributeValue, error) {
	item := map[string]types.AttributeValue{
		colPKey:      &types.AttributeValueMemberS{Value: keys.PartitionKey},
		colSKey:      &types.AttributeValueMemberS{Value: keys.SortKey},
		colAid:       &types.AttributeValueMemberS{Value: keys.AggregateIDKey},
		colSeqNr:     &types.AttributeValueMemberN{Value: strconv.FormatUint(event.SeqNr(), 10)},
		colEventId:   &types.AttributeValueMemberS{Value: event.EventID()},
		colTypeName:  &types.AttributeValueMemberS{Value: event.EventTypeName()},
		colPayload:   &types.AttributeValueMemberB{Value: event.Payload()},
		colOccurred:  &types.AttributeValueMemberN{Value: strconv.FormatInt(event.OccurredAt().UnixMilli(), 10)},
		colIsCreated: &types.AttributeValueMemberBOOL{Value: event.IsCreated()},
	}

	m := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, m)
	if tp := m["traceparent"]; tp != "" {
		item[colTraceparent] = &types.AttributeValueMemberS{Value: tp}
	}
	if ts := m["tracestate"]; ts != "" {
		item[colTracestate] = &types.AttributeValueMemberS{Value: ts}
	}

	return item, nil
}

// unmarshalEvent unmarshals a DynamoDB item into an Event.
func (s *Store[T]) unmarshalEvent(item map[string]types.AttributeValue, id es.AggregateID) (es.Event, error) {
	var eventId, typeName string
	var seqNr uint64
	var occurredAt int64
	var isCreated bool
	var payload []byte

	if v, ok := item[colEventId].(*types.AttributeValueMemberS); ok {
		eventId = v.Value
	}
	if v, ok := item[colTypeName].(*types.AttributeValueMemberS); ok {
		typeName = v.Value
	}
	if v, ok := item[colSeqNr].(*types.AttributeValueMemberN); ok {
		seqNr, _ = strconv.ParseUint(v.Value, 10, 64)
	}
	if v, ok := item[colOccurred].(*types.AttributeValueMemberN); ok {
		occurredAt, _ = strconv.ParseInt(v.Value, 10, 64)
	}
	if v, ok := item[colIsCreated].(*types.AttributeValueMemberBOOL); ok {
		isCreated = v.Value
	}
	if v, ok := item[colPayload].(*types.AttributeValueMemberB); ok {
		payload = v.Value
	}

	return es.NewEvent(
		eventId,
		typeName,
		id,
		payload,
		es.WithSeqNr(seqNr),
		es.WithIsCreated(isCreated),
		es.WithOccurredAt(time.UnixMilli(occurredAt)),
	), nil
}

// marshalSnapshot builds a DynamoDB item for the snapshot column set.
// payload は AggregateSerializer[T] が生成したバイト列。
func (s *Store[T]) marshalSnapshot(payload []byte, seqNr, version uint64, keys keyresolver.KeyComponents) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		colPKey:    &types.AttributeValueMemberS{Value: keys.PartitionKey},
		colSKey:    &types.AttributeValueMemberS{Value: keys.SortKey},
		colAid:     &types.AttributeValueMemberS{Value: keys.AggregateIDKey},
		colSeqNr:   &types.AttributeValueMemberN{Value: strconv.FormatUint(seqNr, 10)},
		colVersion: &types.AttributeValueMemberN{Value: strconv.FormatUint(version, 10)},
		colPayload: &types.AttributeValueMemberB{Value: payload},
	}
}

// isTransactionCanceledDueToCondition checks whether the transaction was canceled due to a condition check failure.
func isTransactionCanceledDueToCondition(err error) bool {
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

// CreateTables creates the journal and snapshot tables (for testing/setup).
func (s *Store[T]) CreateTables(ctx context.Context) error {
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
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeAll,
				},
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
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeAll,
				},
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
func (s *Store[T]) WaitForTables(ctx context.Context) error {
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
