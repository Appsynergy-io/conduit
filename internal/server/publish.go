package server

import "context"

// publishEvent sends an event to both the WebSocket EventBus (real-time browser push)
// and the webhook delivery engine (HTTP callbacks to subscribers).
func (s *Server) publishEvent(ctx context.Context, tenantID string, event Event) {
	if s.eventBus != nil {
		s.eventBus.Publish(tenantID, event)
	}
	if s.webhooks != nil {
		s.webhooks.Enqueue(ctx, tenantID, event.Type, event.Data)
	}
}
