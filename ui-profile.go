package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/awesome-gocui/gocui"
	"github.com/nbd-wtf/go-nostr"
)

// profileMenu opens a profile menu for viewing and editing profile metadata and DM relays
func profileMenu(g *gocui.Gui, v *gocui.View) error {
	maxX, maxY := g.Size()
	if v, err := g.SetView("profile", maxX/2-40, maxY/2-10, maxX/2+40, maxY/2+10, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}

		// Get active account
		var account Account
		DB.Where("active = ?", true).First(&account)
		if account.Pubkey == "" {
			return fmt.Errorf("no active account found")
		}

		// Get metadata for the active account
		var metadata Metadata
		DB.Where("pubkey_hex = ?", account.Pubkey).First(&metadata)

		v.Title = "Profile Menu - [e]dit Metadata - [d]m Relays - [ESC]Cancel"
		v.Highlight = true
		v.SelBgColor = gocui.ColorGreen
		v.SelFgColor = gocui.ColorBlack
		v.Editable = false

		// Display profile information
		fmt.Fprintf(v, "Profile for: %s\n\n", account.PubkeyNpub)
		fmt.Fprintf(v, "Name: %s\n", metadata.Name)
		fmt.Fprintf(v, "Display Name: %s\n", metadata.DisplayName)
		fmt.Fprintf(v, "About: %s\n", metadata.About)
		fmt.Fprintf(v, "NIP-05: %s\n", metadata.Nip05)
		fmt.Fprintf(v, "Website: %s\n", metadata.Website)
		fmt.Fprintf(v, "Lightning Address: %s\n", metadata.Lud16)
		fmt.Fprintf(v, "Last Updated: %s\n\n", metadata.MetadataUpdatedAt.Format("2006-01-02 15:04:05"))

		fmt.Fprintf(v, "DM Relays:\n")
		var dmRelays []DMRelay
		DB.Where("pubkey_hex = ?", account.Pubkey).Find(&dmRelays)
		if len(dmRelays) == 0 {
			fmt.Fprintf(v, "  No DM relays configured\n")
		} else {
			for _, relay := range dmRelays {
				fmt.Fprintf(v, "  %s\n", relay.Url)
			}
		}

		if _, err := g.SetCurrentView("profile"); err != nil {
			return err
		}
	}
	return nil
}

// editProfileMetadata opens a form for editing profile metadata
func editProfileMetadata(g *gocui.Gui, v *gocui.View) error {
	maxX, maxY := g.Size()
	g.DeleteView("profile")
	if v, err := g.SetView("profileedit", maxX/2-40, maxY/2-10, maxX/2+40, maxY/2+10, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}

		// Get active account
		var account Account
		DB.Where("active = ?", true).First(&account)
		if account.Pubkey == "" {
			return fmt.Errorf("no active account found")
		}

		// Get metadata for the active account
		var metadata Metadata
		DB.Where("pubkey_hex = ?", account.Pubkey).First(&metadata)

		v.Title = "Edit Profile Metadata - [Enter]Save - [ESC]Cancel"
		v.Highlight = true
		v.SelBgColor = gocui.ColorGreen
		v.SelFgColor = gocui.ColorBlack
		v.Editable = true
		v.KeybindOnEdit = true

		// Pre-populate the form with existing metadata
		metadataMap := map[string]string{
			"name":         metadata.Name,
			"display_name": metadata.DisplayName,
			"about":        metadata.About,
			"nip05":        metadata.Nip05,
			"website":      metadata.Website,
			"lud16":        metadata.Lud16,
		}

		jsonData, _ := json.MarshalIndent(metadataMap, "", "  ")
		fmt.Fprintf(v, "%s", jsonData)

		if _, err := g.SetCurrentView("profileedit"); err != nil {
			return err
		}
	}
	return nil
}

