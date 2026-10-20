package agent

import (
	"testing"
	"time"
)

// 통째로 중단되면 1·2·4·8… 분 뒤 다시 하고, 주기보다 길게는 기다리지 않는다.
func TestRetryDelay(t *testing.T) {
	m := time.Minute
	for _, c := range []struct {
		interval time.Duration
		failures int
		want     time.Duration
	}{
		{30 * m, 0, 30 * m},
		{30 * m, 1, 1 * m},
		{30 * m, 2, 2 * m},
		{30 * m, 4, 8 * m},
		{30 * m, 5, 16 * m},
		{30 * m, 6, 30 * m},
		{30 * m, 100, 30 * m},
		{5 * m, 4, 5 * m},
		{1000 * m, 100, 64 * m},
	} {
		if got := retryDelay(c.interval, c.failures); got != c.want {
			t.Errorf("retryDelay(%v, %d) = %v, want %v", c.interval, c.failures, got, c.want)
		}
	}
}
