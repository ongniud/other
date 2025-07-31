package alertmanager

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/prometheus/prometheus/promql"
)

type QueryFunc func(ctx context.Context, query string, ts time.Time) (promql.Vector, error)

// AlertManager 管理告警规则的评估和通知
type AlertManager struct {
	rules    []*Rule
	interval time.Duration
	queryFn  QueryFunc
	notifier Notifier
	storage  Storage
	stop     chan struct{}
	wg       sync.WaitGroup
	mtx      sync.RWMutex
}

// NewAlertManager 创建新的AlertManager实例
func NewAlertManager(
	rules []*Rule,
	interval time.Duration,
	queryFn QueryFunc,
	notifier Notifier,
	storage Storage,
) *AlertManager {
	return &AlertManager{
		rules:    rules,
		interval: interval,
		queryFn:  queryFn,
		notifier: notifier,
		storage:  storage,
		stop:     make(chan struct{}),
	}
}

// Run 启动AlertManager的主循环
func (am *AlertManager) Run() error {
	// 从存储加载告警状态
	if err := am.restoreAlerts(); err != nil {
		return fmt.Errorf("failed to restore alerts: %v", err)
	}

	// 启动主循环
	am.wg.Add(1)
	go am.loop()

	log.Println("AlertManager started")
	return nil
}

// Stop 停止AlertManager
func (am *AlertManager) Stop() {
	close(am.stop)
	am.wg.Wait()

	// 保存当前告警状态
	if err := am.saveAlerts(); err != nil {
		log.Printf("Failed to save alerts: %v", err)
	}

	log.Println("AlertManager stopped")
}

// loop 主循环
func (am *AlertManager) loop() {
	defer am.wg.Done()

	ticker := time.NewTicker(am.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			am.evaluateAllRules()
		case <-am.stop:
			return
		}
	}
}

// evaluateAllRules 评估所有规则
func (am *AlertManager) evaluateAllRules() {
	am.mtx.RLock()
	defer am.mtx.RUnlock()

	now := time.Now()

	for _, rule := range am.rules {
		am.wg.Add(1)
		go func(r *Rule) {
			defer am.wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), am.interval)
			defer cancel()

			firingAlerts, err := r.Eval(ctx, now, am.queryFn)
			if err != nil {
				log.Printf("Error evaluating rule %s: %v", r.Name, err)
				return
			}
			if len(firingAlerts) > 0 {
				notifications := make([]*Notification, 0, len(firingAlerts))
				for _, alert := range firingAlerts {
					notifications = append(notifications, NewNotification(r, alert))
				}
				if err := am.notifier.Notify(context.Background(), notifications); err != nil {
					log.Printf("Error sending alerts for rule %s: %v", r.Name, err)
				}
			}
		}(rule)
	}
}

// restoreAlerts 从存储恢复告警状态
func (am *AlertManager) restoreAlerts() error {
	am.mtx.Lock()
	defer am.mtx.Unlock()
	for _, rule := range am.rules {
		alerts, err := am.storage.LoadAlerts(rule)
		if err != nil {
			return fmt.Errorf("failed to load alerts for rule %s: %v", rule.Name, err)
		}
		rule.active = make(map[uint64]IAlert)
		for _, alert := range alerts {
			rule.active[alert.Labels().Hash()] = alert
		}
	}
	return nil
}

// saveAlerts 保存当前告警状态到存储
func (am *AlertManager) saveAlerts() error {
	am.mtx.RLock()
	defer am.mtx.RUnlock()
	for _, rule := range am.rules {
		var alerts []IAlert
		for _, alert := range rule.active {
			alerts = append(alerts, alert)
		}
		if err := am.storage.SaveAlerts(rule, alerts); err != nil {
			return fmt.Errorf("failed to save alerts for rule %s: %v", rule.Name, err)
		}
	}
	return nil
}

// AddRule 添加新规则
func (am *AlertManager) AddRule(rule *Rule) error {
	am.mtx.Lock()
	defer am.mtx.Unlock()

	// 检查规则是否已存在
	for _, r := range am.rules {
		if r.Name == rule.Name {
			return errors.New("rule already exists")
		}
	}

	am.rules = append(am.rules, rule)
	return nil
}

// RemoveRule 移除规则
func (am *AlertManager) RemoveRule(name string) error {
	am.mtx.Lock()
	defer am.mtx.Unlock()

	for i, r := range am.rules {
		if r.Name == name {
			// 从存储中删除该规则的告警状态
			if err := am.storage.SaveAlerts(r, nil); err != nil {
				return fmt.Errorf("failed to clear alerts for rule %s: %v", name, err)
			}
			// 从规则列表中移除
			am.rules = append(am.rules[:i], am.rules[i+1:]...)
			return nil
		}
	}

	return errors.New("rule not found")
}
