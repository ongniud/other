package alertmanager

import (
	"context"
	"fmt"
	"time"
)

// 状态和事件定义
const (
	EventTrigger = "trigger"
	EventRecover = "recover"
	EventFire    = "fire"
	EventResolve = "resolve"
)

type IFsm interface {
	Transition(ctx context.Context, active bool, ts time.Time, opts *AlertOpts) (bool, error)
	Snapshot() AlertSnapshot
	Restore(snap AlertSnapshot) error
	State() AlertState
}

func NewFsm(typ AlertType) (IFsm, error) {
	switch typ {
	case AlertTypeBasic:
		return NewPromAlertFsm(), nil
	case AlertTypeMultiTier:
		return NewDegradeFsm(), nil
	default:
		return nil, fmt.Errorf("unsupported alert type: %s", typ)
	}
}
