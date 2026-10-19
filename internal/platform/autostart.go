package platform

import "os"

// SetAutostart 는 로그인할 때 GodseonSync 를 자동으로 실행할지 정한다.
//
// 데이터 폴더를 따로 지정한 실행(GODSEONSYNC_HOME — 테스트·시험용 인스턴스)에서는 아무것도 하지 않는다.
// 로그인 항목은 PC 에 하나뿐이라, 시험용 인스턴스(또는 go test)가 바꾸면 실제 앱의 자동 실행이 바뀌거나
// 지워지는 테스트 바이너리를 가리키게 된다 (2026-10-19 테스트에서 실제로 바뀐 적이 있다).
func SetAutostart(enabled bool) error {
	if SeparateHome() {
		return nil
	}
	return setAutostart(enabled)
}

// SeparateHome 은 데이터 폴더를 따로 지정해 실행 중인지다 (테스트·시험용 인스턴스).
func SeparateHome() bool { return os.Getenv("GODSEONSYNC_HOME") != "" }
