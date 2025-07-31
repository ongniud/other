package alertmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Notification 表示发送的告警通知
type Notification struct {
	Rule     string            `json:"rule"`
	Status   string            `json:"status"`
	Labels   map[string]string `json:"labels"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Value    float64           `json:"value"`
	StartsAt time.Time         `json:"startsAt"`
	EndsAt   time.Time         `json:"endsAt"`
}

func NewNotification(r *Rule, alert IAlert) *Notification {
	snap := alert.Snapshot()
	n := &Notification{
		Rule:     r.Name,
		Status:   string(snap.State),
		Labels:   alert.Labels().Map(),
		StartsAt: snap.FiredAt,
		Value:    alert.GetValue(),
	}
	// 对于已解决的告警，设置结束时间
	if AlertState(snap.State) == AlertStateInactive && !snap.FiredAt.IsZero() {
		n.EndsAt = time.Now()
	}
	return n
}

// Notifier 定义通知器接口
type Notifier interface {
	Notify(ctx context.Context, notifications []*Notification) error
}

type PrintNotifier struct{}

func NewPrintNotifier() *PrintNotifier {
	return &PrintNotifier{}
}

func (p *PrintNotifier) Notify(ctx context.Context, notifications []*Notification) error {
	for _, n := range notifications {
		data, err := json.MarshalIndent(n, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal notification: %w", err)
		}
		fmt.Println(string(data))
	}
	return nil
}
