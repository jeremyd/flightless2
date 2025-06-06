package main

import (
	"log"

	"github.com/awesome-gocui/gocui"
)

func keybindings(g *gocui.Gui) error {

	// tab key (next window)
	if err := g.SetKeybinding("", gocui.KeyTab, gocui.ModNone, next); err != nil {
		TheLog.Panicln(err)
	}

	// q key (quit)
	if err := g.SetKeybinding("", rune(0x71), gocui.ModNone, quit); err != nil {
		TheLog.Panicln(err)
	}

	// r key (refresh)
	if err := g.SetKeybinding("", rune(0x72), gocui.ModNone, refreshAll); err != nil {
		TheLog.Panicln(err)
	}
	// s key (search)
	if err := g.SetKeybinding("", rune(0x73), gocui.ModNone, search); err != nil {
		log.Panicln(err)
	}

	/* v2 View (main) */
	// cursor
	if err := g.SetKeybinding("v2", gocui.KeyArrowDown, gocui.ModNone, cursorDownV2); err != nil {
		log.Panicln(err)
	}
	if err := g.SetKeybinding("v2", gocui.KeyArrowUp, gocui.ModNone, cursorUpV2); err != nil {
		log.Panicln(err)
	}
	// cursor vim
	// j key is down rune
	if err := g.SetKeybinding("v2", rune(0x6a), gocui.ModNone, cursorDownV2); err != nil {
		log.Panicln(err)
	}
	// k key is up
	if err := g.SetKeybinding("v2", rune(0x6b), gocui.ModNone, cursorUpV2); err != nil {
		log.Panicln(err)
	}

	// t key is toggle conversation/follows
	if err := g.SetKeybinding("v2", rune(0x74), gocui.ModNone, toggleConversationFollows); err != nil {
		log.Panicln(err)
	}

	// z key for zaps
	if err := g.SetKeybinding("v2", rune(0x7a), gocui.ModNone, zapUserMenu); err != nil {
		log.Panicln(err)
	}

	/* addrelay view */
	if err := g.SetKeybinding("v2", rune(0x61), gocui.ModNone, addRelay); err != nil {
		log.Panicln(err)
	}

	/* enter key */
	if err := g.SetKeybinding("v2", gocui.KeyEnter, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		_, cy := v.Cursor()
		return askExpand(g, cy)
	}); err != nil {
		log.Panicln(err)
	}

	/* v4 View (relays) */
	/* v4 View (Relay List) */
	// d key (delete)
	if err := g.SetKeybinding("v4", rune(0x64), gocui.ModNone, delRelay); err != nil {
		log.Panicln(err)
	}
	// cursor
	if err := g.SetKeybinding("v4", gocui.KeyArrowDown, gocui.ModNone, cursorDownV4); err != nil {
		log.Panicln(err)
	}
	if err := g.SetKeybinding("v4", gocui.KeyArrowUp, gocui.ModNone, cursorUpV4); err != nil {
		log.Panicln(err)
	}
	// vim cursor
	// j key (down)
	if err := g.SetKeybinding("v4", rune(0x6a), gocui.ModNone, cursorDownV4); err != nil {
		log.Panicln(err)
	}
	// k key (up)
	if err := g.SetKeybinding("v4", rune(0x6b), gocui.ModNone, cursorUpV4); err != nil {
		log.Panicln(err)
	}
	// a key (add new relay)
	if err := g.SetKeybinding("v4", rune(0x61), gocui.ModNone, addRelay); err != nil {
		log.Panicln(err)
	}

	// add relay
	if err := g.SetKeybinding("addrelay", gocui.KeyEnter, gocui.ModNone, doAddRelay); err != nil {
		log.Panicln(err)
	}
	//cancel key
	if err := g.SetKeybinding("addrelay", gocui.KeyEsc, gocui.ModNone, cancelAddRelay); err != nil {
		log.Panicln(err)
	}

	/* search view */
	if err := g.SetKeybinding("msg", gocui.KeyEnter, gocui.ModNone, doSearch); err != nil {
		log.Panicln(err)
	}
	if err := g.SetKeybinding("msg", gocui.KeyEsc, gocui.ModNone, cancelSearch); err != nil {
		log.Panicln(err)
	}

	/* fetch view */
	// rune for "f"
	if err := g.SetKeybinding("v2", rune(0x66), gocui.ModNone, fetch); err != nil {
		log.Panicln(err)
	}

	// rune for "p" - fetch by pubkey/npub
	if err := g.SetKeybinding("", rune(0x70), gocui.ModNone, fetchByPubkey); err != nil {
		log.Panicln(err)
	}

	// Enter key in fetchpubkey view
	if err := g.SetKeybinding("fetchpubkey", gocui.KeyEnter, gocui.ModNone, doFetchByPubkey); err != nil {
		log.Panicln(err)
	}

	// ESC key in fetchpubkey view
	if err := g.SetKeybinding("fetchpubkey", gocui.KeyEsc, gocui.ModNone, cancelFetchPubkey); err != nil {
		log.Panicln(err)
	}

	/* fetch results view */
	if err := g.SetKeybinding("fetchresults", gocui.KeyEsc, gocui.ModNone, closeFetchResults); err != nil {
		log.Panicln(err)
	}

	/* config view for accounts */
	//cancel key
	if err := g.SetKeybinding("config", gocui.KeyEsc, gocui.ModNone, cancelConfig); err != nil {
		log.Panicln(err)
	}
	if err := g.SetKeybinding("config", gocui.KeyEnter, gocui.ModNone, activateConfig); err != nil {
		log.Panicln(err)
	}
	// g key generate key
	if err := g.SetKeybinding("config", rune(0x67), gocui.ModNone, generateConfig); err != nil {
		log.Panicln(err)
	}
	// unsupported: edit
	//if err := g.SetKeybinding("config", gocui.KeyEnter, gocui.ModNone, configEdit); err != nil {
	//	log.Panicln(err)
	//}

	// c key (Config)
	if err := g.SetKeybinding("", rune(0x63), gocui.ModNone, config); err != nil {
		log.Panicln(err)
	}

	// n key (new config)
	if err := g.SetKeybinding("config", rune(0x6e), gocui.ModNone, configNew); err != nil {
		log.Panicln(err)
	}
	// d key (delete config)
	if err := g.SetKeybinding("config", rune(0x64), gocui.ModNone, doConfigDel); err != nil {
		log.Panicln(err)
	}
	if err := g.SetKeybinding("config", gocui.KeyArrowDown, gocui.ModNone, cursorDownConfig); err != nil {
		log.Panicln(err)
	}
	if err := g.SetKeybinding("config", gocui.KeyArrowUp, gocui.ModNone, cursorUpConfig); err != nil {
		log.Panicln(err)
	}
	// p key (show private key)
	if err := g.SetKeybinding("config", rune(0x70), gocui.ModNone, configShowPrivateKey); err != nil {
		log.Panicln(err)
	}
	/* config submenu (new/edit) */
	//cancel key
	if err := g.SetKeybinding("confignew", gocui.KeyEsc, gocui.ModNone, cancelConfigNew); err != nil {
		log.Panicln(err)
	}

	if err := g.SetKeybinding("confignew", gocui.KeyEnter, gocui.ModNone, doConfigNew); err != nil {
		log.Panicln(err)
	}

	//cancel key
	if err := g.SetKeybinding("configshow", gocui.KeyEsc, gocui.ModNone, cancelConfigShow); err != nil {
		log.Panicln(err)
	}

	/* profile menu */
	// m key (Profile Menu)
	if err := g.SetKeybinding("", rune(0x6d), gocui.ModNone, profileMenu); err != nil {
		log.Panicln(err)
	}

	// e key (Edit Profile Metadata)
	if err := g.SetKeybinding("profile", rune(0x65), gocui.ModNone, editProfileMetadata); err != nil {
		log.Panicln(err)
	}

	// d key (Edit DM Relays)
	if err := g.SetKeybinding("profile", rune(0x64), gocui.ModNone, editDMRelays); err != nil {
		log.Panicln(err)
	}

	// ESC key (Cancel Profile Menu)
	if err := g.SetKeybinding("profile", gocui.KeyEsc, gocui.ModNone, cancelProfile); err != nil {
		log.Panicln(err)
	}

	/* profile fields selection */
	// Enter key (Select Field)
	if err := g.SetKeybinding("profilefields", gocui.KeyEnter, gocui.ModNone, selectProfileField); err != nil {
		log.Panicln(err)
	}

	// ESC key (Cancel Profile Fields Selection)
	if err := g.SetKeybinding("profilefields", gocui.KeyEsc, gocui.ModNone, cancelProfileEdit); err != nil {
		log.Panicln(err)
	}

	// Arrow keys for navigation
	if err := g.SetKeybinding("profilefields", gocui.KeyArrowDown, gocui.ModNone, cursorDown); err != nil {
		log.Panicln(err)
	}
	if err := g.SetKeybinding("profilefields", gocui.KeyArrowUp, gocui.ModNone, cursorUp); err != nil {
		log.Panicln(err)
	}

	/* field edit */
	// Enter key (Save Field Edit)
	if err := g.SetKeybinding("fieldedit", gocui.KeyEnter, gocui.ModNone, saveSingleField); err != nil {
		log.Panicln(err)
	}

	// ESC key (Cancel Field Edit)
	if err := g.SetKeybinding("fieldedit", gocui.KeyEsc, gocui.ModNone, cancelFieldEdit); err != nil {
		log.Panicln(err)
	}

	/* DM relays list */
	// n key (Add DM Relay)
	if err := g.SetKeybinding("dmrelayslist", rune(0x6e), gocui.ModNone, addDMRelay); err != nil {
		log.Panicln(err)
	}

	// d key (Delete DM Relay)
	if err := g.SetKeybinding("dmrelayslist", rune(0x64), gocui.ModNone, deleteDMRelay); err != nil {
		log.Panicln(err)
	}

	// s key (Save DM Relays)
	if err := g.SetKeybinding("dmrelayslist", rune(0x73), gocui.ModNone, saveDMRelaysChanges); err != nil {
		log.Panicln(err)
	}

	// ESC key (Cancel DM Relays Edit)
	if err := g.SetKeybinding("dmrelayslist", gocui.KeyEsc, gocui.ModNone, cancelDMRelaysEdit); err != nil {
		log.Panicln(err)
	}

	// Arrow keys for navigation
	if err := g.SetKeybinding("dmrelayslist", gocui.KeyArrowDown, gocui.ModNone, cursorDown); err != nil {
		log.Panicln(err)
	}
	if err := g.SetKeybinding("dmrelayslist", gocui.KeyArrowUp, gocui.ModNone, cursorUp); err != nil {
		log.Panicln(err)
	}

	/* Add DM relay */
	// Enter key (Save New DM Relay)
	if err := g.SetKeybinding("adddmrelay", gocui.KeyEnter, gocui.ModNone, saveNewDMRelay); err != nil {
		log.Panicln(err)
	}

	// ESC key (Cancel Add DM Relay)
	if err := g.SetKeybinding("adddmrelay", gocui.KeyEsc, gocui.ModNone, cancelAddDMRelay); err != nil {
		log.Panicln(err)
	}

	/* Remove old DM relays edit keybindings - these are no longer needed */

	/* posting view */
	//cancel key
	if err := g.SetKeybinding("v5", gocui.KeyEsc, gocui.ModNone, cancelInput); err != nil {
		log.Panicln(err)
	}

	// tab key
	if err := g.SetKeybinding("v5", gocui.KeyTab, gocui.ModNone, confirmPostInput); err != nil {
		log.Panicln(err)
	}

	// x key (switch theme)
	if err := g.SetKeybinding("", rune(0x78), gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		return switchTheme(g)
	}); err != nil {
		log.Panicln(err)
	}

	return nil
}
