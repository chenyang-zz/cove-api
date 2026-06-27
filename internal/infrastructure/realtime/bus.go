package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/boxify/api-go/internal/domain"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

const conversationTopicPrefix = "conversation:"

type Broker interface {
	Publish(ctx context.Context, topic string, event domain.Event) error
	Subscribe(ctx context.Context, topic string) (Subscription, error)
}

type Subscription interface {
	Events() <-chan domain.Event
	Close(ctx context.Context) error
}

type ForwardOptions struct {
	StopEvents map[string]struct{}
}

func ConversationTopic(conversationID uuid.UUID) string {
	return conversationTopicPrefix + conversationID.String()
}

func Forward(ctx context.Context, sub Subscription, out chan<- domain.Event, opts ForwardOptions) error {
	defer close(out)
	defer sub.Close(context.Background())

	stopEvents := opts.StopEvents
	if len(stopEvents) == 0 {
		stopEvents = map[string]struct{}{
			domain.EventTypeDone:  {},
			domain.EventTypeError: {},
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-sub.Events():
			if !ok {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case out <- event:
			}
			if _, stop := stopEvents[event.EventName()]; stop {
				return nil
			}
		}
	}
}

type eventEnvelope struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

type textEventData struct {
	Text string `json:"text"`
}

type metaEventData struct {
	ConversationID uuid.UUID `json:"conversation_id"`
	Title          string    `json:"title"`
}

type errorEventData struct {
	Message string `json:"message"`
}

func MarshalEvent(event domain.Event) ([]byte, error) {
	var data any = map[string]any{}
	switch e := event.(type) {
	case *domain.TextEvent:
		data = textEventData{Text: e.Text}
	case *domain.MetaEvent:
		data = metaEventData{ConversationID: e.ConversationID, Title: e.Title}
	case *domain.ErrorEvent:
		data = errorEventData{Message: e.Message}
	}

	return json.Marshal(struct {
		Event string `json:"event"`
		Data  any    `json:"data"`
	}{
		Event: event.EventName(),
		Data:  data,
	})
}

func UnmarshalEvent(payload []byte) (domain.Event, error) {
	var envelope eventEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, err
	}

	switch envelope.Event {
	case domain.EventTypeToken:
		var data textEventData
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			return nil, err
		}
		return domain.NewTokenEvent(data.Text), nil
	case domain.EventTypeDone:
		var data textEventData
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			return nil, err
		}
		return domain.NewDoneEvent(data.Text), nil
	case domain.EventTypeMeta:
		var data metaEventData
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			return nil, err
		}
		return domain.NewMetaEvent(data.ConversationID, data.Title), nil
	case domain.EventTypeError:
		var data errorEventData
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			return nil, err
		}
		return domain.NewErrorEvent(data.Message), nil
	case domain.EventTypePing:
		return domain.NewPingEvent(), nil
	default:
		return domain.NewBaseEvent(envelope.Event), nil
	}
}

type RedisBroker struct {
	client *goredis.Client
	log    *slog.Logger
}

func NewRedisBroker(client *goredis.Client) *RedisBroker {
	return &RedisBroker{
		client: client,
		log:    slog.Default(),
	}
}

func (b *RedisBroker) Publish(ctx context.Context, topic string, event domain.Event) error {
	payload, err := MarshalEvent(event)
	if err != nil {
		return err
	}
	return b.client.Publish(ctx, topic, payload).Err()
}

func (b *RedisBroker) Subscribe(ctx context.Context, topic string) (Subscription, error) {
	pubsub := b.client.Subscribe(ctx, topic)
	if _, err := pubsub.Receive(ctx); err != nil {
		_ = pubsub.Close()
		return nil, err
	}

	sub := &redisSubscription{
		pubsub: pubsub,
		events: make(chan domain.Event, 16),
		log:    b.log,
	}
	go sub.run(ctx)
	return sub, nil
}

type redisSubscription struct {
	pubsub *goredis.PubSub
	events chan domain.Event
	log    *slog.Logger
	once   sync.Once
}

func (s *redisSubscription) Events() <-chan domain.Event {
	return s.events
}

func (s *redisSubscription) Close(ctx context.Context) error {
	_ = ctx
	var err error
	s.once.Do(func() {
		err = s.pubsub.Close()
	})
	return err
}

func (s *redisSubscription) run(ctx context.Context) {
	defer close(s.events)
	ch := s.pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-ch:
			if !ok {
				return
			}
			event, err := UnmarshalEvent([]byte(message.Payload))
			if err != nil {
				s.log.WarnContext(ctx, "解析实时事件失败", slog.String("error", err.Error()))
				continue
			}
			select {
			case <-ctx.Done():
				return
			case s.events <- event:
			}
		}
	}
}

type MemoryBroker struct {
	mu     sync.Mutex
	topics map[string]map[*memorySubscription]struct{}
}

func NewMemoryBroker() *MemoryBroker {
	return &MemoryBroker{topics: map[string]map[*memorySubscription]struct{}{}}
}

func (b *MemoryBroker) Publish(ctx context.Context, topic string, event domain.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for sub := range b.topics[topic] {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sub.events <- event:
		default:
		}
	}
	return nil
}

func (b *MemoryBroker) Subscribe(ctx context.Context, topic string) (Subscription, error) {
	sub := &memorySubscription{
		broker: b,
		topic:  topic,
		events: make(chan domain.Event, 16),
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if b.topics[topic] == nil {
		b.topics[topic] = map[*memorySubscription]struct{}{}
	}
	b.topics[topic][sub] = struct{}{}
	return sub, nil
}

type memorySubscription struct {
	broker *MemoryBroker
	topic  string
	events chan domain.Event
	once   sync.Once
}

func (s *memorySubscription) Events() <-chan domain.Event {
	return s.events
}

func (s *memorySubscription) Close(ctx context.Context) error {
	_ = ctx
	s.once.Do(func() {
		s.broker.mu.Lock()
		defer s.broker.mu.Unlock()
		delete(s.broker.topics[s.topic], s)
		if len(s.broker.topics[s.topic]) == 0 {
			delete(s.broker.topics, s.topic)
		}
		close(s.events)
	})
	return nil
}
