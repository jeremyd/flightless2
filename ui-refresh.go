package main

import (
	"fmt"

	"github.com/awesome-gocui/gocui"
)

func refresh(g *gocui.Gui) error {
	v2, _ := g.View("v2")
	// size of screen
	//_, vY := v2.Size()
	v2.Clear()

	for _, x := range []string{"fren1", "fren2", "fren3"} {
		fmt.Fprint(v2, x+"\n")
	}

	v2.Highlight = true
	v2.SelBgColor = gocui.ColorCyan
	v2.SelFgColor = gocui.ColorBlack
	return nil
}

func refreshV2(g *gocui.Gui, v *gocui.View) error {
	v2, _ := g.View("v2")
	v2.Clear()

	// get the active account pubkey
	account := Account{}
	DB.Where("active = ?", true).First(&account)
	pubkey := account.Pubkey

	var curFollows []Metadata
	m := Metadata{}
	DB.Where("pubkey_hex = ?", pubkey).First(&m)

	assocError := DB.Model(&m).Association("Follows").Find(&curFollows)
	if assocError != nil {
		TheLog.Printf("error getting follows for account: %s", assocError)
	}

	v2Meta = curFollows

	for _, metadata := range v2Meta {
		if metadata.Nip05 != "" {
			fmt.Fprintf(v2, "%-30s %-30s\n", metadata.Name, metadata.Nip05)
		} else {
			fmt.Fprintf(v2, "%-30s\n", metadata.Name)
		}
	}

	v2.Title = fmt.Sprintf("follows (%d)", len(v2Meta))
	v2.Highlight = true
	v2.SelBgColor = gocui.ColorCyan
	v2.SelFgColor = gocui.ColorBlack

	v4, _ := g.View("v4")
	v4.Clear()

	var curDMRelays []DMRelay
	assocError2 := DB.Model(&m).Association("DMRelays").Find(&curDMRelays)
	if assocError2 != nil {
		TheLog.Printf("error getting DM relays for account: %s", assocError2)
	}

	refreshV4(g, curDMRelays)

	return nil
}

func refreshV4(g *gocui.Gui, curDMRelays []DMRelay) error {
	v4, _ := g.View("v4")
	v4.Clear()

	for _, relay := range curDMRelays {
		fmt.Fprintf(v4, "%s\n", relay.Url)
	}

	return nil
}
