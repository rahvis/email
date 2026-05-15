package kumo

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type memoryDeliveryEventStore struct {
	mu             sync.Mutex
	seen           map[string]bool
	applied        []NormalizedDeliveryEvent
	applyFailures  map[string]bool
	deliveryStatus map[int64]string
	suppressed     map[string]int
}

func newMemoryDeliveryEventStore() *memoryDeliveryEventStore {
	return &memoryDeliveryEventStore{
		seen:           map[string]bool{},
		applyFailures:  map[string]bool{},
		deliveryStatus: map[int64]string{},
		suppressed:     map[string]int{},
	}
}

func (m *memoryDeliveryEventStore) IsDuplicateEvent(ctx context.Context, event NormalizedDeliveryEvent) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.seen[m.key(event)], nil
}

func (m *memoryDeliveryEventStore) StoreEvent(ctx context.Context, event NormalizedDeliveryEvent) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := m.key(event)
	if m.seen[key] {
		return false, nil
	}
	m.seen[key] = true
	return true, nil
}

func (m *memoryDeliveryEventStore) ApplyEvent(ctx context.Context, event NormalizedDeliveryEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.applyFailures[event.EventHash] {
		return fmt.Errorf("apply failed")
	}
	current := m.deliveryStatus[event.RecipientInfoID]
	next, changed := NextDeliveryStatus(current, event.DeliveryStatus)
	if changed {
		m.deliveryStatus[event.RecipientInfoID] = next
	}
	if changed && shouldSuppressForEvent(event) {
		m.suppressed[event.Recipient]++
	}
	m.applied = append(m.applied, event)
	return nil
}

func (m *memoryDeliveryEventStore) key(event NormalizedDeliveryEvent) string {
	if event.ProviderEventID != "" {
		return "id:" + event.ProviderEventID
	}
	return "hash:" + event.EventHash
}

func TestNormalizeDeliveryEventsSinglePRDPayload(t *testing.T) {
	body := []byte(`{
		"event_id": "kumo-event-abc",
		"event_type": "delivered",
		"timestamp": 1778600000,
		"message_id": "<1778600000.abc@example.com>",
		"recipient": "user@gmail.com",
		"sender": "news@example.com",
		"queue": "campaign_981:tenant_42@gmail.com",
		"headers": {
			"X-BM-Tenant-ID": "42",
			"X-BM-Campaign-ID": "981",
			"X-BM-Recipient-ID": "12345",
			"X-BM-Message-ID": "<1778600000.abc@example.com>"
		},
		"response": "250 2.0.0 OK",
		"remote_mx": "gmail-smtp-in.l.google.com"
	}`)

	events, failed, err := NormalizeDeliveryEvents(body)
	require.NoError(t, err)
	require.Equal(t, 0, failed)
	require.Len(t, events, 1)

	event := events[0]
	require.Equal(t, "kumo-event-abc", event.ProviderEventID)
	require.Equal(t, "delivered", event.EventType)
	require.Equal(t, DeliveryStatusDelivered, event.DeliveryStatus)
	require.Equal(t, int64(42), event.TenantID)
	require.Equal(t, int64(981), event.CampaignID)
	require.Equal(t, int64(981), event.TaskID)
	require.Equal(t, int64(12345), event.RecipientInfoID)
	require.Equal(t, "1778600000.abc@example.com", event.MessageID)
	require.Equal(t, "campaign_981:tenant_42@gmail.com", event.QueueName)
	require.NotEmpty(t, event.EventHash)
}

func TestNormalizeDeliveryEventsBatchAndKumoLogRecord(t *testing.T) {
	body := []byte(`{
		"events": [
			{
				"type": "Delivery",
				"id": "spool-id-not-provider-event-id",
				"created": 1778600001,
				"recipient": "user@gmail.com",
				"queue": "api_17:tenant_42@gmail.com",
				"headers": {
					"X-BM-Api-ID": "17",
					"X-BM-Api-Log-ID": "77441",
					"X-BM-Message-ID": "<api-message@example.com>"
				},
				"response": {"code": "250", "content": "OK"}
			},
			{"type": "TransientFailure", "message_id": "<defer@example.com>", "recipient": "user@yahoo.com"}
		]
	}`)

	events, failed, err := NormalizeDeliveryEvents(body)
	require.NoError(t, err)
	require.Equal(t, 0, failed)
	require.Len(t, events, 2)

	require.Empty(t, events[0].ProviderEventID, "native Kumo log id is a message spool id, not a unique event id")
	require.Equal(t, DeliveryStatusDelivered, events[0].DeliveryStatus)
	require.Equal(t, int64(17), events[0].APIID)
	require.Equal(t, int64(77441), events[0].APILogID)
	require.Equal(t, int64(42), events[0].TenantID)
	require.Equal(t, "api-message@example.com", events[0].MessageID)
	require.Equal(t, "250 OK", events[0].Response)
	require.Equal(t, DeliveryStatusDeferred, events[1].DeliveryStatus)
}

func TestNormalizeDeliveryEventsStableHashWhenProviderEventIDMissing(t *testing.T) {
	body := []byte(`{"type":"Bounce","message_id":"<m@example.com>","recipient":"u@example.com"}`)
	events, failed, err := NormalizeDeliveryEvents(body)
	require.NoError(t, err)
	require.Equal(t, 0, failed)
	require.Len(t, events, 1)
	require.Empty(t, events[0].ProviderEventID)
	require.NotEmpty(t, events[0].EventHash)

	events2, _, err := NormalizeDeliveryEvents(body)
	require.NoError(t, err)
	require.Equal(t, events[0].EventHash, events2[0].EventHash)
}

