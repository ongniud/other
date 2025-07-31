package alertmanager

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/prometheus/prometheus/model/labels"
)

type AlertType string

const (
	AlertTypeBasic     AlertType = "basic"
	AlertTypeMultiTier AlertType = "multi-tier"
	AlertTypeCustom    AlertType = "custom"
)

type AlertState string

const (
	AlertStateInactive AlertState = "inactive"
	AlertStatePending  AlertState = "pending"
	AlertStateFiring   AlertState = "firing"

	AlertStateL0 AlertState = "l0"
	AlertStateL1 AlertState = "l1"
	AlertStateL2 AlertState = "l2"
	AlertStateL3 AlertState = "l3"
)

// AlertSnapshot 用于状态持久化
type AlertSnapshot struct {
	State          string                   `json:"state"`
	ActiveAt       time.Time                `json:"activeAt"`
	FiredAt        time.Time                `json:"firedAt"`
	LastSentAt     time.Time                `json:"lastSentAt"`
	StateEnteredAt map[AlertState]time.Time `json:"stateEnteredAt"`
}

type IAlert interface {
	Labels() labels.Labels

	State() AlertState
	Snapshot() AlertSnapshot
	Transition(ctx context.Context, firing bool, now time.Time) (shouldNotify bool, err error)

	SetValue(v float64)
	GetValue() float64

	Marshal() ([]byte, error)
	Restore(data []byte, opt *AlertOpts) error
}

type Alert struct {
	labels labels.Labels
	typ    AlertType
	opt    *AlertOpts

	mtx   sync.RWMutex
	Value float64
	fsm   IFsm
}

func NewAlert(typ AlertType, lbs labels.Labels, opt *AlertOpts) (*Alert, error) {
	fsm, err := NewFsm(typ)
	if err != nil {
		return nil, err
	}
	return &Alert{
		labels: lbs,
		typ:    typ,
		opt:    opt,
		fsm:    fsm,
	}, nil
}

func (a *Alert) Transition(ctx context.Context, active bool, ts time.Time) (bool, error) {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	return a.fsm.Transition(ctx, active, ts, a.opt)
}

func (a *Alert) State() AlertState {
	a.mtx.RLock()
	defer a.mtx.RUnlock()
	return AlertState(a.fsm.State())
}

func (a *Alert) SetValue(v float64) {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	a.Value = v
}

func (a *Alert) GetValue() float64 {
	a.mtx.RLock()
	defer a.mtx.RUnlock()
	return a.Value
}

func (a *Alert) Labels() labels.Labels {
	a.mtx.RLock()
	defer a.mtx.RUnlock()
	return a.labels
}

func (a *Alert) Snapshot() AlertSnapshot {
	a.mtx.RLock()
	defer a.mtx.RUnlock()
	return a.fsm.Snapshot()
}

type alertPersisted struct {
	Labels   labels.Labels `json:"labels"`
	Value    float64       `json:"value"`
	Snapshot AlertSnapshot `json:"machine"`
	Typ      AlertType     `json:"type"`
}

func (a *Alert) Marshal() ([]byte, error) {
	a.mtx.RLock()
	defer a.mtx.RUnlock()
	persisted := alertPersisted{
		Labels:   a.labels,
		Value:    a.Value,
		Typ:      a.typ,
		Snapshot: a.fsm.Snapshot(),
	}
	return json.Marshal(persisted)
}

func (a *Alert) Restore(data []byte, opt *AlertOpts) error {
	var persisted alertPersisted
	if err := json.Unmarshal(data, &persisted); err != nil {
		return err
	}
	a.labels = persisted.Labels
	a.Value = persisted.Value
	a.typ = persisted.Typ
	a.opt = opt

	fsm, err := NewFsm(persisted.Typ)
	if err != nil {
		return err
	}
	if err := fsm.Restore(persisted.Snapshot); err != nil {
		return err
	}
	a.fsm = fsm
	return nil
}
