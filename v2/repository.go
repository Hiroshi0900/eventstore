package eventstore

// Repository[T] は集約 T の高レベルロード/保存 API です。
type Repository[T Aggregate] struct {
	store       EventStore
	createBlank func(AggregateID) T
	serializer  AggregateSerializer[T]
	config      Config
}

// NewRepository は Repository[T] を生成します。
func NewRepository[T Aggregate](
	store EventStore,
	createBlank func(AggregateID) T,
	serializer AggregateSerializer[T],
	config Config,
) *Repository[T] {
	return &Repository[T]{
		store:       store,
		createBlank: createBlank,
		serializer:  serializer,
		config:      config,
	}
}
