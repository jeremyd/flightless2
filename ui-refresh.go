package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/awesome-gocui/gocui"
	"github.com/nbd-wtf/go-nostr/nip19"
)

// current page?
var displayV2Meta []Metadata

// wrapText wraps text to fit within a given width, preserving words
func wrapText(text string, width int) string {
	words := strings.Fields(strings.TrimSpace(text))
	if len(words) == 0 {
		return text
	}

	var wrapped strings.Builder
	line := words[0]
	spaceLeft := width - len(line)

	for _, word := range words[1:] {
		if len(word)+1 > spaceLeft {
			wrapped.WriteString(line + "\n")
			line = word
			spaceLeft = width - len(line)
		} else {
			line += " " + word
			spaceLeft -= len(word) + 1
		}
	}
	wrapped.WriteString(line)
	return wrapped.String()
}

func refreshAll(g *gocui.Gui, v *gocui.View) error {
	refreshV2Conversations(g, v)
	v2, _ := g.View("v2")
	_, cy := v2.Cursor()
	refreshV3(g, cy)
	refreshV4(g, cy)
	return nil
}

func refreshV2Conversations(g *gocui.Gui, v *gocui.View) error {
	v2, _ := g.View("v2")
	_, oldCursor := v2.Cursor()
	v2.Clear()

	// get the active account pubkey
	account := Account{}
	DB.Where("active = ?", true).First(&account)
	pubkey := account.Pubkey

	var allMessages []ChatMessage
	DB.Where("to_pubkey = ?", pubkey).Find(&allMessages)

	// group the messages by from_pubkey
	conversations := make(map[string][]ChatMessage)
	for _, message := range allMessages {
		conversations[message.FromPubkey] = append(conversations[message.FromPubkey], message)
	}

	// print the pubkeys we have conversations with
	newV2meta := []Metadata{}
	v2.Title = "Pubkey navigator"
	for pubkey, _ := range conversations {
		m := Metadata{}
		if err := DB.First(&m, "pubkey_hex = ?", pubkey).Error; err != nil {
			TheLog.Printf("error getting metadata for pubkey: %s, %s", pubkey, err)
			// creating db record
			m.PubkeyHex = pubkey
			m.PubkeyNpub, _ = nip19.EncodePublicKey(pubkey)
			m.MetadataUpdatedAt = time.Unix(0, 0)
			m.ContactsUpdatedAt = time.Unix(0, 0)
			DB.Create(&m)
		}
		newV2meta = append(newV2meta, m)
	}

	// sort by most recent chatMessage
	sort.Slice(newV2meta, func(i, j int) bool {
		conversationLatest1 := conversations[newV2meta[i].PubkeyHex]
		sort.Slice(conversationLatest1, func(i, j int) bool {
			return conversationLatest1[i].Timestamp.Before(conversationLatest1[j].Timestamp)
		})
		conversationLatest2 := conversations[newV2meta[j].PubkeyHex]
		sort.Slice(conversationLatest2, func(i, j int) bool {
			return conversationLatest2[i].Timestamp.Before(conversationLatest2[j].Timestamp)
		})
		return conversationLatest1[len(conversationLatest1)-1].Timestamp.After(conversationLatest2[len(conversationLatest2)-1].Timestamp)
	})
	v2Meta = newV2meta

	_, vSizeY := v2.Size()
	maxDisplay := vSizeY - 1

	// Calculate the slice of metadata to display based on current offset
	endIdx := CurrOffset + maxDisplay
	if endIdx > len(v2Meta) {
		endIdx = len(v2Meta)
	}
	displayV2Meta = v2Meta[CurrOffset:endIdx]

	// Display the metadata
	for _, metadata := range displayV2Meta {
		if metadata.Nip05 != "" {
			fmt.Fprintf(v2, "%-30s %-30s\n", metadata.Name, metadata.Nip05)
		} else if metadata.Name != "" {
			fmt.Fprintf(v2, "%-30s\n", metadata.Name)
		} else if metadata.DisplayName != "" {
			fmt.Fprintf(v2, "%-30s\n", metadata.DisplayName)
		} else {
			fmt.Fprintf(v2, "%-30s\n", metadata.PubkeyHex)
		}
	}

	v2.SetCursor(0, oldCursor)

	/*
		// Reset cursor to first line if needed
		if cursor < 0 {
			cursor = 0
			v2.SetCursor(0, 0)
			v2.SetHighlight(0, true)
		}
	*/

	return nil
}

func refreshV3(g *gocui.Gui, cy int) error {
	v3, _ := g.View("v3")
	v3.Clear()
	if len(displayV2Meta) != 0 && cy <= len(displayV2Meta) {
		var account Account
		DB.First(&account, "active = ?", true)
		var toMe []ChatMessage
		var fromMe []ChatMessage
		if err := DB.Find(&toMe, "from_pubkey = ? AND to_pubkey = ?", displayV2Meta[cy].PubkeyHex, account.Pubkey).Error; err != nil {
			TheLog.Printf("error getting conversation messages: %s", err)
			return err
		}
		if err := DB.Find(&fromMe, "from_pubkey = ? AND to_pubkey = ?", account.Pubkey, displayV2Meta[cy].PubkeyHex).Error; err != nil {
			TheLog.Printf("error getting conversation messages: %s", err)
			return err
		}
		// Example combining messages from different sources
		var allMessages []ChatMessage
		allMessages = append(allMessages, toMe...)
		allMessages = append(allMessages, fromMe...)
		// Sort by timestamp
		sort.Slice(allMessages, func(i, j int) bool {
			return allMessages[i].Timestamp.Before(allMessages[j].Timestamp)
		})

		width, _ := v3.Size()
		// Account for borders and some padding
		contentWidth := width - 10

		var buffer strings.Builder
		for _, message := range allMessages {
			humanTime := message.Timestamp.Format("Jan _2 3:04 PM")
			if message.FromPubkey == displayV2Meta[cy].PubkeyHex {
				header := fmt.Sprintf("\x1b[1;40m%s (%s):\x1b[0m\n", displayV2Meta[cy].Name, humanTime)
				buffer.WriteString(header)
				wrappedContent := wrapText(message.Content, contentWidth)
				buffer.WriteString(wrappedContent)
				buffer.WriteString("\n\n")
			} else {
				header := fmt.Sprintf("\x1b[1;44m-> (%s)\x1b[0m\n", humanTime)
				buffer.WriteString(header)
				wrappedContent := wrapText(message.Content, contentWidth)
				buffer.WriteString(wrappedContent)
				buffer.WriteString("\n\n")
			}
		}
		v3.Write([]byte(buffer.String()))
	}
	return nil
}

