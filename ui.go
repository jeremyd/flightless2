package main

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/awesome-gocui/gocui"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

var curViewNum int = 0
var selectableViews = []string{"v2", "v3", "v4"}
var v2Meta []Metadata
var searchTerm = ""
var followSearch = false
var CurrOffset = 0
var followPages []Metadata
var followTarget Metadata
var highlighted []string

func quit(g *gocui.Gui, v *gocui.View) error {
	p, err := os.FindProcess(os.Getpid())

	if err != nil {
		return err
	}

	p.Signal(syscall.SIGTERM)
	return nil
}

func next(g *gocui.Gui, v *gocui.View) error {
	for _, view := range selectableViews {
		t, _ := g.View(view)
		//v.FrameColor = gocui.NewRGBColor(255, 255, 255)
		t.Highlight = false
	}
	if curViewNum == len(selectableViews)-1 {
		curViewNum = 0
	} else {
		curViewNum += 1
	}
	newV, err := g.SetCurrentView(selectableViews[curViewNum])
	if err != nil {
		TheLog.Println("ERROR selecting view")
		return nil
	}
	//v.FrameColor = gocui.NewRGBColor(200, 100, 100)
	newV.Highlight = true
	newV.SelBgColor = gocui.ColorCyan
	newV.SelFgColor = gocui.ColorBlack
	return nil
}

func config(g *gocui.Gui, v *gocui.View) error {
	maxX, maxY := g.Size()
	accounts := []Account{}
	aerr := DB.Find(&accounts).Error
	if aerr != nil {
		TheLog.Printf("error getting accounts: %s", aerr)
	}
	if v, err := g.SetView("config", maxX/2-50, maxY/2-len(accounts), maxX/2+50, maxY/2+1, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}

		theKey := ""
		for _, acct := range accounts {
			theKey = Decrypt(string(Password), acct.Privatekey)
			if len(theKey) != 64 {
				fmt.Fprintf(v, "invalid key.. delete please: %s", theKey)
			} else {
				activeNotice := ""
				if acct.Active {
					activeNotice = "*"
				}
				fmt.Fprintf(v, "%s[%s ... ] for %s\n", activeNotice, theKey[0:5], acct.PubkeyNpub)
				// full priv key printing
				//fmt.Fprintf(v, "[%s] for %s\n", theKey, acct.Pubkey)
			}
		}

		v.Title = "Config Private Keys - [Enter]Use key - [ESC]Cancel - [n]ew key - [d]elete key - [g]enerate key - [p]rivate key reveal"
		v.Highlight = true
		v.SelBgColor = gocui.ColorGreen
		v.SelFgColor = gocui.ColorBlack
		v.Editable = false
		v.KeybindOnEdit = true
		if _, err := g.SetCurrentView("config"); err != nil {
			TheLog.Println("error setting current view to config")
			return nil
		}
	}
	return nil
}

func search(g *gocui.Gui, v *gocui.View) error {
	maxX, maxY := g.Size()
	if v, err := g.SetView("msg", maxX/2-30, maxY/2, maxX/2+30, maxY/2+2, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}
		if _, err := g.SetCurrentView("msg"); err != nil {
			return err
		}
		v.Title = "Search"
		v.Editable = true
		v.KeybindOnEdit = true
	}
	return nil
}

func doSearch(g *gocui.Gui, v *gocui.View) error {
	followSearch = false
	// zero out the highlighted list
	highlighted = []string{}
	CurrOffset = 0
	msg, eM := g.View("msg")
	if eM != nil {
		return nil
	}
	searchTerm = "%" + msg.Buffer() + "%"
	g.DeleteView("msg")
	g.SetCurrentView("v2")
	refreshV2(g, v)
	//refreshV3(g, v)
	return nil
}

