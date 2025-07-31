package alertmanager

import (
	"context"
	"encoding/json"
	"github.com/prometheus/prometheus/model/labels"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAlert_Transition_InactiveToPending(t *testing.T) {
	opts := &AlertOpts{
		HoldDuration: 1 * time.Minute,
	}
	alert, _ := NewAlert(AlertTypeBasic, labels.FromStrings("instance", "test1"), opts)

	shouldSend, err := alert.Transition(context.Background(), true, time.Now())
	require.NoError(t, err)
	require.False(t, shouldSend, "should not send notification on initial trigger")
	require.Equal(t, AlertStatePending, alert.State(), "should transition to pending machine")
}

func TestAlert_Transition_InactiveToFiring_NoHold(t *testing.T) {
	opts := &AlertOpts{
		HoldDuration: 0, // No hold duration
	}
	alert, _ := NewAlert(AlertTypeBasic, labels.FromStrings("alertname", "TestAlert"), opts)

	// Should transition directly to firing
	shouldSend, err := alert.Transition(context.Background(), true, time.Now())
	require.NoError(t, err)
	require.True(t, shouldSend, "should send notification when firing immediately")
	require.Equal(t, AlertStateFiring, alert.State())
}

func TestAlert_Transition_PendingToFiring(t *testing.T) {
	opts := &AlertOpts{
		HoldDuration: 1 * time.Minute,
	}
	alert, _ := NewAlert(AlertTypeBasic, labels.FromStrings("instance", "test1"), opts)

	// First transition to pending
	_, _ = alert.Transition(context.Background(), true, time.Now())

	// Then transition to firing after hold duration
	shouldSend, err := alert.Transition(context.Background(), true, time.Now().Add(2*time.Minute))
	require.NoError(t, err)
	require.True(t, shouldSend, "should send notification when firing")
	require.Equal(t, AlertStateFiring, alert.State(), "should transition to firing machine")
}

func TestAlert_Transition_PendingToFiring_AfterHold(t *testing.T) {
	holdDuration := 1 * time.Minute
	opts := &AlertOpts{
		HoldDuration: holdDuration,
	}
	alert, _ := NewAlert(AlertTypeBasic, labels.FromStrings("alertname", "TestAlert"), opts)

	// Transition to pending
	startTime := time.Now()
	_, err := alert.Transition(context.Background(), true, startTime)
	require.NoError(t, err)

	// Try transition before hold duration (should stay pending)
	shouldSend, err := alert.Transition(context.Background(), true, startTime.Add(holdDuration/2))
	require.NoError(t, err)
	require.False(t, shouldSend)
	require.Equal(t, AlertStatePending, alert.State())

	// After hold duration (should fire)
	shouldSend, err = alert.Transition(context.Background(), true, startTime.Add(holdDuration+time.Second))
	require.NoError(t, err)
	require.True(t, shouldSend)
	require.Equal(t, AlertStateFiring, alert.State())
}

func TestAlert_Transition_PendingToInactive_WhenResolved(t *testing.T) {
	opts := &AlertOpts{HoldDuration: 1 * time.Minute}
	alert, _ := NewAlert(AlertTypeBasic, labels.FromStrings("alertname", "TestAlert"), opts)
	// Transition to pending
	_, err := alert.Transition(context.Background(), true, time.Now())
	require.NoError(t, err)

	// Resolve while in pending machine
	shouldSend, err := alert.Transition(context.Background(), false, time.Now())
	require.NoError(t, err)
	require.True(t, shouldSend, "should send notification when resolved from pending")
	require.Equal(t, AlertStateInactive, alert.State())
}

func TestAlert_Transition_Firing_KeepFiring(t *testing.T) {
	keepFiring := 5 * time.Minute
	opts := &AlertOpts{HoldDuration: 0, KeepFiringFor: keepFiring}
	alert, _ := NewAlert(AlertTypeBasic, labels.FromStrings("alertname", "TestAlert"), opts)

	// Transition to firing
	fireTime := time.Now()
	_, err := alert.Transition(context.Background(), true, fireTime)
	require.NoError(t, err)

	// Before keepFiring duration
	shouldSend, err := alert.Transition(context.Background(), true, fireTime.Add(keepFiring/2))
	require.NoError(t, err)
	require.False(t, shouldSend, "should not send notification if not time to resend yet")
	require.Equal(t, AlertStateFiring, alert.State())

	// After keepFiring duration (should auto-resolve)
	shouldSend, err = alert.Transition(context.Background(), true, fireTime.Add(keepFiring+time.Second))
	require.NoError(t, err)
	require.True(t, shouldSend, "should send notification when auto-resolving")
	require.Equal(t, AlertStateInactive, alert.State())
}

func TestAlert_Transition_FiringToInactive_WhenResolved1(t *testing.T) {
	opts := &AlertOpts{
		HoldDuration:  1 * time.Minute,
		KeepFiringFor: 10 * time.Minute,
		ResendDelay:   5 * time.Minute,
	}
	alert, _ := NewAlert(AlertTypeBasic, labels.FromStrings("instance", "test1"), opts)

	// Transition to pending
	_, _ = alert.Transition(context.Background(), true, time.Now())
	// Transition to firing
	_, _ = alert.Transition(context.Background(), true, time.Now().Add(2*time.Minute))

	// Then resolve
	shouldSend, err := alert.Transition(context.Background(), false, time.Now().Add(3*time.Minute))
	require.NoError(t, err)
	require.True(t, shouldSend, "should send notification when resolved")
	require.Equal(t, AlertStateInactive, alert.State(), "should transition to inactive machine")
}

func TestAlert_Transition_FiringToInactive_WhenResolved2(t *testing.T) {
	opts := &AlertOpts{HoldDuration: 0} // Immediate firing
	alert, _ := NewAlert(AlertTypeBasic, labels.FromStrings("alertname", "TestAlert"), opts)

	// Transition to firing
	_, err := alert.Transition(context.Background(), true, time.Now())
	require.NoError(t, err)

	// Resolve from firing machine
	shouldSend, err := alert.Transition(context.Background(), false, time.Now())
	require.NoError(t, err)
	require.True(t, shouldSend, "should send notification when resolved from firing")
	require.Equal(t, AlertStateInactive, alert.State())
}

func TestAlert_Transition_Firing_KeepFiringExpires(t *testing.T) {
	opts := &AlertOpts{
		HoldDuration:  1 * time.Minute,
		KeepFiringFor: 3 * time.Minute,
		ResendDelay:   1 * time.Minute,
	}
	alert, _ := NewAlert(AlertTypeBasic, labels.FromStrings("instance", "test2"), opts)
	t0 := time.Now()
	_, _ = alert.Transition(context.Background(), true, t0)
	_, _ = alert.Transition(context.Background(), true, t0.Add(2*time.Minute))
	shouldSend, err := alert.Transition(context.Background(), true, t0.Add(5*time.Minute))
	require.NoError(t, err)
	require.True(t, shouldSend, "should send resolved after keepFiring expires")
	require.Equal(t, AlertStateInactive, alert.State())
}

func TestAlert_Transition_Firing_ResendDelay1(t *testing.T) {
	opts := &AlertOpts{
		HoldDuration: 1 * time.Minute,
		ResendDelay:  2 * time.Minute,
	}
	alert, _ := NewAlert(AlertTypeBasic, labels.FromStrings("instance", "test3"), opts)
	t0 := time.Now()
	_, _ = alert.Transition(context.Background(), true, t0)
	_, _ = alert.Transition(context.Background(), true, t0.Add(2*time.Minute))

	shouldSend, err := alert.Transition(context.Background(), true, t0.Add(3*time.Minute))
	require.NoError(t, err)
	require.False(t, shouldSend, "should not resend before resend delay")

	shouldSend, err = alert.Transition(context.Background(), true, t0.Add(5*time.Minute))
	require.NoError(t, err)
	require.True(t, shouldSend, "should resend after resend delay")
}

func TestAlert_Transition_Firing_ResendDelay2(t *testing.T) {
	resendDelay := 5 * time.Minute
	opts := &AlertOpts{HoldDuration: 0, ResendDelay: resendDelay}
	alert, _ := NewAlert(AlertTypeBasic, labels.FromStrings("alertname", "TestAlert"), opts)

	// Transition to firing
	firstSendTime := time.Now()
	shouldSend, err := alert.Transition(context.Background(), true, firstSendTime)
	require.NoError(t, err)
	require.True(t, shouldSend, "should send initial notification")

	// Before resend delay
	shouldSend, err = alert.Transition(context.Background(), true, firstSendTime.Add(resendDelay/2))
	require.NoError(t, err)
	require.False(t, shouldSend, "should not resend before delay")

	// After resend delay
	shouldSend, err = alert.Transition(context.Background(), true, firstSendTime.Add(resendDelay+time.Second))
	require.NoError(t, err)
	require.True(t, shouldSend, "should resend after delay")
	require.Equal(t, AlertStateFiring, alert.State())
}

func TestAlert_Transition_AllZeroDurations(t *testing.T) {
	opts := &AlertOpts{
		HoldDuration:  0,
		KeepFiringFor: 0,
		ResendDelay:   0,
	}
	alert, _ := NewAlert(AlertTypeBasic, labels.FromStrings("instance", "test4"), opts)
	t0 := time.Now()
	shouldSend, err := alert.Transition(context.Background(), true, t0)
	require.NoError(t, err)
	require.True(t, shouldSend)
	require.Equal(t, AlertStateFiring, alert.State())

	shouldSend, err = alert.Transition(context.Background(), true, t0.Add(10*time.Second))
	require.NoError(t, err)
	require.False(t, shouldSend, "should not resend when resendDelay=0")

	shouldSend, err = alert.Transition(context.Background(), false, t0.Add(20*time.Second))
	require.NoError(t, err)
	require.True(t, shouldSend)
	require.Equal(t, AlertStateInactive, alert.State())
}

func TestAlert_MarshalUnmarshal1(t *testing.T) {
	opts := &AlertOpts{
		HoldDuration:  1 * time.Minute,
		KeepFiringFor: 10 * time.Minute,
		ResendDelay:   5 * time.Minute,
	}
	alert, _ := NewAlert(AlertTypeBasic, labels.FromStrings("foo", "bar"), opts)
	alert.SetValue(42.5)
	_, _ = alert.Transition(context.Background(), true, time.Now())

	data, err := alert.Marshal()
	require.NoError(t, err)

	var restored Alert
	err = restored.Restore(data, opts)
	require.NoError(t, err)
	require.Equal(t, alert.Labels(), restored.Labels())
	require.Equal(t, alert.GetValue(), restored.GetValue())
	require.Equal(t, alert.State(), restored.State())

	// Check raw JSON content can be decoded
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Contains(t, decoded, "labels")
	require.Contains(t, decoded, "value")
	require.Contains(t, decoded, "machine")
}

func TestAlert_MarshalUnmarshal2(t *testing.T) {
	opts := &AlertOpts{}
	alert, _ := NewAlert(AlertTypeBasic, labels.FromStrings("instance", "test1"), opts)

	// Set some machine
	alert.SetValue(42.0)
	_, _ = alert.Transition(context.Background(), true, time.Now())
	_, _ = alert.Transition(context.Background(), true, time.Now().Add(2*time.Minute))

	// Marshal
	data, err := alert.Marshal()
	require.NoError(t, err)

	// Unmarshal to new alert
	newAlert := &Alert{}
	err = newAlert.Restore(data, opts)
	require.NoError(t, err)

	// Verify
	require.Equal(t, alert.Labels(), newAlert.Labels())
	require.Equal(t, alert.GetValue(), newAlert.GetValue())
	require.Equal(t, alert.State(), newAlert.State())

	snap1, _ := alert.Marshal()
	snap2, _ := newAlert.Marshal()
	require.Equal(t, snap1, snap2)
}
