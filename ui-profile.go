package main

import (
	"context"
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
	if v, err := g.SetView("profilefields", maxX/2-40, maxY/2-10, maxX/2+40, maxY/2+10, 0); err != nil {
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

		v.Title = "Select Field to Edit - [Enter]Select - [ESC]Cancel"
		v.Highlight = true
		v.SelBgColor = gocui.ColorGreen
		v.SelFgColor = gocui.ColorBlack
		v.Editable = false

		// Display fields for selection
		fmt.Fprintf(v, "1. Name: %s\n", metadata.Name)
		fmt.Fprintf(v, "2. Display Name: %s\n", metadata.DisplayName)
		fmt.Fprintf(v, "3. About: %s\n", metadata.About)
		fmt.Fprintf(v, "4. NIP-05: %s\n", metadata.Nip05)
		fmt.Fprintf(v, "5. Website: %s\n", metadata.Website)
		fmt.Fprintf(v, "6. Lightning Address: %s\n", metadata.Lud16)
		fmt.Fprintf(v, "\n7. Save All Changes\n")
		fmt.Fprintf(v, "8. Cancel\n")

		if _, err := g.SetCurrentView("profilefields"); err != nil {
			return err
		}
	}
	return nil
}

// selectProfileField handles the selection of a field to edit
func selectProfileField(g *gocui.Gui, v *gocui.View) error {
	_, cy := v.Cursor()

	// Get active account
	var account Account
	DB.Where("active = ?", true).First(&account)
	if account.Pubkey == "" {
		return fmt.Errorf("no active account found")
	}

	// Get metadata for the active account
	var metadata Metadata
	DB.Where("pubkey_hex = ?", account.Pubkey).First(&metadata)

	// Handle selection based on cursor position
	switch cy {
	case 0: // Name
		return editSingleField(g, "Name", metadata.Name)
	case 1: // Display Name
		return editSingleField(g, "Display Name", metadata.DisplayName)
	case 2: // About
		return editSingleField(g, "About", metadata.About)
	case 3: // NIP-05
		return editSingleField(g, "NIP-05", metadata.Nip05)
	case 4: // Website
		return editSingleField(g, "Website", metadata.Website)
	case 5: // Lightning Address
		return editSingleField(g, "Lightning Address", metadata.Lud16)
	case 6: // Save All Changes
		return saveProfileChanges(g, v)
	case 7: // Cancel
		g.DeleteView("profilefields")
		return profileMenu(g, v)
	}

	return nil
}

var currentEditingField string

// editSingleField opens an editor for a single metadata field
func editSingleField(g *gocui.Gui, fieldName string, currentValue string) error {
	maxX, maxY := g.Size()
	g.DeleteView("profilefields")

	if v, err := g.SetView("fieldedit", maxX/2-40, maxY/2-3, maxX/2+40, maxY/2+3, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}

		v.Title = fmt.Sprintf("Edit %s - [Enter]Save - [ESC]Cancel", fieldName)
		v.Highlight = false
		v.Editable = true
		v.KeybindOnEdit = true
		v.Wrap = true

		// Store the field name for reference when saving
		v.SetOrigin(0, 0)
		v.SetCursor(0, 0)
		v.Clear()
		fmt.Fprintf(v, "%s", currentValue)

		// Store the field name in the global variable
		currentEditingField = fieldName

		if _, err := g.SetCurrentView("fieldedit"); err != nil {
			return err
		}
	}
	return nil
}