func refreshV2(g *gocui.Gui, v *gocui.View) error {
	TheLog.Println("refreshing v2")
	v2, _ := g.View("v2")
	v2.Clear()

	// get the active account pubkey
	account := Account{}
	DB.Where("active = ?", true).First(&account)
	pubkey := account.Pubkey

	var curFollows []Metadata
	m := Metadata{}
	DB.Where("pubkey_hex = ?", pubkey).First(&m)

	// Handle search vs normal view
	if searchTerm != "" {
		// Search within follows
		if err := DB.Model(&m).Association("Follows").Find(&curFollows, "name LIKE ? OR nip05 LIKE ?", searchTerm, searchTerm); err != nil {
			TheLog.Printf("error searching follows: %s", err)
			return nil
		}
		v2.Title = fmt.Sprintf("follows search: %s (%d)", searchTerm, len(curFollows))
	} else {
		// Get all follows
		assocError := DB.Model(&m).Association("Follows").Find(&curFollows)
		if assocError != nil {
			TheLog.Printf("error getting follows for account: %s", assocError)
			return nil
		}
		v2.Title = fmt.Sprintf("all follows (%d)", len(curFollows))
	}

	// only display follows that have >0 DM relays
	v2MetaFiltered := []Metadata{}
	for _, follow := range curFollows {
		dmRelayCount := DB.Model(&follow).Association("DMRelays").Count()
		if dmRelayCount != 0 {
			v2MetaFiltered = append(v2MetaFiltered, follow)
		}
	}

	// sort by recent ChatMessages

	v2Meta = v2MetaFiltered

	_, vSizeY := v2.Size()
	maxDisplay := vSizeY - 1

	// Calculate the slice of metadata to display based on current offset
	endIdx := CurrOffset + maxDisplay
	if endIdx > len(v2Meta) {
		endIdx = len(v2Meta)
	}
	displayV2Meta = v2Meta[CurrOffset:endIdx]

	// Display the metadata
	for _, metadata := range displayV2Meta {
		if metadata.Nip05 != "" {
			fmt.Fprintf(v2, "%-30s %-30s\n", metadata.Name, metadata.Nip05)
		} else {
			fmt.Fprintf(v2, "%-30s\n", metadata.Name)
		}
	}

	// Reset cursor to first line if needed
	if _, cy := v2.Cursor(); cy < 0 {
		v2.SetCursor(0, 0)
		v2.SetHighlight(0, true)
	}

	return nil
}

func refreshV4(g *gocui.Gui, cursor int) error {
	v4, _ := g.View("v4")
	v4.Clear()

	myDMRelays := []DMRelay{}
	account := Account{}
	DB.Where("active = ?", true).First(&account)
	DB.Where("pubkey_hex = ?", account.Pubkey).Find(&myDMRelays)
	fmt.Fprintf(v4, "My DM relays:\n")
	for _, relay := range myDMRelays {
		fmt.Fprintf(v4, "%s\n", relay.Url)
	}

	if len(displayV2Meta) == 0 || cursor >= len(displayV2Meta) {
		return nil
	}
	curDMRelays := []DMRelay{}
	DB.Where("pubkey_hex = ?", displayV2Meta[cursor].PubkeyHex).Find(&curDMRelays)
	fmt.Fprintf(v4, "\n%s DM relays:\n", displayV2Meta[cursor].Name)
	for _, relay := range curDMRelays {
		fmt.Fprintf(v4, "%s\n", relay.Url)
	}

	var RelayStatuses []RelayStatus
	DB.Find(&RelayStatuses)
	fmt.Fprintf(v4, "\nConnected relays:\n")
	for _, relayStatus := range RelayStatuses {
		var shortStatus string
		if relayStatus.Status == "connection established" {
			shortStatus = "⌛✅"
		} else if relayStatus.Status == "connection established: EOSE" {
			shortStatus = "✅"
		} else if relayStatus.Status == "waiting" {
			shortStatus = "⌛"
		} else {
			shortStatus = "❌"
		}
		fmt.Fprintf(v4, "%s %s\n", shortStatus, relayStatus.Url)
	}

	return nil
}

func refreshV5(g *gocui.Gui, cursor int) error {
	v5, _ := g.View("v5")

	v5.Clear()
	v5.Title = fmt.Sprintf("TYPING TO %s:", displayV2Meta[cursor].Name)
	v5.Editable = true
	v5.Subtitle = "press (ENTER) twice -or- (TAB) to post - (ESC) to cancel"
	v5.FgColor = gocui.NewRGBColor(255, 255, 255)
	v5.BgColor = gocui.NewRGBColor(0, 0, 0)
	g.DeleteKeybinding("v5", gocui.KeyEnter, gocui.ModNone)
	v5.Editor = &messageEditor{gui: g}
	g.Cursor = true
	g.SetCurrentView("v5")

	return nil
}
