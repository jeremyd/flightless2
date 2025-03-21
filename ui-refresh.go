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

// 0 is the conversations results page, 1 is the search results page, 2 is the follows page
var v2MetaDisplay = 0

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
	return refreshAllViews(g, v)
}

func refreshV2Conversations(g *gocui.Gui, v *gocui.View) error {
	v2MetaDisplay = 0
	v2, _ := g.View("v2")
	_, oldCursor := v2.Cursor()
	v2.Clear()

	// get the active account pubkey
	account := Account{}
	DB.Where("active = ?", true).First(&account)
	pubkey := account.Pubkey

	// this does not show, messages that we sent ..
	var allMessages []ChatMessage
	DB.Where("to_pubkey = ?", pubkey).Find(&allMessages)

	// group the messages by from_pubkey
	conversations := make(map[string][]ChatMessage)
	for _, message := range allMessages {
		conversations[message.FromPubkey] = append(conversations[message.FromPubkey], message)
	}

	// print the pubkeys we have conversations with
	newV2meta := []Metadata{}
	v2.Title = fmt.Sprintf("Pubkey navigator - active conversations (%d)", len(conversations))
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

func refreshV2(g *gocui.Gui, v *gocui.View) error {
	v2, err := g.View("v2")
	if err != nil {
		return err
	}
	v2.Clear()

	var account Account
	DB.First(&account, "active = ?", true)
	var m Metadata
	DB.First(&m, "pubkey_hex = ?", account.Pubkey)

	var curFollows []Metadata

	if searchTerm != "" {
		// Search in all records
		if err := DB.Where("name LIKE ? OR nip05 LIKE ? OR display_name LIKE ?", searchTerm, searchTerm, searchTerm).Find(&curFollows).Error; err != nil {
			TheLog.Printf("error querying for all metadata: %s", err)
		}
		searchTermTrimmed := strings.Trim(searchTerm, "%")
		v2.Title = fmt.Sprintf("Pubkey navigator - search results: %s (%d)", searchTermTrimmed, len(curFollows))
	} else {
		// Get all records
		if err := DB.Find(&curFollows).Error; err != nil {
			TheLog.Printf("error querying for all metadata: %s", err)
		}
		v2.Title = fmt.Sprintf("Pubkey navigator - all records (%d)", len(curFollows))
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

	v2Meta = curFollows

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
		} else {
			fmt.Fprintf(v2, "%-30s\n", metadata.PubkeyHex)
		}
	}

	return nil
}

// refreshV2Follows displays only the follows in the v2 view
func refreshV2Follows(g *gocui.Gui, v *gocui.View) error {
	v2, err := g.View("v2")
	if err != nil {
		return err
	}
	v2.Clear()

	var account Account
	DB.First(&account, "active = ?", true)
	var m Metadata
	DB.First(&m, "pubkey_hex = ?", account.Pubkey)

	var curFollows []Metadata

	if searchTerm != "" {
		// Search in follows
		searchTermTrimmed := strings.Trim(searchTerm, "%")
		var follows []Metadata
		assocError := DB.Model(&m).Association("Follows").Find(&follows)
		if assocError != nil {
			TheLog.Printf("error getting follows for account: %s", assocError)
			return nil
		}

		// Then filter by search term
		for _, follow := range follows {
			if strings.Contains(strings.ToLower(follow.Name), strings.ToLower(searchTermTrimmed)) ||
				strings.Contains(strings.ToLower(follow.Nip05), strings.ToLower(searchTermTrimmed)) ||
				strings.Contains(strings.ToLower(follow.DisplayName), strings.ToLower(searchTermTrimmed)) {
				curFollows = append(curFollows, follow)
			}
		}
		v2.Title = fmt.Sprintf("Pubkey navigator - follows search: %s (%d)", searchTermTrimmed, len(curFollows))
	} else {
		// Get all follows
		assocError := DB.Model(&m).Association("Follows").Find(&curFollows)
		if assocError != nil {
			TheLog.Printf("error getting follows for account: %s", assocError)
			return nil
		}
		v2.Title = fmt.Sprintf("Pubkey navigator - follows (%d)", len(curFollows))
	}

	// only display follows that have >0 DM relays
	v2MetaFiltered := []Metadata{}
	for _, follow := range curFollows {
		dmRelayCount := DB.Model(&follow).Association("DMRelays").Count()
		if dmRelayCount != 0 {
			v2MetaFiltered = append(v2MetaFiltered, follow)
		}
	}

	// Use filtered follows if available
	if len(v2MetaFiltered) > 0 {
		v2Meta = v2MetaFiltered
		v2.Title = fmt.Sprintf("%s (filtered by has DM relays: %d)", v2.Title, len(v2MetaFiltered))
	} else {
		v2Meta = curFollows
	}

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
		} else {
			fmt.Fprintf(v2, "%-30s\n", metadata.PubkeyHex)
		}
	}

	return nil
}