func cursorDownV2(g *gocui.Gui, v *gocui.View) error {
	if v != nil {

		cx, cy := v.Cursor()
		_, vSizeY := v.Size()

		if followSearch && (cy+CurrOffset+1) >= len(followPages) {
			// end of list
			return nil
		}

		if !followSearch && len(v2Meta) != vSizeY-1 && (cy+1) >= len(v2Meta) {
			// end of list
			return nil
		}

		if (cy + 1) >= (vSizeY - 1) {
			// end of page / next page
			if err := v.SetCursor(0, 0); err != nil {
				if err := v.SetOrigin(0, 0); err != nil {
					return err
				}
			}
			CurrOffset += (vSizeY - 1)
			refreshV2(g, v)
			//refreshV3(g, v)
			return nil
		}

		if err := v.SetCursor(cx, cy+1); err != nil {
			ox, oy := v.Origin()
			if err := v.SetOrigin(ox, oy+1); err != nil {
				return err
			}
		}
		//refreshV3(g, v)
	}
	return nil
}

func cursorUpV2(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		cx, cy := v.Cursor()
		_, vSizeY := v.Size()
		if cy == 0 && CurrOffset == 0 {
			return nil
		}
		// page up
		if cy == 0 {
			if CurrOffset >= (vSizeY - 1) {
				CurrOffset -= (vSizeY - 1)
			} else {
				CurrOffset = 0
			}
			refresh(g)
			ox, oy := v.Origin()
			if err := v.SetCursor(cx, vSizeY-2); err != nil && oy > 0 {
				if err := v.SetOrigin(ox, oy-1); err != nil {
					return err
				}
			}
			// just up
		} else {
			ox, oy := v.Origin()
			if err := v.SetCursor(cx, cy-1); err != nil && oy > 0 {
				if err := v.SetOrigin(ox, oy-1); err != nil {
					return err
				}
			}
		}
		//refreshV3(g, v)
	}
	return nil
}

func configNew(
	g *gocui.Gui,
	v *gocui.View,
) error {
	maxX, maxY := g.Size()
	g.DeleteView("config")
	if v, err := g.SetView("confignew", maxX/2-50, maxY/2-1, maxX/2+50, maxY/2+1, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}

		v.Title = "New/Edit Private Key - [Enter]Save - [ESC]Cancel -"
		v.Highlight = true
		v.SelBgColor = gocui.ColorGreen
		v.SelFgColor = gocui.ColorBlack
		v.Editable = true
		v.KeybindOnEdit = true
		if _, err := g.SetCurrentView("confignew"); err != nil {
			return err
		}
	}
	return nil
}

func activateConfig(
	g *gocui.Gui,
	v *gocui.View,
) error {
	cView, _ := g.View("config")
	_, cy := cView.Cursor()
	accounts := []Account{}
	aerr := DB.Find(&accounts).Error
	if aerr != nil {
		TheLog.Printf("error getting accounts: %s", aerr)
	}
	for i, acct := range accounts {
		if i != cy {
			acct.Active = false
			DB.Save(&acct)
		}
	}

	accounts[cy].Active = true
	DB.Save(accounts[cy])
	g.DeleteView("config")
	g.SetCurrentView("v2")
	return nil
}

func generateConfig(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		sk := nostr.GeneratePrivateKey()
		encKey := Encrypt(string(Password), sk)
		pk, ep := nostr.GetPublicKey(sk)
		npub, ep2 := nip19.EncodePublicKey(pk)
		if ep != nil || ep2 != nil {
			TheLog.Printf("error getting public key: %s", ep)
		}
		account := Account{Privatekey: encKey, Pubkey: pk, PubkeyNpub: npub, Active: true}
		e2 := DB.Save(&account).Error
		if e2 != nil {
			TheLog.Printf("error saving private key: %s", e2)
		}

		g.SetCurrentView("v2")
		g.DeleteView("config")
	}
	return nil
}

