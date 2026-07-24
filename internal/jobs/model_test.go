package jobs

import (
	"testing"
	"time"
)

func TestRetryDelayIncreases(t *testing.T) {
	if !(RetryDelay(1) == 30*time.Second && RetryDelay(2) > RetryDelay(1) && RetryDelay(3) > RetryDelay(2)) {
		t.Fatal("retry schedule must increase")
	}
}