func TestIngestNormalizedEventsDedupesAndAppliesOnce(t *testing.T) {
	store := newMemoryDeliveryEventStore()
	restore := setEventStoreForTesting(store)
	defer restore()

	events := []NormalizedDeliveryEvent{
		{ProviderEventID: "evt-1", EventHash: "hash-1", EventType: "delivered", DeliveryStatus: DeliveryStatusDelivered, RecipientInfoID: 11, Recipient: "u@example.com"},
		{ProviderEventID: "evt-1", EventHash: "hash-1", EventType: "delivered", DeliveryStatus: DeliveryStatusDelivered, RecipientInfoID: 11, Recipient: "u@example.com"},
	}
	result, err := IngestNormalizedEvents(context.Background(), events)
	require.NoError(t, err)
	require.Equal(t, &WebhookIngestResult{Accepted: 1, Duplicates: 1}, result)
	require.Len(t, store.applied, 1)
	require.Equal(t, DeliveryStatusDelivered, store.deliveryStatus[11])
}

func TestNextDeliveryStatusHandlesOutOfOrderEvents(t *testing.T) {
	next, changed := NextDeliveryStatus(DeliveryStatusDelivered, DeliveryStatusDeferred)
	require.False(t, changed)
	require.Equal(t, DeliveryStatusDelivered, next)

	next, changed = NextDeliveryStatus(DeliveryStatusDeferred, DeliveryStatusBounced)
	require.True(t, changed)
	require.Equal(t, DeliveryStatusBounced, next)

	next, changed = NextDeliveryStatus(DeliveryStatusDelivered, DeliveryStatusComplained)
	require.True(t, changed)
	require.Equal(t, DeliveryStatusComplained, next)
}

func TestTenantConflictOnlyBlocksKnownMismatches(t *testing.T) {
	require.False(t, tenantConflict(0, 42))
	require.False(t, tenantConflict(42, 0))
	require.False(t, tenantConflict(42, 42))
	require.True(t, tenantConflict(42, 88))
}

func TestBounceEventSuppressesRecipientWithoutDoubleCount(t *testing.T) {
	store := newMemoryDeliveryEventStore()
	restore := setEventStoreForTesting(store)
	defer restore()

	event := NormalizedDeliveryEvent{
		ProviderEventID: "bounce-1",
		EventHash:       "bounce-hash",
		EventType:       "Bounce",
		DeliveryStatus:  DeliveryStatusBounced,
		RecipientInfoID: 99,
		Recipient:       "bad@example.com",
	}
	result, err := IngestNormalizedEvents(context.Background(), []NormalizedDeliveryEvent{event, event})
	require.NoError(t, err)
	require.Equal(t, 1, result.Accepted)
	require.Equal(t, 1, result.Duplicates)
	require.Equal(t, 1, store.suppressed["bad@example.com"])
}

func TestSecondBounceEventDoesNotDoubleSuppressAfterTerminalState(t *testing.T) {
	store := newMemoryDeliveryEventStore()
	restore := setEventStoreForTesting(store)
	defer restore()

	first := NormalizedDeliveryEvent{
		ProviderEventID: "bounce-1",
		EventHash:       "bounce-hash-1",
		EventType:       "Bounce",
		DeliveryStatus:  DeliveryStatusBounced,
		RecipientInfoID: 99,
		Recipient:       "bad@example.com",
	}
	second := first
	second.ProviderEventID = "bounce-2"
	second.EventHash = "bounce-hash-2"

	result, err := IngestNormalizedEvents(context.Background(), []NormalizedDeliveryEvent{first, second})
	require.NoError(t, err)
	require.Equal(t, 2, result.Accepted)
	require.Equal(t, 1, store.suppressed["bad@example.com"])
}

func TestPartialBatchReportsAcceptedDuplicateAndFailed(t *testing.T) {
	store := newMemoryDeliveryEventStore()
	restore := setEventStoreForTesting(store)
	defer restore()

	bodyMap := map[string]interface{}{
		"events": []interface{}{
			map[string]interface{}{"event_id": "ok-1", "event_type": "delivered", "message_id": "<ok@example.com>", "recipient": "ok@example.com", "headers": map[string]interface{}{"X-BM-Recipient-ID": "1"}},
			map[string]interface{}{"event_id": "dup-1", "event_type": "delivered", "message_id": "<dup@example.com>", "recipient": "dup@example.com", "headers": map[string]interface{}{"X-BM-Recipient-ID": "2"}},
			"not-an-event",
		},
	}
	body, err := json.Marshal(bodyMap)
	require.NoError(t, err)

	events, failed, err := NormalizeDeliveryEvents(body)
	require.NoError(t, err)
	require.Equal(t, 1, failed)
	require.Len(t, events, 2)

	store.seen["id:dup-1"] = true
	result, err := IngestNormalizedEvents(context.Background(), events)
	require.NoError(t, err)
	result.Failed += failed

	require.Equal(t, 1, result.Accepted)
	require.Equal(t, 1, result.Duplicates)
	require.Equal(t, 1, result.Failed)
}

func TestUnknownMessageEventNormalizesForOrphanDiagnostics(t *testing.T) {
	events, failed, err := NormalizeDeliveryEvents([]byte(`{"type":"Bounce","response":"550 no such user"}`))
	require.NoError(t, err)
	require.Equal(t, 0, failed)
	require.Len(t, events, 1)
	require.Equal(t, DeliveryStatusBounced, events[0].DeliveryStatus)
	require.Empty(t, events[0].MessageID)
	require.Empty(t, events[0].Recipient)
	require.NotEmpty(t, events[0].EventHash)
}
