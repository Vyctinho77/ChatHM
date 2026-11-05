package wameow

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestAuthenticationNotificationDoesNotWaitForKeyPreparation(t *testing.T) {
	var loggedIn atomic.Bool
	checked := make(chan struct{}, 1)
	notified := make(chan struct{}, 1)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	go waitForAuthentication(ctx, func() bool {
		select {
		case checked <- struct{}{}:
		default:
		}
		return loggedIn.Load()
	}, func() bool { return true }, func() { notified <- struct{}{} })
	<-checked
	select {
	case <-notified:
		t.Fatal("reported connection before authentication")
	default:
	}
	loggedIn.Store(true)
	select {
	case <-notified:
	case <-ctx.Done():
		t.Fatal("did not report successful authentication")
	}
}

func TestReplacedClientCannotReportAuthentication(t *testing.T) {
	waitForAuthentication(t.Context(), func() bool { return true }, func() bool { return false }, func() { t.Fatal("replaced client changed connection state") })
}