func configShowPrivateKey(
	g *gocui.Gui,
	v *gocui.View,
) error {
	maxX, maxY := g.Size()
	cView, _ := g.View("config")
	_, cy := cView.Cursor()
	accounts := []Account{}
	aerr := DB.Find(&accounts).Error
	if aerr != nil {
		TheLog.Printf("error getting accounts: %s", aerr)
	}
	sk := Decrypt(string(Password), accounts[cy].Privatekey)
	g.DeleteView("config")
	if v, err := g.SetView("configshow", maxX/2-50, maxY/2-1, maxX/2+50, maxY/2+1, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}

		fmt.Fprintf(v, "%s", sk)
		v.Title = "*** Showing Private Key ***  [ESC]Dismiss"
		v.Highlight = true
		v.SelBgColor = gocui.ColorGreen
		v.SelFgColor = gocui.ColorBlack
		v.Editable = false
		v.KeybindOnEdit = true
		if _, err := g.SetCurrentView("configshow"); err != nil {
			return err
		}
	}
	return nil
}

func doConfigNew(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		line := v.Buffer()
		if line == "" {
			TheLog.Println("no private key entered")
			g.SetCurrentView("v2")
			g.DeleteView("confignew")
			return nil
		}
		//fmt.Println(line)
		//fmt.Println("saving config")
		useKey := line
		if line[0:1] == "n" {
			prefix, value, err := nip19.Decode(line)
			if err != nil {
				TheLog.Println("error decoding nip19 key")
				return nil
			} else {
				TheLog.Println("decoded nip19 " + prefix + "to hex")
				useKey = value.(string)
			}
		}

		encKey := Encrypt(string(Password), useKey)
		pk, ep := nostr.GetPublicKey(useKey)
		npub, ep2 := nip19.EncodePublicKey(pk)
		if ep != nil || ep2 != nil {
			TheLog.Printf("error getting public key: %s", ep)
		}
		account := Account{Privatekey: encKey, Pubkey: pk, PubkeyNpub: npub, Active: true}
		e2 := DB.Save(&account).Error
		if e2 != nil {
			TheLog.Printf("error saving private key: %s", e2)
		}

		g.SetCurrentView("v2")
		g.DeleteView("confignew")
		//refresh(g, v)
	}
	return nil
}

func cancelConfig(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("config")
	g.SetCurrentView("v2")
	return nil
}

func doConfigDel(g *gocui.Gui, v *gocui.View) error {
	cView, _ := g.View("config")
	_, cy := cView.Cursor()
	accounts := []Account{}
	aerr := DB.Find(&accounts).Error
	if aerr != nil {
		TheLog.Printf("error getting accounts: %s", aerr)
	}
	if v != nil {
		line := v.Buffer()
		if line == "" {
			g.SetCurrentView("v2")
			g.DeleteView("config")
			return nil
		}
		e2 := DB.Delete(&accounts[cy]).Error
		if e2 != nil {
			TheLog.Printf("error deleting private key: %s", e2)
		}

		// activate a different account if there are any
		DB.Find(&accounts)
		anyActive := false
		anyAccounts := (len(accounts) > 0)
		for _, a := range accounts {
			if a.Active {
				anyActive = true
			}
		}
		if !anyActive && anyAccounts {
			accounts[0].Active = true
			DB.Save(&accounts[0])
		}

		g.DeleteView("config")
	}
	return nil
}

func cursorDownConfig(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		_, cy := v.Cursor()
		_, sy := v.Size()
		// end of view
		if cy >= sy-1 {
			return nil
		}
		// move cursor and origin
		if err := v.SetCursor(0, cy+1); err != nil {
			ox, oy := v.Origin()
			if err := v.SetOrigin(ox, oy+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func cursorUpConfig(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		_, cy := v.Cursor()
		// top of view
		if cy == 0 {
			return nil
		}
		// move cursor and origin
		if err := v.SetCursor(0, cy-1); err != nil {
			ox, oy := v.Origin()
			if err := v.SetOrigin(ox, oy-1); err != nil {
				return err
			}
		}
	}
	return nil
}

func cancelConfigNew(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("confignew")
	g.SetCurrentView("v2")
	return nil
}

func cancelConfigShow(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("configshow")
	g.SetCurrentView("v2")
	return nil
}
