package tui

import (
	"fmt"
	"image"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"sonmi/internal/style"
)

type CameraPanel struct {
	img  image.Image
	pipe chan image.Image
	w, h int
}

var _ tea.Model = CameraPanel{}

type frameMsg image.Image

func waitForFrame(pipe chan image.Image) tea.Cmd {
	return func() tea.Msg {
		return frameMsg(<-pipe)
	}
}

func InitializeCameraPanel(pipe chan image.Image) CameraPanel {
	return CameraPanel{
		pipe: pipe,
	}
}

func (p CameraPanel) Init() tea.Cmd {
	return waitForFrame(p.pipe)
}

func (p CameraPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.w = (msg.Width / 2) - 4
		p.h = msg.Height - 4
	case frameMsg:
		p.img = image.Image(msg)
		return p, waitForFrame(p.pipe)
	}

	return p, nil
}

func (p CameraPanel) View() string {
	links := "\n" + "Devlog: sonmi.netlify.app"
	links += "\n" + "Live Stream: not yet available (YouTube stream planned)" + "\n"

	if p.img == nil {
		return "Waiting for camera stream..." + links
	}

	s := style.Header.Render("ESP32-CAM") + "\n\n"
	s += imgToANSI(p.img, p.w, p.h)
	s += links

	return s
}

func imgToANSI(img image.Image, w int, h int) string {
	bounds := img.Bounds()
	imgW, imgH := bounds.Dx(), bounds.Dy()

	ratio := float64(imgW) / float64(imgH)
	newW := w
	newH := int(float64(w) / ratio / 2.0)

	if newH > h {
		newH = h
		newW = int(float64(h) * ratio * 2.0)
	}

	var sb strings.Builder
	for y := range newH {
		for x := range newW {
			srcX := int(float64(x)/float64(newW)*float64(imgW)) + bounds.Min.X
			srcYTop := int(float64(y*2)/float64(newH*2)*float64(imgH)) + bounds.Min.Y
			srcYBot := int(float64(y*2+1)/float64(newH*2)*float64(imgH)) + bounds.Min.Y

			r1, g1, b1, _ := img.At(srcX, srcYTop).RGBA()
			r2, g2, b2, _ := img.At(srcX, srcYBot).RGBA()

			fmt.Fprintf(&sb, "\033[38;2;%d;%d;%d;48;2;%d;%d;%dm▀", r1>>8, g1>>8, b1>>8, r2>>8, g2>>8, b2>>8)
		}

		sb.WriteString("\033[0m\n")
	}

	return sb.String()
}
