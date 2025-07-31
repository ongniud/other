package alertmanager

import (
	"context"
	"log"
	"time"

	"github.com/looplab/fsm"
)

// PromAlertFsm 线程安全的状态机实现
type PromAlertFsm struct {
	fsm *fsm.FSM

	activeAt   time.Time
	firedAt    time.Time
	lastSentAt time.Time

	events    fsm.Events
	callbacks fsm.Callbacks
}

func NewPromAlertFsm() *PromAlertFsm {
	a := &PromAlertFsm{}
	a.events = fsm.Events{
		{Name: EventTrigger, Src: []string{string(AlertStateInactive)}, Dst: string(AlertStatePending)},
		{Name: EventFire, Src: []string{string(AlertStatePending), string(AlertStateInactive)}, Dst: string(AlertStateFiring)},
		{Name: EventResolve, Src: []string{string(AlertStatePending), string(AlertStateFiring)}, Dst: string(AlertStateInactive)},
	}
	a.callbacks = fsm.Callbacks{
		"enter_pending": func(_ context.Context, e *fsm.Event) { a.activeAt = time.Now() },
		"enter_firing":  func(_ context.Context, e *fsm.Event) { a.firedAt = time.Now() },
		"enter_inactive": func(_ context.Context, e *fsm.Event) {
			if e.Src == string(AlertStateFiring) {
				a.firedAt = time.Time{}
			}
		},
	}
	a.fsm = fsm.NewFSM(
		string(AlertStateInactive),
		a.events,
		a.callbacks,
	)
	return a
}

func (a *PromAlertFsm) State() AlertState {
	return AlertState(a.fsm.Current())
}

func (a *PromAlertFsm) Transition(ctx context.Context, active bool, ts time.Time, opts *AlertOpts) (bool, error) {
	current := AlertState(a.fsm.Current())

	// 记录初始状态和输入参数
	log.Printf("[StateMachine] Transition - current state: %s, active: %v, timestamp: %v, hold: %v, keepFiring: %v, resendDelay: %v",
		current, active, ts.Format(time.RFC3339), opts.HoldDuration, opts.KeepFiringFor, opts.ResendDelay)

	switch {
	case !active && current != AlertStateInactive:
		log.Printf("[StateMachine] Resolving alert from state %s", current)
		if err := a.fsm.Event(ctx, EventResolve); err != nil {
			log.Printf("[StateMachine] Error resolving alert: %v", err)
			return false, err
		}
		log.Printf("[StateMachine] Successfully resolved to inactive state")
		return true, nil

	case active && current == AlertStateInactive:
		if opts.HoldDuration == 0 {
			log.Printf("[StateMachine] Immediate firing (hold=0)")
			if err := a.fsm.Event(ctx, EventFire); err != nil {
				log.Printf("[StateMachine] Error firing alert: %v", err)
				return false, err
			}
			a.lastSentAt = ts
			log.Printf("[StateMachine] Successfully fired immediately")
			return true, nil
		}
		log.Printf("[StateMachine] Triggering alert (will enter pending state)")
		if err := a.fsm.Event(ctx, EventTrigger); err != nil {
			log.Printf("[StateMachine] Error triggering alert: %v", err)
			return false, err
		}
		log.Printf("[StateMachine] Successfully triggered (now pending)")
		return false, nil

	case active && current == AlertStatePending:
		duration := ts.Sub(a.activeAt)
		if duration < opts.HoldDuration {
			log.Printf("[StateMachine] Hold duration not met: %v < %v (remaining: %v)",
				duration, opts.HoldDuration, opts.HoldDuration-duration)
			return false, nil
		}
		log.Printf("[StateMachine] Hold duration met, firing alert")
		if err := a.fsm.Event(ctx, EventFire); err != nil {
			log.Printf("[StateMachine] Error firing from pending state: %v", err)
			return false, err
		}
		a.lastSentAt = ts
		log.Printf("[StateMachine] Successfully fired (now firing), lastSentAt: %v", a.lastSentAt.Format(time.RFC3339))
		return true, nil

	case active && current == AlertStateFiring:
		if opts.KeepFiringFor > 0 {
			duration := ts.Sub(a.firedAt)
			if duration >= opts.KeepFiringFor {
				log.Printf("[StateMachine] KeepFiring duration (%v) met, auto-resolving alert", opts.KeepFiringFor)
				if err := a.fsm.Event(ctx, EventResolve); err != nil {
					log.Printf("[StateMachine] Error auto-resolving alert: %v", err)
					return false, err
				}
				log.Printf("[StateMachine] Successfully auto-resolved (now inactive)")
				return true, nil
			}
			log.Printf("[StateMachine] KeepFiring duration not met: %v/%v (remaining: %v)",
				duration, opts.KeepFiringFor, opts.KeepFiringFor-duration)
		}

		duration := ts.Sub(a.lastSentAt)
		if opts.ResendDelay > 0 && duration >= opts.ResendDelay {
			log.Printf("[StateMachine] Resend delay (%v) met, resending notification", opts.ResendDelay)
			a.lastSentAt = ts
			log.Printf("[StateMachine] Notification resent, lastSentAt updated to: %v", a.lastSentAt.Format(time.RFC3339))
			return true, nil
		}
		log.Printf("[StateMachine] Resend delay not met: %v/%v (remaining: %v)",
			duration, opts.ResendDelay, opts.ResendDelay-duration)
	}

	log.Printf("[StateMachine] No state transition occurred")
	return false, nil
}

func (a *PromAlertFsm) Snapshot() AlertSnapshot {
	return AlertSnapshot{
		State:      a.fsm.Current(),
		ActiveAt:   a.activeAt,
		FiredAt:    a.firedAt,
		LastSentAt: a.lastSentAt,
	}
}

func (a *PromAlertFsm) Restore(snap AlertSnapshot) error {
	a.activeAt = snap.ActiveAt
	a.firedAt = snap.FiredAt
	a.lastSentAt = snap.LastSentAt
	// FSM需要重建以保证状态一致性
	a.fsm = fsm.NewFSM(
		snap.State,
		a.events,
		a.callbacks,
	)
	return nil
}
