package main

import (
	"errors"
	"strings"
	"time"

	"github.com/awesome-gocui/gocui"
)

// RELAYS
func addRelay(g *gocui.Gui, v *gocui.View) error {
	maxX, maxY := g.Size()
	//prevViewName := v.Name()
	if v, err := g.SetView("addrelay", maxX/2-30, maxY/2, maxX/2+30, maxY/2+2, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}
		if _, err := g.SetCurrentView("addrelay"); err != nil {
			return err
		}
		v.Title = "Add Relay? [enter] to save / [ESC] to cancel"
		v.Editable = true
		v.KeybindOnEdit = true
		/*
			v2, _ := g.View("v2")
			_, cy := v2.Cursor()
			if prevViewName == "v2" && len(v2Meta) > 0 {
				curM := v2Meta[cy]
				var curServer RecommendServer
				ViewDB.Model(&curM).Association("Servers").Find(&curServer, "recommended_by = ?", curM.PubkeyHex)
				if curServer.Url == "" {
				} else {
					fmt.Fprintf(v, "%s", curServer.Url)
				}
			}
		*/
	}
	return nil
}

func doAddRelay(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		line := v.Buffer()
		if line == "" {
			g.SetCurrentView("v2")
			g.DeleteView("addrelay")
			refreshV4(g, 0)
			return nil
		}
		err := DB.Create(&RelayStatus{Url: line, Status: "waiting", LastEOSE: time.Unix(0, 0), LastDisco: time.Unix(0, 0)}).Error
		if err != nil {
			TheLog.Println("error adding relay")
		}
		g.DeleteView("addrelay")
		refreshV4(g, 0)
		g.SetCurrentView("v2")
	}
	return nil
}

func cancelAddRelay(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("addrelay")
	g.SetCurrentView("v2")
	return nil
}

func cursorDownV4(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		cx, cy := v.Cursor()
		//relays := []Relay{}
		//DB.Find(&relays)
		if cy < len(strings.Split(v.Buffer(), "\n"))-1 {
			if err := v.SetCursor(cx, cy+1); err != nil {
				ox, oy := v.Origin()
				if err := v.SetOrigin(ox, oy+1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func cursorUpV4(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		ox, oy := v.Origin()
		cx, cy := v.Cursor()
		if err := v.SetCursor(cx, cy-1); err != nil && oy > 0 {
			if err := v.SetOrigin(ox, oy-1); err != nil {
				return err
			}
		}
	}
	return nil
}

func delRelay(g *gocui.Gui, v *gocui.View) error {
	_, cy := v.Cursor()
	if cy < len(strings.Split(v.Buffer(), "\n"))-1 {
		lines := strings.Split(v.Buffer(), "\n")
		if len(lines) <= cy {
			return nil
		}

		line := strings.TrimSpace(lines[cy])
		if line == "" {
			return nil
		}

		// Extract relay URL from the line
		parts := strings.Split(line, " ")
		if len(parts) < 2 {
			return nil
		}
		relayUrl := parts[1]

		// Find existing relay status
		var status RelayStatus
		result := DB.Where("url = ?", relayUrl).First(&status)
		if result.Error != nil {
			TheLog.Printf("error finding relay status: %v", result.Error)
			return result.Error
		}

		// Update status to deleting
		status.Status = "deleting"
		if err := DB.Save(&status).Error; err != nil {
			TheLog.Printf("error updating relay status: %v", err)
			return err
		}

		TheLog.Printf("marked relay %s for deletion", relayUrl)
	}
	return nil
}
