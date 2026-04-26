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

// Store is the DynamoDB-backed EventStore implementation.
type Store struct {
	client      Client
	keyResolver *keyresolver.Resolver
	config      Config
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

// New creates a new DynamoDB event store.
func New(client Client, config Config) *Store {
	return &Store{
		client:      client,
		keyResolver: keyresolver.New(keyresolver.Config{ShardCount: config.ShardCount}),
		config:      config,
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

// GetLatestSnapshotByID retrieves the latest snapshot for the given aggregate ID.
func (s *Store) GetLatestSnapshotByID(ctx context.Context, id es.AggregateID) (*es.SnapshotData, error) {
	keys := s.keyResolver.ResolveSnapshotKeys(id)

	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.config.SnapshotTableName),
		Key: map[string]types.AttributeValue{
			colPKey: &types.AttributeValueMemberS{Value: keys.PartitionKey},
			colSKey: &types.AttributeValueMemberS{Value: keys.SortKey},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot: %w", err)
	}

	if result.Item == nil {
		return nil, nil
	}

	return s.unmarshalSnapshot(result.Item)
}

// GetEventsByIDSinceSeqNr retrieves all events since the given sequence number.
func (s *Store) GetEventsByIDSinceSeqNr(ctx context.Context, id es.AggregateID, seqNr uint64) ([]es.Event, error) {
	aidKey := s.keyResolver.ResolveAggregateIDKey(id)

	// Query by aggregate ID using the GSI
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
		ScanIndexForward: aws.Bool(true), // ascending order
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
func (s *Store) PersistEvent(ctx context.Context, event es.Event, version uint64) error {
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
// 楽観ロックは snapshot.Version を起点にする。期待 version は snapshot.Version - 1 で、
// snapshot.Version <= 1 の場合は初回作成扱い (attribute_not_exists 条件) になる。
// snapshot.Version 自体を新バージョンとして書き込む（呼び出し側で次の version を渡す契約）。
func (s *Store) PersistEventAndSnapshot(ctx context.Context, event es.Event, snapshot es.SnapshotData) error {
	eventKeys := s.keyResolver.ResolveEventKeys(event.AggregateID(), event.SeqNr())
	snapshotKeys := s.keyResolver.ResolveSnapshotKeys(event.AggregateID())

	eventItem, err := s.marshalEvent(ctx, event, eventKeys)
	if err != nil {
		return err
	}

	snapshotItem, err := s.marshalSnapshot(snapshot, snapshotKeys)
	if err != nil {
		return err
	}

	// uint64 underflow を避けるため snapshot.Version <= 1 を初回判定に使う。
	isFirstSnapshot := snapshot.Version <= 1
	var expectedVersion uint64
	if !isFirstSnapshot {
		expectedVersion = snapshot.Version - 1
	}

	// Use TransactWriteItems for atomic operation
	transactItems := []types.TransactWriteItem{
		// Persist the event
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

	// Add snapshot operation
	if isFirstSnapshot {
		// First snapshot — use Put with condition that the item does not exist
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
		// Update snapshot with optimistic locking
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
func (s *Store) marshalEvent(ctx context.Context, event es.Event, keys keyresolver.KeyComponents) (map[string]types.AttributeValue, error) {
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
func (s *Store) unmarshalEvent(item map[string]types.AttributeValue, id es.AggregateID) (es.Event, error) {
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

// marshalSnapshot marshals a SnapshotData into a DynamoDB snapshot item.
func (s *Store) marshalSnapshot(snapshot es.SnapshotData, keys keyresolver.KeyComponents) (map[string]types.AttributeValue, error) {
	item := map[string]types.AttributeValue{
		colPKey:    &types.AttributeValueMemberS{Value: keys.PartitionKey},
		colSKey:    &types.AttributeValueMemberS{Value: keys.SortKey},
		colAid:     &types.AttributeValueMemberS{Value: keys.AggregateIDKey},
		colSeqNr:   &types.AttributeValueMemberN{Value: strconv.FormatUint(snapshot.SeqNr, 10)},
		colVersion: &types.AttributeValueMemberN{Value: strconv.FormatUint(snapshot.Version, 10)},
		colPayload: &types.AttributeValueMemberB{Value: snapshot.Payload},
	}
	return item, nil
}

// unmarshalSnapshot unmarshals a DynamoDB item into SnapshotData.
func (s *Store) unmarshalSnapshot(item map[string]types.AttributeValue) (*es.SnapshotData, error) {
	var snapshot es.SnapshotData

	if v, ok := item[colSeqNr].(*types.AttributeValueMemberN); ok {
		snapshot.SeqNr, _ = strconv.ParseUint(v.Value, 10, 64)
	}
	if v, ok := item[colVersion].(*types.AttributeValueMemberN); ok {
		snapshot.Version, _ = strconv.ParseUint(v.Value, 10, 64)
	}
	if v, ok := item[colPayload].(*types.AttributeValueMemberB); ok {
		snapshot.Payload = v.Value
	}

	return &snapshot, nil
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
func (s *Store) CreateTables(ctx context.Context) error {
	// Create the journal table
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
	if err != nil {
		if !isTableAlreadyExistsError(err) {
			return fmt.Errorf("failed to create journal table: %w", err)
		}
	}

	// Create the snapshot table
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
	if err != nil {
		if !isTableAlreadyExistsError(err) {
			return fmt.Errorf("failed to create snapshot table: %w", err)
		}
	}

	return nil
}

// WaitForTables waits until both tables are active.
func (s *Store) WaitForTables(ctx context.Context) error {
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
