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

	es "github.com/Hiroshi0900/eventstore"
	"github.com/Hiroshi0900/eventstore/internal/keyresolver"
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

// store is the DynamoDB-backed EventStore[A, C, E] implementation.
// concrete type は非公開で、外部からは New() が返す es.EventStore[A, C, E] interface 経由でのみ操作する。
//
// AggregateSerializer / EventSerializer は構築時に注入され、attribute marshal/unmarshal で
// domain 型 ↔ bytes の変換に使われる。Repository 層には serialize の責務が漏れない設計。
type store[A es.Aggregate[C, E], C es.Command, E es.Event] struct {
	client      Client
	keyResolver *keyresolver.Resolver
	config      Config
	aggSer      es.AggregateSerializer[A, C, E]
	evSer       es.EventSerializer[E]
}

// New は DynamoDB-backed EventStore[A, C, E] を生成する。
//
// aggSer / evSer は snapshot / event payload の (de)serialize に使われる。
func New[A es.Aggregate[C, E], C es.Command, E es.Event](
	client Client,
	config Config,
	aggSer es.AggregateSerializer[A, C, E],
	evSer es.EventSerializer[E],
) es.EventStore[A, C, E] {
	return &store[A, C, E]{
		client:      client,
		keyResolver: keyresolver.New(keyresolver.Config{ShardCount: config.ShardCount}),
		config:      config,
		aggSer:      aggSer,
		evSer:       evSer,
	}
}

// TableManager は CreateTables / WaitForTables などテーブル管理 API を提供する。
// store の concrete 型を露出させずにこれらの操作を呼べるようにする抽象。
type TableManager interface {
	CreateTables(ctx context.Context) error
	WaitForTables(ctx context.Context) error
}

// NewWithTables は EventStore[A, C, E] + TableManager の機能を併せ持つオブジェクトを返す。
// テーブル管理が必要なテスト・デプロイ初期化スクリプト等で使う。
func NewWithTables[A es.Aggregate[C, E], C es.Command, E es.Event](
	client Client,
	config Config,
	aggSer es.AggregateSerializer[A, C, E],
	evSer es.EventSerializer[E],
) interface {
	es.EventStore[A, C, E]
	TableManager
} {
	return &store[A, C, E]{
		client:      client,
		keyResolver: keyresolver.New(keyresolver.Config{ShardCount: config.ShardCount}),
		config:      config,
		aggSer:      aggSer,
		evSer:       evSer,
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

// GetLatestSnapshot returns the latest snapshot or found=false.
func (s *store[A, C, E]) GetLatestSnapshot(ctx context.Context, id es.AggregateID) (es.StoredSnapshot[A], bool, error) {
	var zero es.StoredSnapshot[A]
	keys := s.keyResolver.ResolveSnapshotKeys(id)
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.config.SnapshotTableName),
		Key: map[string]types.AttributeValue{
			colPKey: &types.AttributeValueMemberS{Value: keys.PartitionKey},
			colSKey: &types.AttributeValueMemberS{Value: keys.SortKey},
		},
	})
	if err != nil {
		return zero, false, fmt.Errorf("get snapshot: %w", err)
	}
	if out.Item == nil {
		return zero, false, nil
	}

	seqNr, err := getN(out.Item, colSeqNr)
	if err != nil {
		return zero, false, err
	}
	version, err := getN(out.Item, colVersion)
	if err != nil {
		return zero, false, err
	}
	payload, _ := getB(out.Item, colPayload)
	occurred, _ := getN(out.Item, colOccurred)

	agg, err := s.aggSer.Deserialize(payload)
	if err != nil {
		return zero, false, err
	}
	return es.StoredSnapshot[A]{
		Aggregate: agg,
		SeqNr:     seqNr,
		Version:   version,
		// #nosec G115 -- UnixMilli は実用範囲で int64 に収まる
		OccurredAt: time.UnixMilli(int64(occurred)).UTC(),
	}, true, nil
}

// LoadStreamAfter returns events with SeqNr > seqNr using a strongly consistent primary-table query.
func (s *store[A, C, E]) LoadStreamAfter(ctx context.Context, id es.AggregateID, seqNr uint64) ([]es.StoredEvent[E], error) {
	keys := s.keyResolver.ResolveEventKeys(id, seqNr)
	out, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName: aws.String(s.config.JournalTableName),
		KeyConditionExpression: aws.String("#pk = :pk AND #sk > :sk"),
		ExpressionAttributeNames: map[string]string{
			"#pk": colPKey,
			"#sk": colSKey,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: keys.PartitionKey},
			":sk": &types.AttributeValueMemberS{Value: keys.SortKey},
		},
		ConsistentRead:   aws.Bool(true),
		ScanIndexForward: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	stored := make([]es.StoredEvent[E], 0, len(out.Items))
	for _, item := range out.Items {
		ev, err := s.unmarshalEvent(item)
		if err != nil {
			return nil, err
		}
		stored = append(stored, ev)
	}
	return stored, nil
}