// saveSingleField saves the edited field value
func saveSingleField(g *gocui.Gui, v *gocui.View) error {
	// Get the field name we're editing from the global variable
	fieldName := currentEditingField
	if fieldName == "" {
		TheLog.Println("Error: No field name stored")
		g.DeleteView("fieldedit")
		return editProfileMetadata(g, v)
	}

	// Get the new value
	newValue := strings.TrimSpace(v.Buffer())

	// Get active account
	var account Account
	DB.Where("active = ?", true).First(&account)
	if account.Pubkey == "" {
		g.DeleteView("fieldedit")
		return fmt.Errorf("no active account found")
	}

	// Get metadata for the active account
	var metadata Metadata
	DB.Where("pubkey_hex = ?", account.Pubkey).First(&metadata)

	// Update the appropriate field in memory
	switch fieldName {
	case "Name":
		metadata.Name = newValue
	case "Display Name":
		metadata.DisplayName = newValue
	case "About":
		metadata.About = newValue
	case "NIP-05":
		metadata.Nip05 = newValue
	case "Website":
		metadata.Website = newValue
	case "Lightning Address":
		metadata.Lud16 = newValue
	}

	// Update the database
	updates := map[string]interface{}{}

	switch fieldName {
	case "Name":
		updates["name"] = newValue
	case "Display Name":
		updates["display_name"] = newValue
	case "About":
		updates["about"] = newValue
	case "NIP-05":
		updates["nip05"] = newValue
	case "Website":
		updates["website"] = newValue
	case "Lightning Address":
		updates["lud16"] = newValue
	}

	if len(updates) > 0 {
		if err := DB.Model(&Metadata{}).Where("pubkey_hex = ?", account.Pubkey).Updates(updates).Error; err != nil {
			TheLog.Printf("Error updating metadata in database: %v", err)
		}
	}

	// Also update metadata_updated_at
	if err := DB.Model(&Metadata{}).Where("pubkey_hex = ?", account.Pubkey).
		Update("metadata_updated_at", time.Now()).Error; err != nil {
		TheLog.Printf("Error updating metadata_updated_at in database: %v", err)
	}

	// Publish the updated metadata to relays
	// Create metadata map for the event
	metadataMap := map[string]string{
		"name":         metadata.Name,
		"display_name": metadata.DisplayName,
		"about":        metadata.About,
		"nip05":        metadata.Nip05,
		"website":      metadata.Website,
		"lud16":        metadata.Lud16,
	}

	// Convert to JSON
	jsonContent, err := json.Marshal(metadataMap)
	if err != nil {
		TheLog.Printf("Error creating metadata JSON: %v", err)
		return err
	}

	// Get the private key
	sk := Decrypt(string(Password), account.Privatekey)

	// Create a new metadata event
	ev := nostr.Event{
		Kind:      0,
		PubKey:    account.Pubkey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Content:   string(jsonContent),
	}

	// Sign the event
	err = ev.Sign(sk)
	if err != nil {
		TheLog.Printf("Error signing metadata event: %v", err)
		return err
	}

	// Publish to relays
	TheLog.Printf("Publishing metadata to relays after updating %s...", fieldName)
	for _, relay := range nostrRelays {
		ctx := context.Background()
		err := relay.Publish(ctx, ev)
		if err != nil {
			TheLog.Printf("Error publishing metadata to relay %s: %v", relay.URL, err)
		}
	}

	g.DeleteView("fieldedit")
	return editProfileMetadata(g, v)
}

// cancelFieldEdit cancels editing a single field
func cancelFieldEdit(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("fieldedit")
	return editProfileMetadata(g, v)
}

// saveProfileChanges saves all profile changes and publishes to relays
func saveProfileChanges(g *gocui.Gui, v *gocui.View) error {
	// Get active account
	var account Account
	DB.Where("active = ?", true).First(&account)
	if account.Pubkey == "" {
		return fmt.Errorf("no active account found")
	}

	// Get metadata for the active account
	var metadata Metadata
	DB.Where("pubkey_hex = ?", account.Pubkey).First(&metadata)

	// Create metadata map for the event
	metadataMap := map[string]string{
		"name":         metadata.Name,
		"display_name": metadata.DisplayName,
		"about":        metadata.About,
		"nip05":        metadata.Nip05,
		"website":      metadata.Website,
		"lud16":        metadata.Lud16,
	}

	// Convert to JSON
	jsonContent, err := json.Marshal(metadataMap)
	if err != nil {
		TheLog.Printf("Error creating metadata JSON: %v", err)
		return err
	}

	// Get the private key
	sk := Decrypt(string(Password), account.Privatekey)

	// Create a new metadata event
	ev := nostr.Event{
		Kind:      0,
		PubKey:    account.Pubkey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Content:   string(jsonContent),
	}

	// Sign the event
	err = ev.Sign(sk)
	if err != nil {
		TheLog.Printf("Error signing metadata event: %v", err)
		return err
	}

	// Update metadata_updated_at in the database
	if err := DB.Model(&Metadata{}).Where("pubkey_hex = ?", account.Pubkey).
		Update("metadata_updated_at", time.Now()).Error; err != nil {
		TheLog.Printf("Error updating metadata_updated_at in database: %v", err)
	}

	// Publish to relays
	TheLog.Println("Publishing metadata to relays...")
	for _, relay := range nostrRelays {
		ctx := context.Background()
		err := relay.Publish(ctx, ev)
		if err != nil {
			TheLog.Printf("Error publishing metadata to relay %s: %v", relay.URL, err)
		}
	}

	g.DeleteView("profilefields")
	return profileMenu(g, v)
}

