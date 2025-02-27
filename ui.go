package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

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
var enterTwice = 0

// Custom editor to handle shift+enter in editable views
type messageEditor struct {
	gui *gocui.Gui
}

func (e *messageEditor) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	TheLog.Printf("messageEditor Edit key: %v (type: %T), ch: %v (decimal: %d) (type: %T), mod: %v\n",
		key, key, ch, ch, ch, mod)
	if key == gocui.KeyEnter {
		enterTwice++
	} else {
		enterTwice = 0
	}
	if enterTwice >= 2 {
		confirmPostInput(e.gui, v)
		return
	}
	gocui.DefaultEditor.Edit(v, key, ch, mod)
}

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
		v.Title = "Search"
		v.Editable = true
		v.KeybindOnEdit = true
		v.Clear()

		if _, err := g.SetCurrentView("msg"); err != nil {
			return err
		}
	}
	return nil
}

func doSearch(g *gocui.Gui, v *gocui.View) error {
	followSearch = false
	// zero out the highlighted list
	CurrOffset = 0
	msg, eM := g.View("msg")
	if eM != nil {
		return nil
	}
	searchTerm = "%" + strings.TrimSpace(msg.Buffer()) + "%"
	if searchTerm == "%%" {
		searchTerm = ""
	}
	if err := g.DeleteView("msg"); err != nil {
		return err
	}
	if _, err := g.SetCurrentView("v2"); err != nil {
		return err
	}
	refreshV2(g, v)
	refreshV3(g, 0)
	refreshV4(g, 0)
	return nil
}

func askExpand(g *gocui.Gui, cursor int) error {
	refreshV5(g, cursor)
	return nil
}

func cancelInput(g *gocui.Gui, v *gocui.View) error {
	// set v5 back to keybinds content
	v5, _ := g.View("v5")
	v5.Title = "Keybinds"
	v5.Subtitle = ""
	v5.Editable = false
	v5.Frame = true
	v5.BgColor = uiColorBg
	v5.FgColor = uiColorFg
	v5.Clear()
	NoticeColor := "\033[1;36m%s\033[0m"
	s := fmt.Sprintf("(%s)earch", fmt.Sprintf(NoticeColor, "S"))
	q := fmt.Sprintf("(%s)uit", fmt.Sprintf(NoticeColor, "Q"))
	f := fmt.Sprintf("(%s)efresh", fmt.Sprintf(NoticeColor, "R"))
	t := fmt.Sprintf("(%s)next window", fmt.Sprintf(NoticeColor, "TAB"))
	a := fmt.Sprintf("(%s)dd relay", fmt.Sprintf(NoticeColor, "A"))

	fmt.Fprintf(v5, "%-30s%-30s%-30s%-30s%-30s\n", s, q, f, t, a)
	z := fmt.Sprintf("(%s)Select ALL", fmt.Sprintf(NoticeColor, "Z"))
	d := fmt.Sprintf("(%s)elete relay", fmt.Sprintf(NoticeColor, "D"))
	c := fmt.Sprintf("(%s)onfigure keys", fmt.Sprintf(NoticeColor, "C"))
	fmt.Fprintf(v5, "%-30s%-30s%-30s\n\n", z, d, c)

	g.DeleteKeybinding("v5", gocui.KeyEnter, gocui.ModNone)
	g.SetCurrentView("v2")
	return nil
}

func confirmPostInput(g *gocui.Gui, v *gocui.View) error {
	v.Title = "Confirm Post? ENTER to post / ESC to cancel"
	v.BgColor = uiColorBg
	v.Editable = false
	g.SetKeybinding("v5", gocui.KeyEnter, gocui.ModNone, postInput)
	return nil
}

func postInput(g *gocui.Gui, v *gocui.View) error {

	msg := strings.TrimSpace(v.Buffer())
	if msg == "" {
		return nil
	}

	TheLog.Printf("would have posting message: %s\n", msg)

	v2, err := g.View("v2")
	if err != nil {
		return err
	}
	_, cy := v2.Cursor()
	if cy >= len(displayV2Meta) {
		return nil
	}

	m := displayV2Meta[cy]
	TheLog.Printf("posted to pubkey of %s", m.PubkeyHex)

	account := Account{}
	DB.Where("active = ?", true).First(&account)
	DB.Create(&ChatMessage{FromPubkey: account.Pubkey, ToPubkey: m.PubkeyHex, Content: msg})

	cancelInput(g, v)
	refreshV3(g, cy)

	// TODO: Implement actual message sending here
	// You'll need to implement the nostr message sending logic

	// Clear and close input view

	return nil
}

func cursorDownV2(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		cx, cy := v.Cursor()
		_, vSizeY := v.Size()

		// Check if we're at the end of the list
		totalItems := len(v2Meta)
		if followSearch {
			totalItems = len(followPages)
		}

		if (cy + CurrOffset + 1) >= totalItems {
			return nil
		}

		// Clear current highlight
		v.SetHighlight(cy, false)

		// If we're at the bottom of the view, move to next page
		if (cy + 1) >= (vSizeY - 1) {
			if err := v.SetCursor(0, 0); err != nil {
				if err := v.SetOrigin(0, 0); err != nil {
					return err
				}
			}
			CurrOffset += (vSizeY - 1)
			refreshV2Conversations(g, v)
			v.SetHighlight(0, true)
			return nil
		}

		// Move cursor down one line
		if err := v.SetCursor(cx, cy+1); err != nil {
			ox, oy := v.Origin()
			if err := v.SetOrigin(ox, oy+1); err != nil {
				return err
			}
		}
		v.SetHighlight(cy+1, true)
		refreshV3(g, cy+1)
		refreshV4(g, cy+1)
	}
	return nil
}

func cursorUpV2(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		cx, cy := v.Cursor()
		_, vSizeY := v.Size()

		// Check if we're at the top
		if cy == 0 && CurrOffset == 0 {
			return nil
		}

		// Clear current highlight
		v.SetHighlight(cy, false)

		// If we're at the top of the view and there are more items above
		if cy == 0 {
			newOffset := 0
			if CurrOffset >= (vSizeY - 1) {
				newOffset = CurrOffset - (vSizeY - 1)
			}
			CurrOffset = newOffset
			refreshV2Conversations(g, v)

			// Move cursor to bottom of view unless we're at the start
			newY := vSizeY - 2
			if err := v.SetCursor(cx, newY); err != nil {
				ox, oy := v.Origin()
				if err := v.SetOrigin(ox, oy-1); err != nil {
					return err
				}
			}
			v.SetHighlight(newY, true)
			return nil
		}

		// Move cursor up one line
		if err := v.SetCursor(cx, cy-1); err != nil {
			ox, oy := v.Origin()
			if err := v.SetOrigin(ox, oy-1); err != nil {
				return err
			}
		}
		v.SetHighlight(cy-1, true)
		refreshV3(g, cy-1)
		refreshV4(g, cy-1)
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
