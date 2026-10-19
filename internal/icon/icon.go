// Package icon 은 트레이 아이콘과 앱 아이콘을 코드로 그린다 (두 개의 원호 화살표 = 동기화).
package icon

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
)

// State 는 트레이 아이콘 모양이다.
type State int

const (
	Normal State = iota
	Error
	Paused
)

var (
	blue = color.NRGBA{0x25, 0x63, 0xeb, 0xff}
	red  = color.NRGBA{0xd9, 0x3a, 0x3a, 0xff}
	gray = color.NRGBA{0x80, 0x88, 0x94, 0xff}
)

// Style 은 그리는 방법이다.
type Style struct {
	Background color.NRGBA // 둥근 사각형 배경 (알파 0 이면 배경 없음)
	Foreground color.NRGBA // 화살표 색
	Margin     float64     // 가장자리 여백 (0~0.5)
	Rotate     float64     // 화살표 회전 (도) — 동기화 중 애니메이션
	Scale      float64     // 화살표 크기 배율 (0 이면 1)
	Badge      State       // 템플릿 아이콘에서 오류·일시 정지 표시
}

const (
	radius  = 0.27 // 원호 반지름 (아이콘 안쪽 0~1 좌표)
	stroke  = 0.095
	headLen = 0.15
	headW   = 0.13
)

type pt struct{ x, y float64 }

func sub(a, b pt) pt              { return pt{a.x - b.x, a.y - b.y} }
func cross(a, b pt) float64       { return a.x*b.y - a.y*b.x }
func dist(a, b pt) float64        { return math.Hypot(a.x-b.x, a.y-b.y) }
func polar(deg float64) pt        { r := deg * math.Pi / 180; return pt{math.Cos(r), math.Sin(r)} }
func at(c pt, r float64, d pt) pt { return pt{c.x + d.x*r, c.y + d.y*r} }

func inTriangle(p, a, b, c pt) bool {
	d1, d2, d3 := cross(sub(b, a), sub(p, a)), cross(sub(c, b), sub(p, b)), cross(sub(a, c), sub(p, c))
	neg := d1 < 0 || d2 < 0 || d3 < 0
	pos := d1 > 0 || d2 > 0 || d3 > 0
	return !(neg && pos)
}

// inArrows 는 0~1 좌표의 점이 화살표 두 개 안에 있는지 본다 (화면 좌표: y 가 아래로).
func inArrows(p pt) bool {
	c := pt{0.5, 0.5}
	for _, arc := range [][2]float64{{200, 335}, {20, 155}} {
		start, end := arc[0], arc[1]
		ang := math.Atan2(p.y-c.y, p.x-c.x) * 180 / math.Pi
		if ang < 0 {
			ang += 360
		}
		d := dist(p, c)
		if math.Abs(d-radius) <= stroke/2 && ang >= start && ang <= end {
			return true
		}
		if dist(p, at(c, radius, polar(start))) <= stroke/2 { // 시작 쪽 둥근 끝
			return true
		}
		// 끝 쪽 화살촉: 원호 끝에서 진행 방향(시계 방향 접선)으로 뾰족하게
		e := at(c, radius, polar(end))
		n := polar(end)
		tng := pt{-n.y, n.x}
		tip := pt{e.x + tng.x*headLen, e.y + tng.y*headLen}
		if inTriangle(p, at(e, headW, n), at(e, -headW, n), tip) {
			return true
		}
	}
	return false
}

func inRoundRect(p pt, m, r float64) bool {
	lo, hi := m, 1-m
	if p.x < lo || p.x > hi || p.y < lo || p.y > hi {
		return false
	}
	cx := math.Min(math.Max(p.x, lo+r), hi-r)
	cy := math.Min(math.Max(p.y, lo+r), hi-r)
	return dist(p, pt{cx, cy}) <= r
}

// Render 는 size×size 아이콘을 그린다 (4×4 슈퍼샘플링으로 가장자리를 부드럽게).
func Render(size int, st Style) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const ss = 4
	rot := -st.Rotate * math.Pi / 180
	scale := st.Scale
	if scale == 0 {
		scale = 1
	}
	badge := pt{0.8, 0.8}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var bg, fg float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					p := pt{(float64(x) + (float64(sx)+0.5)/ss) / float64(size), (float64(y) + (float64(sy)+0.5)/ss) / float64(size)}
					inner := 1 - 2*st.Margin
					q := pt{((p.x-st.Margin)/inner - 0.5) / scale, ((p.y-st.Margin)/inner - 0.5) / scale}
					q = pt{q.x*math.Cos(rot) - q.y*math.Sin(rot) + 0.5, q.x*math.Sin(rot) + q.y*math.Cos(rot) + 0.5}
					arrows := inArrows(q)
					if st.Badge != Normal {
						d := dist(p, badge)
						if d < 0.24 {
							arrows = false
						}
						if st.Badge == Error && d <= 0.17 {
							arrows = true
						}
						if st.Badge == Paused && math.Abs(p.y-badge.y) <= 0.15 &&
							(math.Abs(p.x-(badge.x-0.07)) <= 0.04 || math.Abs(p.x-(badge.x+0.07)) <= 0.04) {
							arrows = true
						}
					}
					if st.Background.A > 0 && inRoundRect(p, st.Margin, 0.22*(1-2*st.Margin)) {
						bg++
					}
					if arrows {
						fg++
					}
				}
			}
			bg /= ss * ss
			fg /= ss * ss
			img.SetNRGBA(x, y, blend(st.Background, bg, st.Foreground, fg))
		}
	}
	return img
}

