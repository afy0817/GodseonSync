//go:build !darwin && !windows

package window

// Supported 는 전용 창을 쓸 수 있는지 알려 준다.
func Supported() bool { return false }

// Run 은 이 OS 에서는 지원하지 않는다.
func Run(url string) error { return ErrUnsupported }

// Focus 는 아무것도 하지 않는다.
func Focus() {}

// Close 는 아무것도 하지 않는다.
func Close() {}
