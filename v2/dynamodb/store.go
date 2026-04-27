// Package dynamodb provides a DynamoDB-backed EventStore implementation for v2.
package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	es "github.com/Hiroshi0900/eventstore/v2"
	"github.com/Hiroshi0900/eventstore/v2/internal/keyresolver"
)

// Client is the subset of dynamodb.Client used by Store. Defined as an
// interface to enable mocking in tests.
type Client interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

// Config holds DynamoDB-specific settings.
type Config struct {
	JournalTableName  string
	SnapshotTableName string
	JournalGSIName    string
	SnapshotGSIName   string
	ShardCount        uint64
}

// DefaultConfig returns the default DynamoDB configuration.
func DefaultConfig() Config {
	return Config{
		JournalTableName:  "journal",
		SnapshotTableName: "snapshot",
		JournalGSIName:    "journal-aid-index",
		SnapshotGSIName:   "snapshot-aid-index",
		ShardCount:        1,
	}
}

// Store implements es.EventStore on top of DynamoDB.
type Store struct {
	client      Client
	keyResolver *keyresolver.Resolver
	config      Config
}

// New creates a new Store.
func New(client Client, config Config) *Store {
	return &Store{
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

// Compile-time interface assertion.
var _ es.EventStore = (*Store)(nil)

// GetLatestSnapshot returns the latest snapshot or nil.
func (s *Store) GetLatestSnapshot(ctx context.Context, id es.AggregateID) (*es.SnapshotEnvelope, error) {
	keys := s.keyResolver.ResolveSnapshotKeys(id)
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: awssdk.String(s.config.SnapshotTableName),
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
	return s.unmarshalSnapshot(out.Item)
}

// GetEventsSince queries the journal GSI for events with SeqNr > seqNr.
func (s *Store) GetEventsSince(ctx context.Context, id es.AggregateID, seqNr uint64) ([]*es.EventEnvelope, error) {
	aidKey := s.keyResolver.ResolveAggregateIDKey(id)
	out, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              awssdk.String(s.config.JournalTableName),
		IndexName:              awssdk.String(s.config.JournalGSIName),
		KeyConditionExpression: awssdk.String("#aid = :aid AND #seq_nr > :seq_nr"),
		ExpressionAttributeNames: map[string]string{
			"#aid":    colAid,
			"#seq_nr": colSeqNr,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":aid":    &types.AttributeValueMemberS{Value: aidKey},
			":seq_nr": &types.AttributeValueMemberN{Value: strconv.FormatUint(seqNr, 10)},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	envs := make([]*es.EventEnvelope, 0, len(out.Items))
	for _, item := range out.Items {
		ev, err := s.unmarshalEvent(item)
		if err != nil {
			return nil, err
		}
		envs = append(envs, ev)
	}
	return envs, nil
}

// PersistEvent writes a single event without snapshot. ConditionExpression
// rejects duplicate (pkey, skey) entries, mapping the conditional check
// failure to ErrDuplicateAggregate.
func (s *Store) PersistEvent(ctx context.Context, ev *es.EventEnvelope, expectedVersion uint64) error {
	item := s.marshalEvent(ctx, ev)
	_, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           awssdk.String(s.config.JournalTableName),
		Item:                item,
		ConditionExpression: awssdk.String("attribute_not_exists(#pkey)"),
		ExpressionAttributeNames: map[string]string{
			"#pkey": colPKey,
		},
	})
	if err != nil {
		var ccfe *types.ConditionalCheckFailedException
		if errors.As(err, &ccfe) {
			return es.NewDuplicateAggregateError(ev.AggregateID.AsString())
		}
		return fmt.Errorf("put event: %w", err)
	}
	_ = expectedVersion // not used for non-snapshot writes
	return nil
}

// PersistEventAndSnapshot writes both event and snapshot atomically using
// TransactWriteItems. The snapshot put has a Version condition for optimistic
// locking: existing snapshot's version must equal snap.Version - 1 (or the
// snapshot must not exist yet, for the initial write).
func (s *Store) PersistEventAndSnapshot(ctx context.Context, ev *es.EventEnvelope, snap *es.SnapshotEnvelope) error {
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
					TableName:           awssdk.String(s.config.JournalTableName),
					Item:                eventItem,
					ConditionExpression: awssdk.String("attribute_not_exists(#pkey)"),
					ExpressionAttributeNames: map[string]string{
						"#pkey": colPKey,
					},
				},
			},
			{
				Put: &types.Put{
					TableName:                 awssdk.String(s.config.SnapshotTableName),
					Item:                      snapshotItem,
					ConditionExpression:       awssdk.String(condExpr),
					ExpressionAttributeNames:  map[string]string{"#version": colVersion},
					ExpressionAttributeValues: condValues(expected),
				},
			},
		},
	})
	if err != nil {
		var tce *types.TransactionCanceledException
		if errors.As(err, &tce) {
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

// --- attribute marshal / unmarshal ---

func (s *Store) marshalEvent(ctx context.Context, ev *es.EventEnvelope) map[string]types.AttributeValue {
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

func (s *Store) marshalSnapshot(snap *es.SnapshotEnvelope) map[string]types.AttributeValue {
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

func (s *Store) unmarshalEvent(item map[string]types.AttributeValue) (*es.EventEnvelope, error) {
	aidStr, _ := getS(item, colAid)
	id, err := parseAggregateIDKey(aidStr)
	if err != nil {
		return nil, err
	}
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
		AggregateID:   id,
		SeqNr:         seqNr,
		IsCreated:     isCreated,
		// #nosec G115 -- UnixMilli timestamp fits within int64 for any realistic value
		OccurredAt:  time.UnixMilli(int64(occurred)).UTC(),
		Payload:     payload,
		TraceParent: traceparent,
		TraceState:  tracestate,
	}, nil
}

func (s *Store) unmarshalSnapshot(item map[string]types.AttributeValue) (*es.SnapshotEnvelope, error) {
	aidStr, _ := getS(item, colAid)
	id, err := parseAggregateIDKey(aidStr)
	if err != nil {
		return nil, err
	}
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
		AggregateID: id,
		SeqNr:       seqNr,
		Version:     version,
		Payload:     payload,
		// #nosec G115 -- UnixMilli timestamp fits within int64 for any realistic value
		OccurredAt: time.UnixMilli(int64(occurred)).UTC(),
	}, nil
}

// --- low-level helpers ---

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

// parseAggregateIDKey reverses Resolver.ResolveAggregateIDKey == "{TypeName}-{Value}".
// We split on the first dash; aggregate values may legitimately contain dashes
// (e.g., ULIDs), so use the prefix only as type discriminator.
func parseAggregateIDKey(s string) (es.AggregateID, error) {
	for i := 0; i < len(s); i++ {
		if s[i] == '-' {
			return es.NewAggregateID(s[:i], s[i+1:]), nil
		}
	}
	return nil, fmt.Errorf("invalid aggregate id key: %q", s)
}
