package main

import (
	"fmt"
	"time"

	"github.com/awesome-gocui/gocui"
	"github.com/nbd-wtf/go-nostr/nip19"
)

// current page?
var displayV2Meta []Metadata

func refreshV2Conversations(g *gocui.Gui, v *gocui.View) error {
	v2, _ := g.View("v2")
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
	v2.Title = "Pubkeys we have conversations with:"
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

	// Reset cursor to first line if needed
	if _, cy := v2.Cursor(); cy < 0 {
		v2.SetCursor(0, 0)
		v2.SetHighlight(0, true)
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
		DB.Model(&m).Association("Follows").Find(&curFollows, "name LIKE ? OR nip05 LIKE ?", searchTerm, searchTerm)
		v2.Title = fmt.Sprintf("search (%s)", searchTerm)
	} else {
		// Get all follows
		assocError := DB.Model(&m).Association("Follows").Find(&curFollows)
		if assocError != nil {
			TheLog.Printf("error getting follows for account: %s", assocError)
		}
		v2.Title = fmt.Sprintf("follows (%d)", len(v2Meta))
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