// PersistEvent writes a single event. ConditionExpression は同一 (pkey, skey) の重複保存を拒否し、
// ConditionalCheckFailed を ErrDuplicateAggregate にマッピングする。
func (s *store[A, C, E]) PersistEvent(ctx context.Context, ev es.StoredEvent[E], _ uint64) error {
	item, err := s.marshalEvent(ctx, ev)
	if err != nil {
		return err
	}
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
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
			return es.NewDuplicateAggregateError(ev.Event.AggregateID().AsString())
		}
		return fmt.Errorf("put event: %w", err)
	}
	return nil
}

// PersistEventAndSnapshot writes both event and snapshot atomically using TransactWriteItems.
// snapshot put has a Version condition for optimistic locking.
func (s *store[A, C, E]) PersistEventAndSnapshot(ctx context.Context, ev es.StoredEvent[E], snap es.StoredSnapshot[A]) error {
	eventItem, err := s.marshalEvent(ctx, ev)
	if err != nil {
		return err
	}
	snapshotItem, err := s.marshalSnapshot(snap)
	if err != nil {
		return err
	}

	expected := snap.Version - 1
	condExpr := "attribute_not_exists(#version) OR #version = :expected"
	if expected == 0 {
		condExpr = "attribute_not_exists(#version)"
	}

	_, err = s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
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
			return es.NewOptimisticLockError(ev.Event.AggregateID().AsString(), expected, 0)
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

func (s *store[A, C, E]) marshalEvent(ctx context.Context, ev es.StoredEvent[E]) (map[string]types.AttributeValue, error) {
	payload, err := s.evSer.Serialize(ev.Event)
	if err != nil {
		return nil, err
	}
	keys := s.keyResolver.ResolveEventKeys(ev.Event.AggregateID(), ev.SeqNr)
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)

	item := map[string]types.AttributeValue{
		colPKey:      &types.AttributeValueMemberS{Value: keys.PartitionKey},
		colSKey:      &types.AttributeValueMemberS{Value: keys.SortKey},
		colAid:       &types.AttributeValueMemberS{Value: keys.AggregateIDKey},
		colSeqNr:     &types.AttributeValueMemberN{Value: strconv.FormatUint(ev.SeqNr, 10)},
		colPayload:   &types.AttributeValueMemberB{Value: payload},
		colOccurred:  &types.AttributeValueMemberN{Value: strconv.FormatInt(ev.OccurredAt.UnixMilli(), 10)},
		colTypeName:  &types.AttributeValueMemberS{Value: ev.Event.EventTypeName()},
		colEventID:   &types.AttributeValueMemberS{Value: ev.EventID},
		colIsCreated: &types.AttributeValueMemberBOOL{Value: ev.IsCreated},
	}
	if tp := carrier.Get("traceparent"); tp != "" {
		item[colTraceparent] = &types.AttributeValueMemberS{Value: tp}
	}
	if ts := carrier.Get("tracestate"); ts != "" {
		item[colTracestate] = &types.AttributeValueMemberS{Value: ts}
	}
	return item, nil
}

func (s *store[A, C, E]) marshalSnapshot(snap es.StoredSnapshot[A]) (map[string]types.AttributeValue, error) {
	payload, err := s.aggSer.Serialize(snap.Aggregate)
	if err != nil {
		return nil, err
	}
	keys := s.keyResolver.ResolveSnapshotKeys(snap.Aggregate.AggregateID())
	return map[string]types.AttributeValue{
		colPKey:     &types.AttributeValueMemberS{Value: keys.PartitionKey},
		colSKey:     &types.AttributeValueMemberS{Value: keys.SortKey},
		colAid:      &types.AttributeValueMemberS{Value: keys.AggregateIDKey},
		colSeqNr:    &types.AttributeValueMemberN{Value: strconv.FormatUint(snap.SeqNr, 10)},
		colVersion:  &types.AttributeValueMemberN{Value: strconv.FormatUint(snap.Version, 10)},
		colPayload:  &types.AttributeValueMemberB{Value: payload},
		colOccurred: &types.AttributeValueMemberN{Value: strconv.FormatInt(snap.OccurredAt.UnixMilli(), 10)},
	}, nil
}

func (s *store[A, C, E]) unmarshalEvent(item map[string]types.AttributeValue) (es.StoredEvent[E], error) {
	var zero es.StoredEvent[E]
	seqNr, err := getN(item, colSeqNr)
	if err != nil {
		return zero, err
	}
	occurred, err := getN(item, colOccurred)
	if err != nil {
		return zero, err
	}
	payload, _ := getB(item, colPayload)
	typeName, _ := getS(item, colTypeName)
	eventID, _ := getS(item, colEventID)
	isCreated, _ := getBool(item, colIsCreated)
	traceparent, _ := getS(item, colTraceparent)
	tracestate, _ := getS(item, colTracestate)

	ev, err := s.evSer.Deserialize(typeName, payload)
	if err != nil {
		return zero, err
	}

	return es.StoredEvent[E]{
		Event:     ev,
		EventID:   eventID,
		SeqNr:     seqNr,
		IsCreated: isCreated,
		// #nosec G115 -- UnixMilli は実用範囲で int64 に収まる
		OccurredAt:  time.UnixMilli(int64(occurred)).UTC(),
		TraceParent: traceparent,
		TraceState:  tracestate,
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
func (s *store[A, C, E]) CreateTables(ctx context.Context) error {
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
func (s *store[A, C, E]) WaitForTables(ctx context.Context) error {
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