// cancelProfileEdit cancels profile metadata editing
func cancelProfileEdit(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("profilefields")
	profileMenu(g, v)
	return nil
}

// editDMRelays opens a form for editing DM relays
func editDMRelays(g *gocui.Gui, v *gocui.View) error {
	maxX, maxY := g.Size()
	g.DeleteView("profile")
	if v, err := g.SetView("dmrelayslist", maxX/2-40, maxY/2-10, maxX/2+40, maxY/2+10, 0); err != nil {
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

		v.Title = "DM Relays - [n]ew - [d]elete - [s]ave - [ESC]Cancel"
		v.Highlight = true
		v.SelBgColor = gocui.ColorGreen
		v.SelFgColor = gocui.ColorBlack
		v.Editable = false

		// Display instructions
		fmt.Fprintf(v, "These relays will be used for direct messages.\n\n")

		// List existing DM relays
		for i, relay := range dmRelays {
			fmt.Fprintf(v, "%d. %s\n", i+1, relay.Url)
		}

		if len(dmRelays) == 0 {
			fmt.Fprintf(v, "No DM relays configured.\n")
		}

		if _, err := g.SetCurrentView("dmrelayslist"); err != nil {
			return err
		}
	}
	return nil
}

// addDMRelay opens a form to add a new DM relay
func addDMRelay(g *gocui.Gui, v *gocui.View) error {
	maxX, maxY := g.Size()
	if v, err := g.SetView("adddmrelay", maxX/2-30, maxY/2-2, maxX/2+30, maxY/2+2, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}

		v.Title = "Add a new DM Relay - [Enter]Save - [ESC]Cancel"
		v.Editable = true
		v.KeybindOnEdit = true
		v.Wrap = true

		// Make cursor visible
		v.Highlight = true
		v.SelFgColor = gocui.ColorWhite
		v.SelBgColor = gocui.ColorBlue

		fmt.Fprintf(v, "wss://")

		// Set cursor position after the "wss://" prefix
		v.SetCursor(6, 0)

		if _, err := g.SetCurrentView("adddmrelay"); err != nil {
			return err
		}
	}
	return nil
}

// saveNewDMRelay saves a new DM relay
func saveNewDMRelay(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		// Get the relay URL
		relayURL := strings.TrimSpace(v.Buffer())

		// Validate the URL
		if !validateRelayURL(relayURL) {
			// Show error message
			maxX, maxY := g.Size()
			if msgView, err := g.SetView("errormsg", maxX/2-20, maxY/2+3, maxX/2+20, maxY/2+5, 0); err != nil {
				if !errors.Is(err, gocui.ErrUnknownView) {
					return err
				}
				msgView.Title = "Error"
				fmt.Fprintf(msgView, "Invalid relay URL format")
				if _, err := g.SetCurrentView("errormsg"); err != nil {
					return err
				}

				// Add keybinding to dismiss error
				if err := g.SetKeybinding("errormsg", gocui.KeyEnter, gocui.ModNone, closeErrorMsg); err != nil {
					return err
				}
				if err := g.SetKeybinding("errormsg", gocui.KeyEsc, gocui.ModNone, closeErrorMsg); err != nil {
					return err
				}
			}
			return nil
		}

		// Get active account
		var account Account
		DB.Where("active = ?", true).First(&account)
		if account.Pubkey == "" {
			g.DeleteView("addrelay")
			return fmt.Errorf("no active account found")
		}

		// Check if relay already exists
		var existingRelay DMRelay
		result := DB.Where("pubkey_hex = ? AND url = ?", account.Pubkey, relayURL).First(&existingRelay)
		if result.Error == nil {
			// Relay already exists, show error
			maxX, maxY := g.Size()
			if msgView, err := g.SetView("errormsg", maxX/2-20, maxY/2+3, maxX/2+20, maxY/2+5, 0); err != nil {
				if !errors.Is(err, gocui.ErrUnknownView) {
					return err
				}
				msgView.Title = "Error"
				fmt.Fprintf(msgView, "Relay already exists")
				if _, err := g.SetCurrentView("errormsg"); err != nil {
					return err
				}

				// Add keybinding to dismiss error
				if err := g.SetKeybinding("errormsg", gocui.KeyEnter, gocui.ModNone, closeErrorMsg); err != nil {
					return err
				}
				if err := g.SetKeybinding("errormsg", gocui.KeyEsc, gocui.ModNone, closeErrorMsg); err != nil {
					return err
				}
			}
			return nil
		}

		// Add the relay to the database
		dmRelay := DMRelay{
			PubkeyHex: account.Pubkey,
			Url:       relayURL,
		}
		if err := DB.Create(&dmRelay).Error; err != nil {
			TheLog.Printf("Error saving DM relay to database: %v", err)
		}

		// Close the add relay view
		g.DeleteView("adddmrelay")
		editDMRelays(g, v)
		g.SetCurrentView("dmrelayslist")

		return nil
	}
	return nil
}