// saveProfileMetadata saves the edited profile metadata
func saveProfileMetadata(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		// Get active account
		var account Account
		DB.Where("active = ?", true).First(&account)
		if account.Pubkey == "" {
			g.DeleteView("profileedit")
			return fmt.Errorf("no active account found")
		}

		// Parse the JSON content
		var metadataMap map[string]string
		jsonContent := v.Buffer()
		if err := json.Unmarshal([]byte(jsonContent), &metadataMap); err != nil {
			TheLog.Printf("Error parsing metadata JSON: %v", err)
			g.DeleteView("profileedit")
			return err
		}

		// Get the private key
		sk := Decrypt(string(Password), account.Privatekey)

		// Create a new metadata event
		ev := nostr.Event{
			Kind:      0,
			PubKey:    account.Pubkey,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Content:   jsonContent,
		}

		// Sign the event
		err := ev.Sign(sk)
		if err != nil {
			TheLog.Printf("Error signing metadata event: %v", err)
			g.DeleteView("profileedit")
			return err
		}

		// Update local database
		updates := map[string]interface{}{
			"name":                metadataMap["name"],
			"display_name":        metadataMap["display_name"],
			"about":               metadataMap["about"],
			"nip05":               metadataMap["nip05"],
			"website":             metadataMap["website"],
			"lud16":               metadataMap["lud16"],
			"metadata_updated_at": time.Now(),
			"raw_json_content":    jsonContent,
		}

		if err := DB.Model(&Metadata{}).Where("pubkey_hex = ?", account.Pubkey).Updates(updates).Error; err != nil {
			TheLog.Printf("Error updating metadata in database: %v", err)
		}

		// Publish to relays
		TheLog.Println("Publishing metadata to relays...")
		// this could publish!!
		/*
			for _, relay := range nostrRelays {
				_, err := relay.Publish(ev)
				if err != nil {
					TheLog.Printf("Error publishing metadata to relay %s: %v", relay.URL, err)
				}
			}
		*/

		g.DeleteView("profileedit")
		profileMenu(g, v)
	}
	return nil
}

// cancelProfileEdit cancels profile metadata editing
func cancelProfileEdit(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("profileedit")
	profileMenu(g, v)
	return nil
}

// editDMRelays opens a form for editing DM relays
func editDMRelays(g *gocui.Gui, v *gocui.View) error {
	maxX, maxY := g.Size()
	g.DeleteView("profile")
	if v, err := g.SetView("dmrelaysedit", maxX/2-40, maxY/2-10, maxX/2+40, maxY/2+10, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}

		// Get active account
		var account Account
		DB.Where("active = ?", true).First(&account)
		if account.Pubkey == "" {
			return fmt.Errorf("no active account found")
		}

		// Get DM relays for the active account
		var dmRelays []DMRelay
		DB.Where("pubkey_hex = ?", account.Pubkey).Find(&dmRelays)

		v.Title = "Edit DM Relays - [Enter]Save - [ESC]Cancel"
		v.Highlight = true
		v.SelBgColor = gocui.ColorGreen
		v.SelFgColor = gocui.ColorBlack
		v.Editable = true
		v.KeybindOnEdit = true

		// Instructions
		fmt.Fprintf(v, "# Enter one relay URL per line\n")
		fmt.Fprintf(v, "# Example: wss://relay.example.com\n\n")

		// Pre-populate with existing DM relays
		for _, relay := range dmRelays {
			fmt.Fprintf(v, "%s\n", relay.Url)
		}

		if _, err := g.SetCurrentView("dmrelaysedit"); err != nil {
			return err
		}
	}
	return nil
}

// saveDMRelays saves the edited DM relays
func saveDMRelays(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		// Get active account
		var account Account
		DB.Where("active = ?", true).First(&account)
		if account.Pubkey == "" {
			g.DeleteView("dmrelaysedit")
			return fmt.Errorf("no active account found")
		}

		// Parse the relay URLs
		content := v.Buffer()
		lines := strings.Split(content, "\n")
		var relayUrls []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				relayUrls = append(relayUrls, line)
			}
		}

		// Get the private key
		sk := Decrypt(string(Password), account.Privatekey)

		// Create a new DM relay list event
		ev := nostr.Event{
			Kind:      10050,
			PubKey:    account.Pubkey,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Content:   "",
		}

		// Add relay tags
		for _, url := range relayUrls {
			ev.Tags = append(ev.Tags, nostr.Tag{"relay", url})
		}

		// Sign the event
		err := ev.Sign(sk)
		if err != nil {
			TheLog.Printf("Error signing DM relay list event: %v", err)
			g.DeleteView("dmrelaysedit")
			return err
		}

		// Update local database
		DB.Where("pubkey_hex = ?", account.Pubkey).Delete(&DMRelay{})
		for _, url := range relayUrls {
			dmRelay := DMRelay{
				PubkeyHex: account.Pubkey,
				Url:       url,
			}
			if err := DB.Create(&dmRelay).Error; err != nil {
				TheLog.Printf("Error saving DM relay to database: %v", err)
			}
		}

		// Publish to relays
		TheLog.Println("Publishing DM relay list to relays...")
		// this could publish!!
		/*
			for _, relay := range nostrRelays {
				_, err := relay.Publish(ev)
				if err != nil {
					TheLog.Printf("Error publishing DM relay list to relay %s: %v", relay.URL, err)
				}
			}
		*/

		g.DeleteView("dmrelaysedit")
		profileMenu(g, v)
	}
	return nil
}

// cancelDMRelaysEdit cancels DM relays editing
func cancelDMRelaysEdit(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("dmrelaysedit")
	profileMenu(g, v)
	return nil
}

// cancelProfile closes the profile menu
func cancelProfile(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("profile")
	g.SetCurrentView("v2")
	return nil
}
