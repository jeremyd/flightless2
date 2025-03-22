package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/awesome-gocui/gocui"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

func config(g *gocui.Gui, v *gocui.View) error {
	maxX, maxY := g.Size()
	accounts := []Account{}
	aerr := DB.Find(&accounts).Error
	if aerr != nil {
		TheLog.Printf("error getting accounts: %s", aerr)
	}
	if v, err := g.SetView("config", maxX/2-40, maxY/2-len(accounts), maxX/2+40, maxY/2+1, 0); err != nil {
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
				var m Metadata
				DB.Where("pubkey_hex = ?", acct.Pubkey).First(&m)
				fmt.Fprintf(v, "%s[%s] %s\n", activeNotice, m.Name, acct.PubkeyNpub)
				// full priv key printing
				//fmt.Fprintf(v, "[%s] for %s\n", theKey, acct.Pubkey)
			}
		}

		v.Title = "Config Private Keys"
		v.Highlight = true
		v.SelBgColor = gocui.ColorGreen
		v.SelFgColor = gocui.ColorBlack
		v.Editable = false
		v.KeybindOnEdit = true
		if _, err := g.SetCurrentView("config"); err != nil {
			TheLog.Println("error setting current view to config")
			return nil
		}

		// Update the keybinds view to show main configuration menu keybinds
		updateMainConfigKeybindsView(g)
	}
	return nil
}

func configNew(
	g *gocui.Gui,
	v *gocui.View,
) error {
	maxX, maxY := g.Size()
	g.DeleteView("config")
	if v, err := g.SetView("confignew", maxX/2-40, maxY/2-1, maxX/2+40, maxY/2+1, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}

		v.Title = "New/Edit Private Key"
		v.Highlight = true
		v.SelBgColor = gocui.ColorGreen
		v.SelFgColor = gocui.ColorBlack
		v.Editable = true
		v.KeybindOnEdit = true
		if _, err := g.SetCurrentView("confignew"); err != nil {
			return err
		}

		// Update the keybinds view to show configuration menu keybinds
		updateConfigKeybindsView(g)
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

	var acct Account
	DB.Model(&acct).Where("pubkey = ?", accounts[cy].Pubkey).Update("active", true)
	DB.Model(&acct).Where("pubkey != ?", accounts[cy].Pubkey).Update("active", false)

	// Close existing subscriptions
	TheLog.Println("Closing existing subscriptions before switching keys")
	for _, s := range nostrSubs {
		s.Unsub()
		s.Close()
	}
	// Clear the subscriptions array
	nostrSubs = []*nostr.Subscription{}
	TheLog.Println("Closing existing relays before switching keys")
	for _, r := range nostrRelays {
		r.Close()
		UpdateOrCreateRelayStatus(DB, r.URL, "connection error: switching account")
	}

	nostrRelays = []*nostr.Relay{}

	// Kick off DM relay subscriptions for the new key
	TheLog.Printf("Starting DM relay subscriptions for pubkey: %s", accounts[cy].Pubkey)
	go doDMRelays(DB, context.Background())

	g.DeleteView("config")
	g.SetCurrentView("v2")

	// Reset cursor position and offset to prevent panic
	v2, _ := g.View("v2")
	v2.SetCursor(0, 0)
	CurrOffset = 0

	// Refresh all views to update v2 and v3 with the new active key
	refreshAllViews(g, v)

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

		g.DeleteView("config")
		config(g, v)
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
	if v, err := g.SetView("configshow", maxX/2-40, maxY/2-1, maxX/2+40, maxY/2+1, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}

		fmt.Fprintf(v, "%s", sk)
		v.Title = "*** Showing Private Key ***"
		v.Highlight = true
		v.SelBgColor = gocui.ColorGreen
		v.SelFgColor = gocui.ColorBlack
		v.Editable = false
		v.KeybindOnEdit = true
		if _, err := g.SetCurrentView("configshow"); err != nil {
			return err
		}

		// Update the keybinds view to show private key reveal screen keybinds
		updatePrivateKeyRevealKeybindsView(g)
	}
	return nil
}

func doConfigNew(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		line := v.Buffer()
		if line == "" {
			TheLog.Println("no private key entered")
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
		if !isHex(useKey) {
			TheLog.Println("private key was invalid")
			g.DeleteView("confignew")
			return nil
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

		g.DeleteView("confignew")
		g.DeleteView("config")
		config(g, v)
	}
	return nil
}

func cancelConfig(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("config")
	g.SetCurrentView("v2")

	// Reset cursor position and offset to prevent panic
	v2, _ := g.View("v2")
	v2.SetCursor(0, 0)
	CurrOffset = 0

	// Refresh all views to update v2 and v3
	refreshAllViews(g, v)

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
		config(g, v)
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
	config(g, v)

	// Reset cursor position and offset to prevent panic
	v2, _ := g.View("v2")
	v2.SetCursor(0, 0)
	CurrOffset = 0

	// Refresh all views to update v2 and v3
	refreshAllViews(g, v)

	return nil
}

func cancelConfigShow(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("configshow")
	config(g, v)

	// Reset cursor position and offset to prevent panic
	v2, _ := g.View("v2")
	v2.SetCursor(0, 0)
	CurrOffset = 0

	// Refresh all views to update v2 and v3
	refreshAllViews(g, v)

	return nil
}
