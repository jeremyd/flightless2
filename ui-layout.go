package main

import (
	"fmt"

	"github.com/awesome-gocui/gocui"
	tcell "github.com/gdamore/tcell/v2"
)

var (
	uiColorBg    = gocui.NewRGBColor(0, 0, 200)
	uiColorFg    = gocui.Attribute(tcell.ColorWhite)
	uiColorFrame = gocui.NewRGBColor(200, 200, 200)
)

func layout(g *gocui.Gui) error {
	//useBg := gocui.Attribute(tcell.ColorSlateBlue)

	maxX, maxY := g.Size()
	if v, err := g.SetView("v1", -1, -1, maxX, 1, 0); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Editable = false
		v.Wrap = false
		v.Frame = false
		v.BgColor = uiColorBg
		v.FgColor = uiColorFg
		fmt.Fprint(v, AppInfo)
	}

	if v, err := g.SetView("v2", 0, 1, maxX-20, 10, 0); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Wrap = false
		v.Autoscroll = false
		v.BgColor = uiColorBg
		v.FgColor = uiColorFg
		v.FrameColor = uiColorFrame
		v.Editable = false
		v.Highlight = true
		v.SelBgColor = gocui.ColorCyan
		v.SelFgColor = gocui.ColorBlack

		refreshV2Conversations(g, v)
		g.SetCurrentView("v2")
	}

	if v, err := g.SetView("v3", 0, 10, maxX-20, maxY-6, 1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "Conversation"
		v.Wrap = false
		v.Autoscroll = true
		v.BgColor = uiColorBg
		v.FgColor = uiColorFg
		v.FrameColor = uiColorFrame
		v.Editable = false
		refreshV3(g, 0)
	}

	if v, err := g.SetView("v4", maxX-29, 1, maxX-1, maxY-6, 4); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "Relays"
		v.Editable = false
		v.Wrap = false
		v.Autoscroll = true
		v.BgColor = uiColorBg
		v.FgColor = uiColorFg
		v.FrameColor = uiColorFrame
		refreshV4(g, 0)
	}

	if v, err := g.SetView("v5", 0, maxY-6, maxX-1, maxY-1, 1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "Keybinds"
		v.Editable = false
		v.Autoscroll = true
		v.Frame = true
		v.BgColor = uiColorBg
		v.FgColor = uiColorFg
		v.FrameColor = uiColorFrame
		
		// Initialize keybinds view
		updateKeybindsView(g)
	}

	return nil
}