func refreshV3(g *gocui.Gui, cy int) error {
	v3, err := g.View("v3")
	if err != nil {
		return err
	}
	v3.Clear()

	// Apply current theme colors
	v3.BgColor = uiColorBg
	v3.FgColor = uiColorFg
	v3.FrameColor = uiColorBorderFg
	v3.TitleColor = uiColorBorderFont
	/*
		if activeTheme.UseRoundedBorders {
			v3.FrameRunes = customFrameRunes
		}
	*/

	// If there are no items or cursor is out of bounds, return
	if len(displayV2Meta) == 0 || cy >= len(displayV2Meta) {
		return nil
	}

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
			header := fmt.Sprintf("\x1b[1;104m-> (%s)\x1b[0m\n", humanTime)
			buffer.WriteString(header)
			wrappedContent := wrapText(message.Content, contentWidth)
			buffer.WriteString(wrappedContent)
			buffer.WriteString("\n\n")
		}
	}
	v3.Write([]byte(buffer.String()))
	return nil
}

func refreshV4(g *gocui.Gui, cursor int) error {
	v4, err := g.View("v4")
	if err != nil {
		return err
	}
	v4.Clear()

	// Apply current theme colors
	v4.BgColor = uiColorBg
	v4.FgColor = uiColorFg
	v4.FrameColor = uiColorBorderFg
	v4.TitleColor = uiColorBorderFont
	/*
		if activeTheme.UseRoundedBorders {
			v4.FrameRunes = customFrameRunes
		}
	*/

	myDMRelays := []DMRelay{}
	account := Account{}
	DB.Where("active = ?", true).First(&account)
	DB.Where("pubkey_hex = ?", account.Pubkey).Find(&myDMRelays)
	fmt.Fprintf(v4, "My DM relays:\n")
	for _, relay := range myDMRelays {
		fmt.Fprintf(v4, "%s\n", relay.Url)
	}

	if len(displayV2Meta) == 0 || cursor >= len(displayV2Meta) {
		TheLog.Printf("cursor out of range: %d", cursor)
	} else {

		curDMRelays := []DMRelay{}
		DB.Where("pubkey_hex = ?", displayV2Meta[cursor].PubkeyHex).Find(&curDMRelays)
		fmt.Fprintf(v4, "\n%s DM relays:\n", displayV2Meta[cursor].Name)
		for _, relay := range curDMRelays {
			fmt.Fprintf(v4, "%s\n", relay.Url)
		}
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
	v5, err := g.View("v5")
	if err != nil {
		return err
	}

	// Apply current theme colors
	v5.BgColor = uiColorBg
	v5.FgColor = uiColorFg
	v5.FrameColor = uiColorBorderFg
	v5.TitleColor = uiColorBorderFont
	/*
		if activeTheme.UseRoundedBorders {
			v5.FrameRunes = customFrameRunes
		}
	*/

	v5.Clear()

	// Check if user is composing a message
	if isComposingMessage && len(displayV2Meta) > cursor {
		// Set up compose message view
		v5.Title = fmt.Sprintf("TYPING TO %s:", displayV2Meta[cursor].Name)
		v5.Editable = true
		v5.Subtitle = "press (ENTER) twice -or- (TAB) to post - (ESC) to cancel"
		v5.FgColor = gocui.NewRGBColor(255, 255, 255)
		v5.BgColor = gocui.NewRGBColor(0, 0, 0)
		g.DeleteKeybinding("v5", gocui.KeyEnter, gocui.ModNone)
		v5.Editor = &messageEditor{gui: g}
		g.Cursor = true
	} else {
		// Set up normal keybinds view
		v5.Title = "Keybinds"
		v5.Subtitle = ""
		v5.Editable = false
		g.Cursor = false

		// Update keybinds view
		updateKeybindsView(g)
	}

	return nil
}

// updateKeybindsView updates the keybinds view (v5) with the current keybinds
func updateKeybindsView(g *gocui.Gui) error {
	v5, err := g.View("v5")
	if err != nil {
		return err
	}

	v5.Clear()
	// Use the action highlight color (orange-yellow #ffaf00) instead of cyan
	ActionColor := fmt.Sprintf("\033[38;2;%d;%d;%dm%%s\033[0m", 0xff, 0xaf, 0x00)

	// First row of keybinds
	s := fmt.Sprintf("(%s)earch", fmt.Sprintf(ActionColor, "S"))
	q := fmt.Sprintf("(%s)uit", fmt.Sprintf(ActionColor, "Q"))
	r := fmt.Sprintf("(%s)efresh", fmt.Sprintf(ActionColor, "R"))
	t := fmt.Sprintf("(%s)next window", fmt.Sprintf(ActionColor, "TAB"))
	a := fmt.Sprintf("(%s)dd relay", fmt.Sprintf(ActionColor, "A"))

	fmt.Fprintf(v5, "%-40s%-40s%-40s%-40s%-40s\n", s, q, r, t, a)

	// Second row of keybinds
	z := fmt.Sprintf("(%s)ap", fmt.Sprintf(ActionColor, "Z"))
	d := fmt.Sprintf("(%s)elete relay", fmt.Sprintf(ActionColor, "D"))
	c := fmt.Sprintf("(%s)onfigure keys", fmt.Sprintf(ActionColor, "C"))
	fe := fmt.Sprintf("(%s)etch person", fmt.Sprintf(ActionColor, "F"))
	p := fmt.Sprintf("(%s)ubkey lookup", fmt.Sprintf(ActionColor, "P"))

	fmt.Fprintf(v5, "%-40s%-40s%-40s%-40s%-40s\n", z, d, c, fe, p)

	// Third row of keybinds - add profile menu and theme toggle
	tt := fmt.Sprintf("(%s)oggle view", fmt.Sprintf(ActionColor, "T"))
	w := fmt.Sprintf("(%s)write note", fmt.Sprintf(ActionColor, "ENTER"))
	m := fmt.Sprintf("(%s)anage profile", fmt.Sprintf(ActionColor, "M"))
	theme := fmt.Sprintf("(%s)witch theme: %s", fmt.Sprintf(ActionColor, "X"), activeTheme.Name)

	fmt.Fprintf(v5, "%-40s%-40s%-40s-%40s\n\n", w, tt, m, theme)

	return nil
}

// updateConfigKeybindsView updates the keybinds view (v5) with keybinds for the configuration menu
func updateConfigKeybindsView(g *gocui.Gui) error {
	v5, err := g.View("v5")
	if err != nil {
		return err
	}

	v5.Clear()
	// Use the action highlight color (orange-yellow #ffaf00) instead of cyan
	ActionColor := fmt.Sprintf("\033[38;2;%d;%d;%dm%%s\033[0m", 0xff, 0xaf, 0x00)

	// Configuration menu keybinds
	save := fmt.Sprintf("(%s)ave", fmt.Sprintf(ActionColor, "Enter"))
	cancel := fmt.Sprintf("(%s)ancel", fmt.Sprintf(ActionColor, "Esc"))

	fmt.Fprintf(v5, "%-40s%-40s\n", save, cancel)

	return nil
}

// updateMainConfigKeybindsView updates the keybinds view (v5) with keybinds for the main configuration menu
func updateMainConfigKeybindsView(g *gocui.Gui) error {
	v5, err := g.View("v5")
	if err != nil {
		return err
	}

	v5.Clear()
	// Use the action highlight color (orange-yellow #ffaf00) instead of cyan
	ActionColor := fmt.Sprintf("\033[38;2;%d;%d;%dm%%s\033[0m", 0xff, 0xaf, 0x00)

	// Main configuration menu keybinds
	use := fmt.Sprintf("(%s)se key", fmt.Sprintf(ActionColor, "Enter"))
	cancel := fmt.Sprintf("(%s)ancel", fmt.Sprintf(ActionColor, "Esc"))
	new := fmt.Sprintf("(%s)ew key", fmt.Sprintf(ActionColor, "n"))
	delete := fmt.Sprintf("(%s)elete key", fmt.Sprintf(ActionColor, "d"))
	generate := fmt.Sprintf("(%s)enerate key", fmt.Sprintf(ActionColor, "g"))
	reveal := fmt.Sprintf("(%s)rivate key reveal", fmt.Sprintf(ActionColor, "p"))

	fmt.Fprintf(v5, "%-30s%-30s%-30s\n", use, cancel, new)
	fmt.Fprintf(v5, "%-30s%-30s%-30s\n", delete, generate, reveal)

	return nil
}

// updatePrivateKeyRevealKeybindsView updates the keybinds view (v5) with keybinds for the private key reveal screen
func updatePrivateKeyRevealKeybindsView(g *gocui.Gui) error {
	v5, err := g.View("v5")
	if err != nil {
		return err
	}

	v5.Clear()
	// Use the action highlight color (orange-yellow #ffaf00) instead of cyan
	ActionColor := fmt.Sprintf("\033[38;2;%d;%d;%dm%%s\033[0m", 0xff, 0xaf, 0x00)

	// Private key reveal screen keybinds
	dismiss := fmt.Sprintf("(%s)ismiss", fmt.Sprintf(ActionColor, "Esc"))

	fmt.Fprintf(v5, "%-40s\n", dismiss)

	return nil
}

func refreshAllViews(g *gocui.Gui, v *gocui.View) error {
	// Refresh v2 based on current display mode
	if v2MetaDisplay == 0 {
		refreshV2Conversations(g, v)
	} else if v2MetaDisplay == 2 {
		refreshV2Follows(g, v)
	} else {
		refreshV2(g, v)
	}

	// Get current cursor position in v2
	v2, err := g.View("v2")
	if err != nil {
		return err
	}
	_, cy := v2.Cursor()

	// Apply theme to v2
	v2.BgColor = uiColorBg
	v2.FgColor = uiColorFg
	v2.FrameColor = uiColorBorderFg
	v2.TitleColor = uiColorBorderFont
	v2.SelBgColor = uiColorHighlightBg
	v2.SelFgColor = uiColorHighlightFg
	/*
		if activeTheme.UseRoundedBorders {
			v2.FrameRunes = customFrameRunes
		}
	*/

	// Refresh v3 and v4 with current cursor position
	refreshV3(g, cy)
	refreshV4(g, cy)

	// Refresh v5 (keybinds view)
	refreshV5(g, cy)

	return nil
}