// blend 는 배경 위에 전경을 덮은 색이다 (둘 다 덮인 비율로).
func blend(b color.NRGBA, ba float64, f color.NRGBA, fa float64) color.NRGBA {
	ba *= float64(b.A) / 255
	fa *= float64(f.A) / 255
	a := fa + ba*(1-fa)
	if a == 0 {
		return color.NRGBA{}
	}
	mix := func(fc, bc uint8) uint8 {
		return uint8(math.Round((float64(fc)*fa + float64(bc)*ba*(1-fa)) / a))
	}
	return color.NRGBA{mix(f.R, b.R), mix(f.G, b.G), mix(f.B, b.B), uint8(math.Round(a * 255))}
}

// AppStyle 은 앱 아이콘 (파란 둥근 사각형에 흰 화살표).
func AppStyle(margin float64) Style {
	return Style{Background: blue, Foreground: color.NRGBA{0xff, 0xff, 0xff, 0xff}, Margin: margin}
}

// TrayStyle 은 윈도우·리눅스 트레이 아이콘 (상태별 배경색).
func TrayStyle(s State, rotate float64) Style {
	bg := blue
	switch s {
	case Error:
		bg = red
	case Paused:
		bg = gray
	}
	return Style{Background: bg, Foreground: color.NRGBA{0xff, 0xff, 0xff, 0xff}, Margin: 0.02, Rotate: rotate, Scale: 1.15}
}

// TemplateStyle 은 맥 메뉴 막대용 흑백 템플릿 아이콘 (macOS 가 밝기에 맞춰 색을 바꾼다).
func TemplateStyle(s State, rotate float64) Style {
	return Style{Foreground: color.NRGBA{0, 0, 0, 0xff}, Margin: 0.02, Rotate: rotate, Badge: s, Scale: 1.2}
}

// PNG 는 이미지를 PNG 바이트로 만든다.
func PNG(img image.Image) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// ICO 는 여러 크기 이미지를 .ico 로 묶는다. 256 은 PNG, 나머지는 32비트 BMP 로 넣는다 (트레이 호환).
func ICO(imgs []*image.NRGBA) []byte {
	var data [][]byte
	for _, im := range imgs {
		w := im.Bounds().Dx()
		if w >= 256 {
			data = append(data, PNG(im))
			continue
		}
		var b bytes.Buffer
		maskRow := ((w + 31) / 32) * 4
		hdr := []any{uint32(40), int32(w), int32(2 * w), uint16(1), uint16(32), uint32(0),
			uint32(w*w*4 + maskRow*w), int32(0), int32(0), uint32(0), uint32(0)}
		for _, v := range hdr {
			_ = binary.Write(&b, binary.LittleEndian, v)
		}
		for y := w - 1; y >= 0; y-- { // 아래 줄부터
			for x := 0; x < w; x++ {
				c := im.NRGBAAt(x, y)
				b.Write([]byte{c.B, c.G, c.R, c.A})
			}
		}
		for y := w - 1; y >= 0; y-- {
			row := make([]byte, maskRow)
			for x := 0; x < w; x++ {
				if im.NRGBAAt(x, y).A == 0 {
					row[x/8] |= 0x80 >> (x % 8)
				}
			}
			b.Write(row)
		}
		data = append(data, b.Bytes())
	}
	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, []uint16{0, 1, uint16(len(imgs))})
	offset := 6 + 16*len(imgs)
	for i, im := range imgs {
		w := im.Bounds().Dx()
		dim := byte(w)
		if w >= 256 {
			dim = 0
		}
		out.Write([]byte{dim, dim, 0, 0})
		_ = binary.Write(&out, binary.LittleEndian, []uint16{1, 32})
		_ = binary.Write(&out, binary.LittleEndian, []uint32{uint32(len(data[i])), uint32(offset)})
		offset += len(data[i])
	}
	for _, d := range data {
		out.Write(d)
	}
	return out.Bytes()
}

// TrayImages 는 윈도우 트레이용 여러 크기 이미지다 (화면 배율별로 Windows 가 골라 쓴다).
func TrayImages(s State, rotate float64) []*image.NRGBA {
	var out []*image.NRGBA
	for _, size := range []int{16, 20, 24, 32, 48} {
		out = append(out, Render(size, TrayStyle(s, rotate)))
	}
	return out
}
