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
	// Position the view from the top of the screen (y=0) to above the keybinds view (v5)
	if v, err := g.SetView("profile", maxX/2-40, 0, maxX/2+40, maxY-7, 0); err != nil {
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

		v.Title = "Profile Menu"
		v.Highlight = true
		v.SelBgColor = activeTheme.HighlightBg
		v.SelFgColor = activeTheme.HighlightFg
		v.Editable = false
		v.BgColor = activeTheme.Bg
		v.FgColor = activeTheme.Fg

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

		// Update the v5 keybinds view
		updateProfileKeybindsView(g)
	}
	return nil
}

// exitProfileMenu closes the profile menu and returns to the main view
func exitProfileMenu(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("profile")
	// Restore the main keybinds view
	updateKeybindsView(g)
	// Set focus back to the main view (v2)
	g.SetCurrentView("v2")
	return nil
}

// editProfileMetadata opens a form for editing profile metadata
func editProfileMetadata(g *gocui.Gui, v *gocui.View) error {
	maxX, maxY := g.Size()
	g.DeleteView("profile")
	g.DeleteView("profilefields")
	// Position the view from the top of the screen (y=0) to above the keybinds view (v5)
	if v, err := g.SetView("profilefields", maxX/2-40, 0, maxX/2+40, maxY-7, 0); err != nil {
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

		v.Title = "Select Field to Edit"
		v.Highlight = true
		v.SelBgColor = activeTheme.HighlightBg
		v.SelFgColor = activeTheme.HighlightFg
		v.Editable = false
		v.BgColor = activeTheme.Bg
		v.FgColor = activeTheme.Fg

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

		// Update the v5 keybinds view
		updateProfileFieldsKeybindsView(g)
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

	g.DeleteView("profilefields")
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
	case 7: // Save All Changes
		return saveProfileChanges(g, v)
	case 8: // Cancel
		g.DeleteView("profilefields")
		// Restore the main keybinds view
		updateKeybindsView(g)
		// Set focus back to the main view (v2)
		g.SetCurrentView("v2")
		return nil
	}

	return nil
}

var currentEditingField string

// editSingleField opens an editor for a single metadata field
func editSingleField(g *gocui.Gui, fieldName string, currentValue string) error {
	maxX, maxY := g.Size()
	g.DeleteView("profilefields")

	// Position the view from the top of the screen (y=0) to above the keybinds view (v5)
	if v, err := g.SetView("fieldedit", maxX/2-40, 0, maxX/2+40, maxY-7, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}

		v.Title = fmt.Sprintf("Edit %s", fieldName)
		v.Highlight = false
		v.Editable = true
		v.KeybindOnEdit = true
		v.Wrap = true
		v.BgColor = activeTheme.Bg
		v.FgColor = activeTheme.Fg

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

		// Update the v5 keybinds view
		updateFieldEditKeybindsView(g)
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

	if err := DB.Where("pubkey_hex = ?", account.Pubkey).First(&Metadata{}).Error; err != nil {
		DB.Create(&Metadata{
			PubkeyHex: account.Pubkey,
		})
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

	// JUST update the database here..
	// publish is a separate option

	g.DeleteView("fieldedit")
	// If returning to the profile fields menu, update those keybinds
	return editProfileMetadata(g, v)
}

// cancelFieldEdit cancels editing a field and returns to the profile fields menu
func cancelFieldEdit(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("fieldedit")
	// Return to the profile fields menu
	return editProfileMetadata(g, v)
}

// saveProfileChanges saves all profile changes and publishes to relays
func saveProfileChanges(g *gocui.Gui, v *gocui.View) error {
	TheLog.Printf("SANITY")

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
		} else {
			TheLog.Printf("Published metadata to relay: %s", relay.URL)
		}
	}

	// Delete the profile fields view and return to the main view
	g.DeleteView("profilefields")
	// Restore the main keybinds view
	updateKeybindsView(g)
	return nil
}

// cancelProfileEdit cancels profile metadata editing
func cancelProfileEdit(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("profilefields")
	// Restore the main keybinds view
	updateKeybindsView(g)
	// Set focus back to the main view (v2)
	g.SetCurrentView("v2")
	return nil
}

// editDMRelays opens a form for editing DM relays
func editDMRelays(g *gocui.Gui, v *gocui.View) error {
	maxX, maxY := g.Size()
	g.DeleteView("profile")
	g.DeleteView("dmrelayslist")
	// Position the view from the top of the screen (y=0) to above the keybinds view (v5)
	if v, err := g.SetView("dmrelayslist", maxX/2-40, 0, maxX/2+40, maxY-7, 0); err != nil {
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

		v.Title = "DM Relays"
		v.Highlight = true
		v.SelBgColor = activeTheme.HighlightBg
		v.SelFgColor = activeTheme.HighlightFg
		v.Editable = false
		v.BgColor = activeTheme.Bg
		v.FgColor = activeTheme.Fg

		// Display DM relays
		if len(dmRelays) == 0 {
			fmt.Fprintf(v, "No DM relays configured\n")
		} else {
			for _, relay := range dmRelays {
				fmt.Fprintf(v, "%s\n", relay.Url)
			}
		}

		if _, err := g.SetCurrentView("dmrelayslist"); err != nil {
			return err
		}

		// Update the v5 keybinds view
		updateDMRelaysKeybindsView(g)
	}
	return nil
}

