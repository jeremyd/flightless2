package main

import (
	"fmt"

	"github.com/awesome-gocui/gocui"
)

// Theme represents a set of colors and styles for the UI
type Theme struct {
	Name              string
	Bg                gocui.Attribute
	Fg                gocui.Attribute
	HighlightBg       gocui.Attribute
	HighlightFg       gocui.Attribute
	Action            gocui.Attribute
	BorderBg          gocui.Attribute
	BorderFg          gocui.Attribute
	BorderFont        gocui.Attribute
	UseRoundedBorders bool
}

// Available themes
var (
	// Dark theme with orange-yellow highlights (new theme)
	darkTheme = Theme{
		Name:              "Dark",
		Bg:                gocui.NewRGBColor(0x14, 0x14, 0x14), // #141414
		Fg:                gocui.NewRGBColor(0xc6, 0xc6, 0xc6), // #c6c6c6
		HighlightBg:       gocui.NewRGBColor(0x1b, 0x1b, 0x1b), // #1b1b1b
		HighlightFg:       gocui.NewRGBColor(0xff, 0xff, 0xff), // #ffffff
		Action:            gocui.NewRGBColor(0xff, 0xaf, 0x00), // #ffaf00 - orange-yellow for highlighting actions
		BorderBg:          gocui.NewRGBColor(0x14, 0x14, 0x14), // #141414 - match overall bg
		BorderFg:          gocui.NewRGBColor(0x44, 0x44, 0x44), // #444444
		BorderFont:        gocui.NewRGBColor(0x66, 0x66, 0x66), // #666666
		UseRoundedBorders: true,
	}

	// Classic blue theme (original theme)
	classicTheme = Theme{
		Name:              "Classic",
		Bg:                gocui.NewRGBColor(0, 0, 200),     // Blue
		Fg:                gocui.NewRGBColor(255, 255, 255), // White
		HighlightBg:       gocui.NewRGBColor(0, 110, 150),   // highlight
		HighlightFg:       gocui.NewRGBColor(255, 255, 255), // White
		Action:            gocui.NewRGBColor(0, 255, 255),   // Cyan
		BorderBg:          gocui.NewRGBColor(0, 0, 200),     // Blue
		BorderFg:          gocui.NewRGBColor(200, 200, 200), // Light gray
		BorderFont:        gocui.NewRGBColor(200, 200, 200), // Light gray
		UseRoundedBorders: false,
	}

	// Current active theme
	activeTheme = classicTheme

	// Custom border runes for rounded corners
	customFrameRunes = []rune{'─', '│', '╭', '╮', '╰', '╯', '├', '┤', '┬', '┴', '┼'}

	// UI color variables that will be set based on the active theme
	uiColorBg          gocui.Attribute
	uiColorFg          gocui.Attribute
	uiColorHighlightBg gocui.Attribute
	uiColorHighlightFg gocui.Attribute
	uiColorAction      gocui.Attribute
	uiColorBorderBg    gocui.Attribute
	uiColorBorderFg    gocui.Attribute
	uiColorBorderFont  gocui.Attribute
)

// applyTheme sets the UI colors based on the active theme
func applyTheme() {
	uiColorBg = activeTheme.Bg
	uiColorFg = activeTheme.Fg
	uiColorHighlightBg = activeTheme.HighlightBg
	uiColorHighlightFg = activeTheme.HighlightFg
	uiColorAction = activeTheme.Action
	uiColorBorderBg = activeTheme.BorderBg
	uiColorBorderFg = activeTheme.BorderFg
	uiColorBorderFont = activeTheme.BorderFont
}

// switchTheme toggles between dark and classic themes
func switchTheme(g *gocui.Gui) error {
	// Save current state before switching themes
	v2, err := g.View("v2")
	if err != nil {
		return err
	}
	_, cy := v2.Cursor()
	currentView := g.CurrentView()
	currentViewName := ""
	if currentView != nil {
		currentViewName = currentView.Name()
	}

	// Toggle between themes
	if activeTheme.Name == darkTheme.Name {
		activeTheme = classicTheme
	} else {
		activeTheme = darkTheme
	}

	// Apply the new theme
	applyTheme()

	// Set ASCII mode based on theme
	//g.ASCII = !activeTheme.UseRoundedBorders

	// Delete all views to recreate them with new theme colors
	/*
		viewNames := []string{"v1", "v2", "v3", "v4", "v5"}
		for _, name := range viewNames {
			g.DeleteView(name)
		}
	*/

	// Refresh all views to update colors
	g.Update(func(g *gocui.Gui) error {
		// Recreate all views with the new theme
		if err := layout(g); err != nil {
			return err
		}

		// Refresh all views with current content
		if err := refreshAllViews(g, nil); err != nil {
			return err
		}

		// Restore cursor position in v2
		v2, err := g.View("v2")
		if err != nil {
			return err
		}

		// Set cursor position
		if err := v2.SetCursor(0, cy); err != nil {
			// If cursor position is out of bounds, set it to the first line
			v2.SetCursor(0, 0)
		}

		// Highlight the current line
		v2.SetHighlight(cy, true)

		// Restore current view
		if currentViewName != "" {
			g.SetCurrentView(currentViewName)
		}

		return nil
	})

	return nil
}

func layout(g *gocui.Gui) error {
	// Apply the current theme colors before setting up views
	applyTheme()
	
	maxX, maxY := g.Size()
	if v, err := g.SetView("v1", -1, -1, maxX, 1, 0); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Editable = false
		v.Wrap = false
		v.Frame = false
		v.BgColor = uiColorHighlightBg // Apply line highlight bg to header
		v.FgColor = uiColorFg
		v.FrameRunes = customFrameRunes
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
		v.FrameColor = uiColorBorderFg
		v.TitleColor = uiColorBorderFont
		v.Editable = false
		v.Highlight = true
		v.SelBgColor = uiColorHighlightBg
		v.SelFgColor = uiColorHighlightFg
		v.FrameRunes = customFrameRunes

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
		v.FrameColor = uiColorBorderFg
		v.TitleColor = uiColorBorderFont
		v.FrameRunes = customFrameRunes
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
		v.FrameColor = uiColorBorderFg
		v.TitleColor = uiColorBorderFont
		v.FrameRunes = customFrameRunes
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
		v.FrameColor = uiColorBorderFg
		v.TitleColor = uiColorBorderFont
		v.FrameRunes = customFrameRunes

		// Initialize keybinds view
		updateKeybindsView(g)
	}

	return nil
}
