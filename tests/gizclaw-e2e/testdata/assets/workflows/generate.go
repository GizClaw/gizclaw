//go:build ignore

// Command generate creates the committed Workflow PNG and PIXA catalog assets.
package main

import (
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

type workflowIcon struct {
	name  string
	color color.NRGBA
}

var workflowIcons = []workflowIcon{
	{"volc-ast-translate-tts", color.NRGBA{R: 232, G: 93, B: 117, A: 255}},
	{"volc-ast-translate-zh-jp", color.NRGBA{R: 238, G: 126, B: 69, A: 255}},
	{"volc-ast-translate", color.NRGBA{R: 233, G: 174, B: 51, A: 255}},
	{"chatroom", color.NRGBA{R: 118, G: 181, B: 76, A: 255}},
	{"doubao-realtime-conversation", color.NRGBA{R: 44, G: 177, B: 150, A: 255}},
	{"flowcraft-voice-assistant", color.NRGBA{R: 45, G: 156, B: 219, A: 255}},
	{"flowcraft-chat-assistant", color.NRGBA{R: 74, G: 116, B: 218, A: 255}},
	{"flowcraft-tool-chat", color.NRGBA{R: 106, G: 91, B: 207, A: 255}},
	{"flowcraft-journey-guide", color.NRGBA{R: 151, G: 83, B: 196, A: 255}},
	{"flowcraft-route-matcher", color.NRGBA{R: 196, G: 77, B: 167, A: 255}},
	{"flowcraft-multi-role-storyteller", color.NRGBA{R: 213, G: 82, B: 129, A: 255}},
	{"flowcraft-murder-mystery", color.NRGBA{R: 130, G: 79, B: 109, A: 255}},
	{"flowcraft-poetry-adventure-li-bai", color.NRGBA{R: 70, G: 138, B: 143, A: 255}},
	{"flowcraft-werewolf-game", color.NRGBA{R: 73, G: 93, B: 130, A: 255}},
	{"volc-ast-translate-zh-en", color.NRGBA{R: 55, G: 129, B: 191, A: 255}},
	{"flowcraft-assistant", color.NRGBA{R: 73, G: 170, B: 196, A: 255}},
	{"flowcraft-support", color.NRGBA{R: 76, G: 160, B: 112, A: 255}},
	{"chatroom-direct", color.NRGBA{R: 102, G: 171, B: 80, A: 255}},
	{"pet-care", color.NRGBA{R: 218, G: 140, B: 76, A: 255}},
	{"family-circle-chatroom", color.NRGBA{R: 188, G: 102, B: 74, A: 255}},
}

func main() {
	for index, icon := range workflowIcons {
		// Run from this directory so generated assets stay beside the source.
		dir := icon.name
		must(os.MkdirAll(dir, 0o755))
		pixels := renderIcon(index, icon.color, 32)
		must(writePNG(filepath.Join(dir, "icon.png"), pixels, 2))
		must(os.WriteFile(filepath.Join(dir, "icon.pixa"), encodePIXA(pixels), 0o644))
	}
}

func renderIcon(index int, fill color.NRGBA, size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	center := size / 2
	radius := size/2 - 2
	for y := range size {
		for x := range size {
			dx, dy := x-center, y-center
			if dx*dx+dy*dy <= radius*radius {
				img.SetNRGBA(x, y, fill)
			}
		}
	}
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	for bit := range 5 {
		if index&(1<<bit) == 0 {
			continue
		}
		x0 := 7 + (bit%3)*7
		y0 := 7 + (bit/3)*10
		for y := y0; y < y0+5; y++ {
			for x := x0; x < x0+5; x++ {
				img.SetNRGBA(x, y, white)
			}
		}
	}
	// Every icon has a stable center mark, while the surrounding marks encode
	// its catalog position so no two Workflow fixtures share the same image.
	for y := center - 3; y <= center+3; y++ {
		for x := center - 3; x <= center+3; x++ {
			if x == center || y == center {
				img.SetNRGBA(x, y, white)
			}
		}
	}
	return img
}

func writePNG(path string, source *image.NRGBA, scale int) error {
	bounds := source.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, bounds.Dx()*scale, bounds.Dy()*scale))
	for y := range bounds.Dy() {
		for x := range bounds.Dx() {
			value := source.NRGBAAt(x, y)
			for sy := range scale {
				for sx := range scale {
					out.SetNRGBA(x*scale+sx, y*scale+sy, value)
				}
			}
		}
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(file, out); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func encodePIXA(source *image.NRGBA) []byte {
	const headerSize, clipEntrySize, frameEntrySize = 40, 56, 16
	paletteOffset := headerSize
	clipOffset := paletteOffset + 2
	frameOffset := clipOffset + clipEntrySize
	payloadOffset := frameOffset + frameEntrySize
	payloadLength := source.Bounds().Dx() * source.Bounds().Dy() * 2
	data := make([]byte, payloadOffset+payloadLength)
	copy(data, "PIXA")
	binary.LittleEndian.PutUint16(data[4:6], 1)
	binary.LittleEndian.PutUint16(data[6:8], headerSize)
	binary.LittleEndian.PutUint16(data[8:10], uint16(source.Bounds().Dx()))
	binary.LittleEndian.PutUint16(data[10:12], uint16(source.Bounds().Dy()))
	binary.LittleEndian.PutUint16(data[12:14], 1)
	binary.LittleEndian.PutUint16(data[14:16], 1)
	binary.LittleEndian.PutUint32(data[16:20], 1)
	binary.LittleEndian.PutUint32(data[20:24], uint32(paletteOffset))
	binary.LittleEndian.PutUint32(data[24:28], uint32(clipOffset))
	binary.LittleEndian.PutUint32(data[28:32], uint32(frameOffset))
	binary.LittleEndian.PutUint32(data[32:36], uint32(payloadOffset))
	binary.LittleEndian.PutUint32(data[36:40], uint32(payloadLength))
	copy(data[clipOffset:clipOffset+32], "icon")
	binary.LittleEndian.PutUint32(data[clipOffset+40:clipOffset+44], 1)
	binary.LittleEndian.PutUint32(data[clipOffset+44:clipOffset+48], 1000)
	binary.LittleEndian.PutUint16(data[frameOffset:frameOffset+2], 1000)
	binary.LittleEndian.PutUint32(data[frameOffset+8:frameOffset+12], uint32(payloadLength))
	position := payloadOffset
	for y := range source.Bounds().Dy() {
		for x := range source.Bounds().Dx() {
			pixel := source.NRGBAAt(x, y)
			if pixel.A == 0 {
				pixel = color.NRGBA{A: 255}
			}
			value := uint16(pixel.R>>3)<<11 | uint16(pixel.G>>2)<<5 | uint16(pixel.B>>3)
			binary.LittleEndian.PutUint16(data[position:position+2], value)
			position += 2
		}
	}
	return data
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
