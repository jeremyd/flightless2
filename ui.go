package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/awesome-gocui/gocui"
	"github.com/jeremyd/crusher17"
	"github.com/nbd-wtf/go-nostr"
)

var curViewNum int = 0
var selectableViews = []string{"v2", "v3", "v4"}
var v2Meta []Metadata
var searchTerm = ""
var followSearch = false
var CurrOffset = 0
var followPages []Metadata
var enterTwice = 0
var isComposingMessage = false // Global flag to track if user is composing a message

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
	maxX, maxY := g.Size()

	// Create a popup view with the exit message
	if v, err := g.SetView("exitMsg", maxX/2-30, maxY/2-1, maxX/2+30, maxY/2+1, 0); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}

		v.Title = "exiting gracefully"
		v.Wrap = true
		fmt.Fprintln(v, "closing relay connections...")
		g.Update(func(g *gocui.Gui) error {
			return nil
		})
	}

	// Set the current view to the exit message
	if _, err := g.SetCurrentView("exitMsg"); err != nil {
		return err
	}

	// Run the exit code immediately
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		TheLog.Println("Error finding process:", err)
		return err
	}

	// Clear the global GUI instance
	TheGui = nil

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

func search(g *gocui.Gui, v *gocui.View) error {
	maxX, maxY := g.Size()
	if v, err := g.SetView("msg", maxX/2-30, maxY/2, maxX/2+30, maxY/2+2, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}
		v.Title = "Search - [Enter] to confirm, [Esc] to cancel"
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
	// default to search view if it's in conversations
	if v2MetaDisplay == 0 {
		v2MetaDisplay = 1
	}
	// zero out the highlighted list
	CurrOffset = 0

	// Get search term from input
	searchInput := strings.TrimSpace(v.Buffer())

	// Format search term for SQL LIKE query
	if searchInput == "" {
		// If search is empty, clear the search term to show all results
		searchTerm = ""
	} else {
		searchTerm = "%" + searchInput + "%"
	}

	// Close search dialog
	if err := g.DeleteView("msg"); err != nil {
		return err
	}
	if _, err := g.SetCurrentView("v2"); err != nil {
		return err
	}

	// Use the refreshAllViews function to refresh all views with search results
	return refreshAllViews(g, v)
}

func toggleConversationFollows(g *gocui.Gui, v *gocui.View) error {
	// Cycle through the three display modes:
	// 0: Conversations
	// 1: All records
	// 2: Follows only
	if v2MetaDisplay == 0 {
		// Switch from conversations to all records
		v2MetaDisplay = 1
	} else if v2MetaDisplay == 1 {
		// Switch from all records to follows only
		v2MetaDisplay = 2
	} else {
		// Switch from follows only back to conversations
		v2MetaDisplay = 0
	}

	// Refresh all views with the new display mode
	return refreshAllViews(g, v)
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
	isComposingMessage = false
	NoticeColor := "\033[1;36m%s\033[0m"
	s := fmt.Sprintf("(%s)earch", fmt.Sprintf(NoticeColor, "S"))
	q := fmt.Sprintf("(%s)uit", fmt.Sprintf(NoticeColor, "Q"))
	r := fmt.Sprintf("(%s)efresh", fmt.Sprintf(NoticeColor, "R"))
	t := fmt.Sprintf("(%s)next window", fmt.Sprintf(NoticeColor, "TAB"))
	a := fmt.Sprintf("(%s)dd relay", fmt.Sprintf(NoticeColor, "A"))
	w := fmt.Sprintf("(%s)write note", fmt.Sprintf(NoticeColor, "ENTER"))

	fmt.Fprintf(v, "%-30s%-30s%-30s%-30s%-30s%-30s\n", s, q, r, t, a, w)
	z := fmt.Sprintf("(%s)ap", fmt.Sprintf(NoticeColor, "Z"))
	d := fmt.Sprintf("(%s)elete relay", fmt.Sprintf(NoticeColor, "D"))
	c := fmt.Sprintf("(%s)onfigure keys", fmt.Sprintf(NoticeColor, "C"))
	fe := fmt.Sprintf("(%s)etch person", fmt.Sprintf(NoticeColor, "F"))
	p := fmt.Sprintf("(%s)ubkey lookup", fmt.Sprintf(NoticeColor, "P"))
	tt := fmt.Sprintf("(%s)oggle view", fmt.Sprintf(NoticeColor, "T"))
	fmt.Fprintf(v, "%-30s%-30s%-30s%-30s%-30s%-30s\n\n", z, d, c, fe, p, tt)

	g.DeleteKeybinding("v5", gocui.KeyEnter, gocui.ModNone)
	g.SetCurrentView("v2")
	return nil
}