// cancelAddDMRelay cancels adding a new DM relay
func cancelAddDMRelay(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("addrelay")
	g.SetCurrentView("dmrelayslist")
	return nil
}

// closeErrorMsg closes the error message view
func closeErrorMsg(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("errormsg")
	g.SetCurrentView("addrelay")
	return nil
}

// deleteDMRelay deletes the selected DM relay
func deleteDMRelay(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		_, cy := v.Cursor()
		lines := strings.Split(v.Buffer(), "\n")

		// Skip the header lines
		cy = cy - 2
		if cy < 0 || cy >= len(lines)-2 {
			return nil
		}

		// Extract the relay URL from the line
		line := lines[cy+2]
		if !strings.Contains(line, ".") {
			return nil
		}

		parts := strings.SplitN(line, ". ", 2)
		if len(parts) < 2 {
			return nil
		}
		relayURL := parts[1]

		// Get active account
		var account Account
		DB.Where("active = ?", true).First(&account)
		if account.Pubkey == "" {
			return fmt.Errorf("no active account found")
		}

		// Delete the relay from the database
		DB.Where("pubkey_hex = ? AND url = ?", account.Pubkey, relayURL).Delete(&DMRelay{})

		// Refresh the relay list
		return editDMRelays(g, v)
	}
	return nil
}

// saveDMRelaysChanges saves all DM relay changes and publishes to relays
func saveDMRelaysChanges(g *gocui.Gui, v *gocui.View) error {
	// Get active account
	var account Account
	DB.Where("active = ?", true).First(&account)
	if account.Pubkey == "" {
		return fmt.Errorf("no active account found")
	}

	// Get DM relays for the active account
	var dmRelays []DMRelay
	DB.Where("pubkey_hex = ?", account.Pubkey).Find(&dmRelays)

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
	for _, relay := range dmRelays {
		ev.Tags = append(ev.Tags, nostr.Tag{"relay", relay.Url})
	}

	// Sign the event
	err := ev.Sign(sk)
	if err != nil {
		TheLog.Printf("Error signing DM relay list event: %v", err)
		return err
	}

	// Publish to relays
	TheLog.Println("Publishing DM relay list to relays...")
	for _, relay := range nostrRelays {
		ctx := context.Background()
		err := relay.Publish(ctx, ev)
		if err != nil {
			TheLog.Printf("Error publishing DM relay list to relay %s: %v", relay.URL, err)
		}
	}

	// we also need to start the stream from this new set of DM relays...

	g.DeleteView("dmrelayslist")
	return profileMenu(g, v)
}

// cancelDMRelaysEdit cancels DM relays editing
func cancelDMRelaysEdit(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("dmrelayslist")
	return profileMenu(g, v)
}

// cancelProfile closes the profile menu
func cancelProfile(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("profile")
	g.SetCurrentView("v2")
	return nil
}

// cursorDown moves the cursor down in a view
func cursorDown(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		cx, cy := v.Cursor()
		lineCount := strings.Count(v.Buffer(), "\n")
		if cy < lineCount {
			if err := v.SetCursor(cx, cy+1); err != nil {
				ox, oy := v.Origin()
				if err := v.SetOrigin(ox, oy+1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// cursorUp moves the cursor up in a view
func cursorUp(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		ox, oy := v.Origin()
		cx, cy := v.Cursor()
		if err := v.SetCursor(cx, cy-1); err != nil && oy > 0 {
			if err := v.SetOrigin(ox, oy-1); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateRelayURL checks if a URL is a valid relay URL
func validateRelayURL(url string) bool {
	// Basic validation - must start with wss:// or ws://
	if !strings.HasPrefix(url, "wss://") && !strings.HasPrefix(url, "ws://") {
		return false
	}

	// Must have some content after the protocol
	if len(strings.TrimPrefix(strings.TrimPrefix(url, "wss://"), "ws://")) < 1 {
		return false
	}

	return true
}
