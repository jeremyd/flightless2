package main

import (
	"fmt"

	"github.com/awesome-gocui/gocui"
	tcell "github.com/gdamore/tcell/v2"
)

func layout(g *gocui.Gui) error {
	//useBg := gocui.Attribute(tcell.ColorSlateBlue)

	useBg := gocui.NewRGBColor(0, 0, 200)
	useFg := gocui.Attribute(tcell.ColorWhite)
	useFrame := gocui.NewRGBColor(200, 200, 200)
	maxX, maxY := g.Size()
	if v, err := g.SetView("v1", -1, -1, maxX, 1, 0); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Editable = false
		v.Wrap = false
		v.Frame = false
		v.BgColor = useBg
		v.FgColor = useFg
		fmt.Fprint(v, AppInfo)
	}

	if v, err := g.SetView("v2", 0, 1, maxX-20, maxY-20, 0); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Wrap = false
		v.Autoscroll = false
		v.BgColor = useBg
		v.FgColor = useFg
		v.FrameColor = useFrame
		v.Editable = false
		v.Highlight = true
		v.SelBgColor = gocui.ColorCyan
		v.SelFgColor = gocui.ColorBlack

		refreshV2Conversations(g, v)
		g.SetCurrentView("v2")
	}

	if v, err := g.SetView("v3", 0, maxY-21, maxX-20, maxY-6, 1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "Compose"
		v.Wrap = true
		v.Autoscroll = false
		v.BgColor = useBg
		v.FgColor = useFg
		v.FrameColor = useFrame
		v.Editable = true
	}

	if v, err := g.SetView("v4", maxX-29, 1, maxX-1, maxY-6, 4); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "Relays"
		v.Editable = false
		v.Wrap = false
		v.Autoscroll = true
		v.BgColor = useBg
		v.FgColor = useFg
		v.FrameColor = useFrame
	}

	if v, err := g.SetView("v5", 0, maxY-6, maxX-1, maxY-1, 1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "Keybinds"
		v.Editable = false
		v.Frame = true
		v.BgColor = useBg
		v.FgColor = useFg
		v.FrameColor = useFrame
		v.Clear()
		NoticeColor := "\033[1;36m%s\033[0m"
		s := fmt.Sprintf("(%s)earch", fmt.Sprintf(NoticeColor, "S"))
		q := fmt.Sprintf("(%s)uit", fmt.Sprintf(NoticeColor, "Q"))
		f := fmt.Sprintf("(%s)efresh", fmt.Sprintf(NoticeColor, "R"))
		t := fmt.Sprintf("(%s)next window", fmt.Sprintf(NoticeColor, "TAB"))
		a := fmt.Sprintf("(%s)dd relay", fmt.Sprintf(NoticeColor, "A"))

		fmt.Fprintf(v, "%-30s%-30s%-30s%-30s%-30s\n", s, q, f, t, a)
		z := fmt.Sprintf("(%s)Select ALL", fmt.Sprintf(NoticeColor, "Z"))
		d := fmt.Sprintf("(%s)elete relay", fmt.Sprintf(NoticeColor, "D"))
		c := fmt.Sprintf("(%s)onfigure keys", fmt.Sprintf(NoticeColor, "C"))
		fmt.Fprintf(v, "%-30s%-30s%-30s\n\n", z, d, c)

	}

	return nil
}
