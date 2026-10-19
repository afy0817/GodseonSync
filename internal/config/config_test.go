package config

import (
	"strings"
	"testing"
)

func TestNormDrivePath(t *testing.T) {
	cases := map[string]string{
		" /GodseonSync//문서/ ": "GodseonSync/문서",
		`a\b\c`:               "a/b/c",
		"":                    "",
	}
	for in, want := range cases {
		if got := NormDrivePath(in); got != want {
			t.Errorf("NormDrivePath(%q) = %q, want %q", in, got, want)
		}
	}
}

var testPCs = []PC{
	{ID: "pc-mac", Name: "맥", OS: "mac"},
	{ID: "pc-win1", Name: "윈도우1", OS: "windows"},
	{ID: "pc-win2", Name: "윈도우2", OS: "windows"},
}

func TestValidate(t *testing.T) {
	ok := &Config{PCs: testPCs, Folders: []Folder{
		{ID: "1", Name: "문서", DrivePath: "GodseonSync/문서", Paths: map[string]string{
			"pc-mac": "/Users/me/문서", "pc-win1": `D:\문서`, "pc-win2": `C:\Users\me\문서`}},
		{ID: "2", Name: "사진", DrivePath: "사진 백업", Paths: map[string]string{
			"pc-mac": "~/Pictures", "pc-win1": `D:\사진`}},
	}}
	_ = ok.Normalize()
	if errs := Validate(ok); len(errs) != 0 {
		t.Fatalf("문제가 없어야 함: %v", errs)
	}

	bad := &Config{PCs: testPCs, Folders: []Folder{
		{ID: "1", Name: "a", DrivePath: "X/a", Paths: map[string]string{"pc-win1": `D:\work`, "pc-mac": "relative/path"}},
		{ID: "2", Name: "A", DrivePath: "X/a/b", Paths: map[string]string{"pc-win1": `d:/WORK/sub`}},
		{ID: "3", Name: "", DrivePath: ""},
		{ID: "4", Name: "root", DrivePath: "GodseonSync"},
		{ID: "5", Name: "status", DrivePath: "GodseonSync/.status/x"},
		{ID: "6", Name: "setting", DrivePath: "GodseonSync/setting.json"},
	}}
	_ = bad.Normalize()
	errs := strings.Join(Validate(bad), "\n")
	for _, want := range []string{"겹칩니다", "절대 경로", "이름이 비어", "드라이브 경로를 입력", "폴더 자체는", ".status/x 는 GodseonSync", "setting.json 는 GodseonSync"} {
		if !strings.Contains(errs, want) {
			t.Errorf("오류에 %q 가 있어야 함:\n%s", want, errs)
		}
	}
	// 같은 윈도우라도 PC 가 다르면 같은 경로를 써도 된다
	same := &Config{PCs: testPCs, Folders: []Folder{
		{ID: "1", Name: "a", DrivePath: "X/a", Paths: map[string]string{"pc-win1": `D:\work`, "pc-win2": `D:\work`}},
	}}
	_ = same.Normalize()
	if errs := Validate(same); len(errs) != 0 {
		t.Fatalf("PC 가 다르면 경로가 같아도 됨: %v", errs)
	}
}

func TestFiltersText(t *testing.T) {
	c := &Config{Exclude: []string{".DS_Store", "node_modules/", "- *.bak"}}
	f := &Folder{DrivePath: "GodseonSync/x", Exclude: []string{"*.tmp"}}
	got := FiltersText(c, f)
	for _, want := range []string{"- .DS_Store\n", "- node_modules/**\n", "- *.bak\n", "- *.tmp\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("필터에 %q 가 있어야 함:\n%s", want, got)
		}
	}
}

func TestSameFoldersIgnoresPCs(t *testing.T) {
	a := Default()
	a.Folders = []Folder{{ID: "1", Name: "a", DrivePath: "X/a", Paths: map[string]string{"pc-mac": "/a"}}}
	_ = a.Normalize()
	b := Default()
	b.Folders = []Folder{{ID: "1", Name: "a", DrivePath: "X/a", Paths: map[string]string{"pc-mac": "/a"}}}
	b.PCs = testPCs
	b.Revision = 9
	_ = b.Normalize()
	if !SameFolders(a, b) {
		t.Fatal("PC 목록·revision 이 달라도 폴더 목록이 같으면 같다고 봐야 함")
	}
	b.Folders[0].Paths["pc-win1"] = `D:\a`
	if SameFolders(a, b) {
		t.Fatal("경로가 다르면 달라야 함")
	}
}

func TestNewerSchemaRejected(t *testing.T) {
	c := &Config{Version: SchemaVersion + 1}
	if err := c.Normalize(); err != ErrNewerSchema {
		t.Fatalf("새 버전 설정은 거부해야 함: %v", err)
	}
}
