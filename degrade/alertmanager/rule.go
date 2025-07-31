package alertmanager

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/prometheus/prometheus/model/labels"
)

type AlertOpts struct {
	// for normal
	HoldDuration  time.Duration
	KeepFiringFor time.Duration
	ResendDelay   time.Duration

	// for degrade
	RecoverDuration  time.Duration // 恢复确认时间
	AutoRecoverAfter time.Duration // 自动恢复时间
}

type Rule struct {
	Name      string
	Expr      string
	AlertType AlertType
	AlertOpts *AlertOpts

	Labels      labels.Labels
	Annotations labels.Labels

	mtx    sync.RWMutex
	active map[uint64]IAlert
}

func NewRule(
	name, expr string,
	hold, keepFiring, resendDelay time.Duration,
	lbs, ann labels.Labels,
) (*Rule, error) {
	if name == "" || expr == "" {
		return nil, errors.New("empty name or expr")
	}
	if hold < 0 || keepFiring < 0 || resendDelay < 0 {
		return nil, errors.New("durations cannot be negative")
	}
	return &Rule{
		Name: name,
		Expr: expr,
		AlertOpts: &AlertOpts{
			HoldDuration:  hold,
			KeepFiringFor: keepFiring,
			ResendDelay:   resendDelay,
		},
		Labels:      lbs,
		Annotations: ann,
		active:      make(map[uint64]IAlert),
	}, nil
}

func (r *Rule) newAlert(lbs labels.Labels) (IAlert, error) {
	return NewAlert(r.AlertType, lbs, r.AlertOpts)
}

func (r *Rule) Eval(
	ctx context.Context,
	ts time.Time,
	query QueryFunc,
) ([]IAlert, error) {
	vector, err := query(ctx, r.Expr, ts)
	if err != nil {
		return nil, err
	}

	r.mtx.Lock()
	defer r.mtx.Unlock()

	activeFPs := make(map[uint64]struct{}, len(vector))
	var firingAlerts []IAlert

	for _, sample := range vector {
		lbs := r.formatLabels(sample.Metric)
		fp := lbs.Hash()
		activeFPs[fp] = struct{}{}

		alert, exists := r.active[fp]
		if !exists {
			alert, err = r.newAlert(lbs)
			if err != nil {
				return nil, err
			}
			r.active[fp] = alert
		}

		alert.SetValue(sample.F)

		shouldSend, err := alert.Transition(ctx, true, ts)
		if err != nil {
			log.Printf("alert transition failed: %v\n", err)
			continue
		}
		if shouldSend {
			firingAlerts = append(firingAlerts, alert)
		}
	}

	// 清理非活跃告警
	for fp, alert := range r.active {
		if _, active := activeFPs[fp]; !active {
			shouldSend, _ := alert.Transition(ctx, false, ts)
			if err != nil {
				log.Printf("alert transition failed: %v\n", err)
				continue
			}
			if shouldSend {
				firingAlerts = append(firingAlerts, alert)
				delete(r.active, fp)
			}
		}
	}

	return firingAlerts, nil
}

func (r *Rule) formatLabels(sampleLabels labels.Labels) labels.Labels {
	builder := labels.NewBuilder(sampleLabels)
	r.Labels.Range(func(l labels.Label) {
		if builder.Get(l.Name) == "" {
			builder.Set(l.Name, l.Value)
		}
	})
	builder.Del(labels.MetricName)
	builder.Set(labels.AlertName, r.Name)
	return builder.Labels()
}