func confirmPostInput(g *gocui.Gui, v *gocui.View) error {
	v.Title = "Confirm Post? ENTER to post / ESC to cancel"
	v.BgColor = gocui.ColorGreen
	v.FgColor = gocui.ColorWhite
	v.Editable = false
	g.SetKeybinding("v5", gocui.KeyEnter, gocui.ModNone, postInput)
	return nil
}

func postInput(g *gocui.Gui, v *gocui.View) error {
	msg := strings.TrimSpace(v.Buffer())
	if msg == "" {
		return nil
	}

	v2, err := g.View("v2")
	if err != nil {
		return err
	}
	_, cy := v2.Cursor()
	if cy >= len(displayV2Meta) {
		return nil
	}

	m := displayV2Meta[cy]

	// Get active account
	account := Account{}
	DB.Where("active = ?", true).First(&account)

	// Create and store chat message in local DB
	//DB.Create(&ChatMessage{FromPubkey: account.Pubkey, ToPubkey: m.PubkeyHex, Content: msg})

	// Create and publish nostr event
	go func() {
		if account.Privatekey != "" {
			// Decrypt private key
			decryptedKey := Decrypt(string(Password), account.Privatekey)

			// Get sender's metadata and DM relays
			var senderMeta Metadata
			DB.Preload("DMRelays").Where("pubkey_hex = ?", account.Pubkey).First(&senderMeta)

			// Get recipient's metadata and DM relays
			var recipientMeta Metadata
			DB.Preload("DMRelays").Where("pubkey_hex = ?", m.PubkeyHex).First(&recipientMeta)

			// Collect all unique relays from both sender and receiver
			uniqueRelays := make(map[string]string) // map[url]relay_for_pubkey
			for _, relay := range senderMeta.DMRelays {
				uniqueRelays[relay.Url] = account.Pubkey
			}
			for _, relay := range recipientMeta.DMRelays {
				uniqueRelays[relay.Url] = m.PubkeyHex
			}

			// If no DM relays are set, use all connected relays
			if len(uniqueRelays) == 0 {
				for _, relay := range nostrRelays {
					if relay != nil {
						uniqueRelays[relay.URL] = account.Pubkey
					}
				}
			}

			if len(uniqueRelays) == 0 {
				TheLog.Printf("No relays available for sending message")
				return
			}

			// Convert unique relays to slice and use first relay as primary
			targetRelays := make([]string, 0, len(uniqueRelays))
			var primaryRelay string
			for url := range uniqueRelays {
				if primaryRelay == "" {
					primaryRelay = url
				}
				targetRelays = append(targetRelays, url)
			}

			// Create a GiftWrapEvent
			giftWrap := crusher17.GiftWrapEvent{
				SenderSecretKey: decryptedKey,
				SenderRelay:     primaryRelay,
				CreatedAt:       nostr.Now(),
				ReceiverPubkeys: map[string]string{
					m.PubkeyHex: primaryRelay,
				},
				Content:   msg,
				GiftWraps: make(map[string]string),
			}

			// Wrap the message
			err = giftWrap.Wrap()
			if err != nil {
				TheLog.Printf("Error wrapping message: %v", err)
				return
			}

			// Connect to and publish to all unique relays
			ctx := context.Background()
			for _, relayUrl := range targetRelays {
				// Check if we're already connected
				var isConnected bool
				for _, existingRelay := range nostrRelays {
					if existingRelay != nil && existingRelay.URL == relayUrl {
						// Publish both the sender's and receiver's giftwraps
						for _, wrappedEvent := range giftWrap.GiftWraps {
							var ev nostr.Event
							err := json.Unmarshal([]byte(wrappedEvent), &ev)
							if err != nil {
								TheLog.Printf("Error unmarshaling wrapped event: %v", err)
								continue
							}
							if err := existingRelay.Publish(ctx, ev); err != nil {
								TheLog.Printf("Error publishing giftwrap to existing relay %s: %v", relayUrl, err)
							} else {
								TheLog.Printf("Published giftwrap to existing relay %s", relayUrl)
							}
						}
						isConnected = true
						break
					}
				}

				// If not connected, connect and publish
				if !isConnected {
					relay, err := nostr.RelayConnect(ctx, relayUrl)
					if err != nil {
						fmt.Fprintf(v, "Failed to connect to relay %s: %v\n", relayUrl, err)
						TheLog.Printf("Failed to connect to relay %s: %v", relayUrl, err)
						continue
					}

					// Handle auth if needed
					if account.Privatekey != "" && checkRelayRequiresAuth(relayUrl) {
						err = relay.Auth(ctx, func(evt *nostr.Event) error {
							return evt.Sign(decryptedKey)
						})
						if err != nil {
							TheLog.Printf("Failed to authenticate with relay %s: %v", relayUrl, err)
						}
					}

					// Publish both the sender's and receiver's giftwraps
					for _, wrappedEvent := range giftWrap.GiftWraps {
						var ev nostr.Event
						err := json.Unmarshal([]byte(wrappedEvent), &ev)
						if err != nil {
							TheLog.Printf("Error unmarshaling wrapped event: %v", err)
							continue
						}
						if err := relay.Publish(ctx, ev); err != nil {
							TheLog.Printf("Error publishing giftwrap to new relay %s: %v", relayUrl, err)
						} else {
							TheLog.Printf("Published giftwrap to new relay %s", relayUrl)
						}
					}
					// no we want to disconnect from the relay i believe?
					//nostrRelays = append(nostrRelays, relay)
					relay.Close()
				}
			}
		}
	}()

	// Close input view
	cancelInput(g, v)
	refreshV3(g, cy)

	return nil
}

func cursorDownV2(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		cx, cy := v.Cursor()
		_, vSizeY := v.Size()

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

			// Refresh all views with the updated offset
			refreshAllViews(g, v)

			// Set highlight on first item
			v.SetHighlight(0, true)
			return nil
		}

		if cy >= (len(displayV2Meta) - 1) {
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

		// Refresh v3 and v4 with the new cursor position
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

			// Refresh all views with the updated offset
			refreshAllViews(g, v)

			// Move cursor to bottom of view unless we're at the start
			newY := vSizeY - 2
			if err := v.SetCursor(cx, newY); err != nil {
				ox, oy := v.Origin()
				if err := v.SetOrigin(ox, oy-1); err != nil {
					return err
				}
			}
			v.SetHighlight(newY, true)

			// Refresh v3 with the new cursor position
			refreshV3(g, newY)
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

		// Refresh v3 and v4 with the new cursor position
		refreshV3(g, cy-1)
		refreshV4(g, cy-1)
	}
	return nil
}

func cancelSearch(g *gocui.Gui, v *gocui.View) error {
	if err := g.DeleteView("msg"); err != nil {
		return err
	}
	if _, err := g.SetCurrentView("v2"); err != nil {
		return err
	}
	return nil
}