// exitDMRelaysList closes the DM relays list and returns to the main view
func exitDMRelaysList(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("dmrelayslist")
	// Restore the main keybinds view
	updateKeybindsView(g)
	return nil
}

// addDMRelay opens a form to add a new DM relay
func addDMRelay(g *gocui.Gui, v *gocui.View) error {
	maxX, maxY := g.Size()
	g.DeleteView("dmrelayslist")
	// Position the view from the top of the screen (y=0) to above the keybinds view (v5)
	if v, err := g.SetView("adddmrelay", maxX/2-40, 0, maxX/2+40, maxY-7, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}

		v.Title = "Add DM Relay"
		v.Highlight = false
		v.Editable = true
		v.KeybindOnEdit = true
		v.Wrap = true
		v.BgColor = activeTheme.Bg
		v.FgColor = activeTheme.Fg

		// Set cursor
		v.SetOrigin(0, 0)
		v.SetCursor(0, 1)

		if _, err := g.SetCurrentView("adddmrelay"); err != nil {
			return err
		}

		// Update the v5 keybinds view
		updateAddDMRelayKeybindsView(g)
	}
	return nil
}

// saveNewDMRelay saves a new DM relay and returns to the DM relays list
func saveNewDMRelay(g *gocui.Gui, v *gocui.View) error {
	// Get the relay URL from the input
	relayUrl := strings.TrimSpace(v.Buffer())
	if relayUrl == "" {
		return nil
	}

	// Get active account
	var account Account
	DB.Where("active = ?", true).First(&account)
	if account.Pubkey == "" {
		return fmt.Errorf("no active account found")
	}

	// Create a new DM relay
	dmRelay := DMRelay{
		PubkeyHex: account.Pubkey,
		Url:       relayUrl,
	}

	// Save to database
	if err := DB.Create(&dmRelay).Error; err != nil {
		TheLog.Printf("Error creating DM relay: %v", err)
		return err
	}

	// Delete the add relay view and return to the DM relays list
	g.DeleteView("adddmrelay")
	// Return to the DM relays list with the correct keybinds
	return editDMRelays(g, v)
}

// cancelAddDMRelay cancels adding a new DM relay
func cancelAddDMRelay(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("adddmrelay")
	// Return to the DM relays list
	return editDMRelays(g, v)
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

		relayURL := lines[cy]
		TheLog.Printf("deleting DM relay %s", relayURL)

		// Get active account
		var account Account
		DB.Where("active = ?", true).First(&account)
		if account.Pubkey == "" {
			return fmt.Errorf("no active account found")
		}

		// Delete the relay from the database
		rows := DB.Where("pubkey_hex = ? AND url = ?", account.Pubkey, relayURL).Delete(&DMRelay{}).RowsAffected
		if rows == 0 {
			TheLog.Printf("no rows deleted!")

		}

		// Refresh the relay list
		return editDMRelays(g, v)
	}
	return nil
}

// saveDMRelaysChanges saves changes to DM relays and publishes to relays
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

	// Create relay list for kind 10050 event using tags
	var tags []nostr.Tag
	for _, relay := range dmRelays {
		tags = append(tags, nostr.Tag{"relay", relay.Url})
	}

	// Get the private key
	sk := Decrypt(string(Password), account.Privatekey)

	// Create a new relay list event (kind 10050)
	ev := nostr.Event{
		Kind:      10050,
		PubKey:    account.Pubkey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Content:   "", // Empty content as per the format
		Tags:      tags,
	}

	// Sign the event
	err := ev.Sign(sk)
	if err != nil {
		TheLog.Printf("Error signing relay list event: %v", err)
		return err
	}

	// Publish to relays
	TheLog.Println("Publishing DM relay list to relays...")
	for _, relay := range nostrRelays {
		ctx := context.Background()
		err := relay.Publish(ctx, ev)
		if err != nil {
			TheLog.Printf("Error publishing relay list to relay %s: %v", relay.URL, err)
		} else {
			TheLog.Printf("Published relay list to relay: %s", relay.URL)
		}
	}

	// Delete the DM relays list view and return to the main view
	g.DeleteView("dmrelayslist")
	// Restore the main keybinds view
	updateKeybindsView(g)
	return nil
}

// cancelDMRelaysEdit cancels DM relays editing
func cancelDMRelaysEdit(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("dmrelayslist")
	// Restore the main keybinds view
	updateKeybindsView(g)
	return nil
}

// cancelProfile closes the profile menu
func cancelProfile(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("profile")
	// Restore the main keybinds view
	updateKeybindsView(g)
	// Set focus back to the main view (v2)
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
