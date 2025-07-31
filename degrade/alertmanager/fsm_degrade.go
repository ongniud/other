package alertmanager

import (
	"context"
	"log"
	"time"

	"github.com/looplab/fsm"
)

// DegradeFsm 多级降级状态机
type DegradeFsm struct {
	fsm *fsm.FSM

	// 状态时间记录
	stateEnteredAt map[AlertState]time.Time
	lastSentAt     time.Time

	// 状态机配置
	events    fsm.Events
	callbacks fsm.Callbacks
}

// NewDegradeFsm 创建新的多级降级状态机
func NewDegradeFsm() *DegradeFsm {
	d := &DegradeFsm{
		stateEnteredAt: make(map[AlertState]time.Time),
	}

	// 初始化状态时间记录
	d.stateEnteredAt[AlertStateL0] = time.Now()

	// 定义状态转移规则
	d.events = fsm.Events{
		// 降级路径
		{Name: EventTrigger, Src: []string{string(AlertStateL0)}, Dst: string(AlertStateL1)},
		{Name: EventTrigger, Src: []string{string(AlertStateL1)}, Dst: string(AlertStateL2)},
		{Name: EventTrigger, Src: []string{string(AlertStateL2)}, Dst: string(AlertStateL3)},

		// 恢复路径
		{Name: EventRecover, Src: []string{string(AlertStateL3)}, Dst: string(AlertStateL2)},
		{Name: EventRecover, Src: []string{string(AlertStateL2)}, Dst: string(AlertStateL1)},
		{Name: EventRecover, Src: []string{string(AlertStateL1)}, Dst: string(AlertStateL0)},

		// 完全恢复（可从任何状态直接回到L0）
		{Name: EventResolve, Src: []string{string(AlertStateL1), string(AlertStateL2), string(AlertStateL3)}, Dst: string(AlertStateL0)},
	}

	// 状态进入回调
	d.callbacks = fsm.Callbacks{
		"enter_state": func(_ context.Context, e *fsm.Event) {
			newState := AlertState(e.Dst)
			d.stateEnteredAt[newState] = time.Now()
			log.Printf("[DegradeFsm] Entered state %s at %v", newState, d.stateEnteredAt[newState])
		},
	}

	d.fsm = fsm.NewFSM(
		string(AlertStateL0),
		d.events,
		d.callbacks,
	)

	return d
}

// Transition 状态转移方法
// active: true表示触发降级条件，false表示恢复正常条件
// ts: 当前时间戳
func (d *DegradeFsm) Transition(ctx context.Context, active bool, ts time.Time, opts *AlertOpts) (bool, error) {
	state := AlertState(d.fsm.Current())

	log.Printf("[DegradeFsm] Transition - current: %s, active: %v, time: %v", state, active, ts.Format(time.RFC3339))

	switch {
	case active:
		// 触发降级条件，尝试降级
		return d.handleDegradation(ctx, state, ts, opts)
	case !active && state != AlertStateL0:
		// 恢复正常条件，尝试恢复
		return d.handleRecovery(ctx, state, ts, opts)
	default:
		// 已经是L0状态且active=false，无需处理
		log.Printf("[DegradeFsm] Already in L0 with no degradation")
		return false, nil
	}
}

// handleDegradation 处理降级逻辑
func (d *DegradeFsm) handleDegradation(ctx context.Context, current AlertState, ts time.Time, opts *AlertOpts) (bool, error) {
	// 检查是否已经处于最高级降级
	if current == AlertStateL3 {
		log.Printf("[DegradeFsm] Already at maximum degradation level (L3)")
		return d.checkResend(ts, opts), nil
	}

	// 检查是否满足降级确认时间
	timeInState := ts.Sub(d.stateEnteredAt[current])
	if timeInState < opts.HoldDuration {
		log.Printf("[DegradeFsm] Hold duration not met: %v < %v (remaining: %v)",
			timeInState, opts.HoldDuration, opts.HoldDuration-timeInState)
		return false, nil
	}

	// 执行降级
	if err := d.fsm.Event(ctx, EventTrigger); err != nil {
		log.Printf("[DegradeFsm] Degrade error: %v", err)
		return false, err
	}

	d.lastSentAt = ts
	log.Printf("[DegradeFsm] lastSentAt: %v", d.lastSentAt.Format(time.RFC3339))
	return true, nil
}

// handleRecovery 处理恢复逻辑
func (d *DegradeFsm) handleRecovery(ctx context.Context, current AlertState, ts time.Time, opts *AlertOpts) (bool, error) {
	// 检查自动恢复条件
	if opts.AutoRecoverAfter > 0 {
		timeInState := ts.Sub(d.stateEnteredAt[current])
		if timeInState >= opts.AutoRecoverAfter {
			log.Printf("[DegradeFsm] Auto-recover duration (%v) met, resolving to L0", opts.AutoRecoverAfter)
			if err := d.fsm.Event(ctx, EventResolve); err != nil {
				log.Printf("[DegradeFsm] Resolve error: %v", err)
				return false, err
			}
			d.lastSentAt = ts
			log.Printf("[DegradeFsm] Auto-resolved to L0")
			return true, nil
		}
	}

	// 检查恢复确认时间
	timeInState := ts.Sub(d.stateEnteredAt[current])
	if timeInState < opts.RecoverDuration {
		log.Printf("[DegradeFsm] Recover duration not met: %v < %v (remaining: %v)",
			timeInState, opts.RecoverDuration, opts.RecoverDuration-timeInState)
		return false, nil
	}

	// 执行恢复
	if err := d.fsm.Event(ctx, EventRecover); err != nil {
		log.Printf("[DegradeFsm] Recover error: %v", err)
		return false, err
	}

	d.lastSentAt = ts
	log.Printf("[DegradeFsm] Recovered, lastSentAt: %v", d.lastSentAt.Format(time.RFC3339))
	return true, nil
}

// checkResend 检查是否需要重发通知
func (d *DegradeFsm) checkResend(ts time.Time, opts *AlertOpts) bool {
	if opts.ResendDelay == 0 {
		return false
	}

	elapsed := ts.Sub(d.lastSentAt)
	if elapsed >= opts.ResendDelay {
		d.lastSentAt = ts
		log.Printf("[DegradeFsm] Resend delay (%v) met, resending notification", opts.ResendDelay)
		return true
	}

	log.Printf("[DegradeFsm] Resend delay not met: %v/%v (remaining: %v)",
		elapsed, opts.ResendDelay, opts.ResendDelay-elapsed)
	return false
}

// CurrentState 获取当前状态
func (d *DegradeFsm) State() AlertState {
	return AlertState(d.fsm.Current())
}

// Snapshot 状态快照
func (d *DegradeFsm) Snapshot() AlertSnapshot {
	return AlertSnapshot{
		State:          string(d.State()),
		StateEnteredAt: d.stateEnteredAt,
		LastSentAt:     d.lastSentAt,
	}
}

// Restore 恢复状态
func (d *DegradeFsm) Restore(snap AlertSnapshot) error {
	d.stateEnteredAt = snap.StateEnteredAt
	d.lastSentAt = snap.LastSentAt

	d.fsm = fsm.NewFSM(
		snap.State,
		d.events,
		d.callbacks,
	)
	return nil
}
