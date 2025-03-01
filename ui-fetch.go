package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/awesome-gocui/gocui"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

func fetch(g *gocui.Gui, v *gocui.View) error {
	// Get the pubkey from the highlighted item in v2
	v2, _ := g.View("v2")
	_, cy := v2.Cursor()

	if len(displayV2Meta) == 0 || cy >= len(displayV2Meta) {
		TheLog.Println("out of bounds of the displayV2Meta", cy)
		return nil
	}
	pubkey := displayV2Meta[cy].PubkeyHex

	return showPersonData(g, pubkey)
}

func fetchByPubkey(g *gocui.Gui, v *gocui.View) error {
	maxX, maxY := g.Size()
	if v, err := g.SetView("fetchpubkey", maxX/2-30, maxY/2, maxX/2+30, maxY/2+2, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}
		v.Title = "Enter Pubkey/Npub"
		v.Editable = true
		v.KeybindOnEdit = true
		v.Clear()

		if _, err := g.SetCurrentView("fetchpubkey"); err != nil {
			return err
		}
	}
	return nil
}

func cancelFetchPubkey(g *gocui.Gui, v *gocui.View) error {
	if err := g.DeleteView("fetchpubkey"); err != nil {
		return err
	}
	if _, err := g.SetCurrentView("v2"); err != nil {
		return err
	}
	return nil
}

func closeFetchResults(g *gocui.Gui, v *gocui.View) error {
	if err := g.DeleteView("fetchresults"); err != nil {
		return err
	}
	if _, err := g.SetCurrentView("v2"); err != nil {
		return err
	}
	return nil
}

func doFetchByPubkey(g *gocui.Gui, v *gocui.View) error {
	// Get the input from the fetchpubkey view
	fetchView, fetchErr := g.View("fetchpubkey")
	if fetchErr != nil {
		return nil
	}

	pubkeyInput := strings.TrimSpace(fetchView.Buffer())

	// Check if input is empty
	if pubkeyInput == "" {
		if err := g.DeleteView("fetchpubkey"); err != nil {
			return err
		}
		if _, err := g.SetCurrentView("v2"); err != nil {
			return err
		}
		return nil
	}

	// Process the pubkey/npub input
	pubkey := pubkeyInput
	// Check if it's an npub and convert to hex if needed
	if strings.HasPrefix(pubkeyInput, "npub") {
		_, decodedPubkey, err := nip19.Decode(pubkeyInput)
		if err != nil {
			// Create results view to show error
			maxX, maxY := g.Size()
			if v, err := g.SetView("fetchresults", maxX/4, maxY/4, maxX*3/4, maxY*3/4, 0); err != nil {
				if !errors.Is(err, gocui.ErrUnknownView) {
					return err
				}
				v.Title = "Error"
				v.Editable = false
				v.Wrap = true
				v.Autoscroll = false
				v.Clear()
			}

			fetchResultsView, _ := g.View("fetchresults")
			fetchResultsView.Clear()
			fmt.Fprintf(fetchResultsView, "Error: Invalid npub format\n")
			fmt.Fprintf(fetchResultsView, "\nPress ESC to close this view\n")

			// Delete the fetchpubkey input view
			if err := g.DeleteView("fetchpubkey"); err != nil {
				return err
			}

			// Set the current view to the fetch results
			if _, err := g.SetCurrentView("fetchresults"); err != nil {
				return err
			}

			return nil
		}
		pubkey = decodedPubkey.(string)
	}

	// Delete the fetchpubkey input view
	if err := g.DeleteView("fetchpubkey"); err != nil {
		return err
	}

	return showPersonData(g, pubkey)
}

func showPersonData(g *gocui.Gui, pubkey string) error {
	// Create or get the fetch results view
	maxX, maxY := g.Size()
	if v, err := g.SetView("fetchresults", maxX/4, maxY/4, maxX*3/4, maxY*3/4, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}
		v.Title = "Person Data"
		v.Editable = false
		v.Wrap = true
		v.Autoscroll = true
		v.Clear()
	}

	fetchResultsView, _ := g.View("fetchresults")
	fetchResultsView.Clear()
	fetchResultsView.Autoscroll = true

	// Display a loading message
	fmt.Fprintf(fetchResultsView, "Fetching data for pubkey: %s\n", pubkey)
	fmt.Fprintf(fetchResultsView, "Please wait...\n")

	// Fetch data asynchronously
	go func() {
		// Fetch and process relay list
		fetchRelayList(g, pubkey)

		// Wait a moment for the fetch to complete
		time.Sleep(6 * time.Second)

		// Now display all the information
		g.Update(func(g *gocui.Gui) error {
			fetchResultsView, err := g.View("fetchresults")
			if err != nil {
				return err
			}

			fetchResultsView.Clear()

			// Query the database for the metadata
			var metadata Metadata
			result := DB.Where("pubkey_hex = ?", pubkey).First(&metadata)
			if result.Error != nil {
				fmt.Fprintf(fetchResultsView, "===== PROFILE METADATA =====\n")
				fmt.Fprintf(fetchResultsView, "No metadata found for this pubkey\n")
			} else {
				// Display the metadata
				fmt.Fprintf(fetchResultsView, "===== PROFILE METADATA =====\n")
				fmt.Fprintf(fetchResultsView, "Name: %s\n", metadata.Name)
				fmt.Fprintf(fetchResultsView, "Display Name: %s\n", metadata.DisplayName)
				fmt.Fprintf(fetchResultsView, "About: %s\n", metadata.About)
				fmt.Fprintf(fetchResultsView, "NIP-05: %s\n", metadata.Nip05)
				fmt.Fprintf(fetchResultsView, "Website: %s\n", metadata.Website)
				fmt.Fprintf(fetchResultsView, "Lightning Address: %s\n", metadata.Lud16)
				fmt.Fprintf(fetchResultsView, "Total Follows: %d\n", metadata.TotalFollows)
				fmt.Fprintf(fetchResultsView, "Last Updated: %s\n", metadata.MetadataUpdatedAt.Format("2006-01-02 15:04:05"))
			}

			// Display regular relay list
			var relays []RelayList
			result = DB.Where("pubkey_hex = ?", pubkey).Find(&relays)
			if result.Error != nil {
				TheLog.Printf("Error querying relay_list: %v", result.Error)
			}

			fmt.Fprintf(fetchResultsView, "\n===== RELAY LIST =====\n")
			if len(relays) > 0 {
				for _, relay := range relays {
					readStr := "✓"
					writeStr := "✓"
					if !relay.Read {
						readStr = "✗"
					}
					if !relay.Write {
						writeStr = "✗"
					}
					fmt.Fprintf(fetchResultsView, "  %s [Read: %s, Write: %s]\n", relay.Url, readStr, writeStr)
				}
			} else {
				fmt.Fprintf(fetchResultsView, "  None\n")
			}

			// Display DM relay list
			var dmRelays []DMRelay
			result = DB.Where("pubkey_hex = ?", pubkey).Find(&dmRelays)
			if result.Error != nil {
				TheLog.Printf("Error querying dm_relays: %v", result.Error)
			}

			fmt.Fprintf(fetchResultsView, "\n===== DM RELAY LIST =====\n")
			if len(dmRelays) > 0 {
				for _, relay := range dmRelays {
					fmt.Fprintf(fetchResultsView, "  %s\n", relay.Url)
				}
			} else {
				fmt.Fprintf(fetchResultsView, "  None\n")
			}

			fmt.Fprintf(fetchResultsView, "\nPress ESC to close this view\n")
			return nil
		})
	}()

	// Set the current view to the fetch results
	if _, err := g.SetCurrentView("fetchresults"); err != nil {
		return err
	}

	return nil
}

func fetchRelayList(g *gocui.Gui, pubkey string) {
	TheLog.Printf("Fetching relay list for pubkey: %s", pubkey)

	// Get relays for this pubkey from the database
	var relayList []RelayList
	result := DB.Where("pubkey_hex = ?", pubkey).Find(&relayList)
	if result.Error != nil {
		TheLog.Printf("Error querying relay_list: %v", result.Error)
	}

	// If no relays found for this pubkey, use the global relay list
	if len(relayList) == 0 {
		TheLog.Printf("No relays found for pubkey %s, using global relay list", pubkey)

		// Get active relays from the database
		var relayStatuses []RelayStatus
		result := DB.Find(&relayStatuses)
		if result.Error != nil {
			TheLog.Printf("Error querying relay_status: %v", result.Error)
		}

		if len(relayStatuses) == 0 {
			TheLog.Printf("No relays configured in the database")
			return
		}

		// Create a wait group to wait for all relay connections
		var wg sync.WaitGroup
		var foundMutex sync.Mutex
		var foundRelayList, foundMetadata, foundDMRelayList bool

		wg.Add(len(relayStatuses))
		for _, relayStatus := range relayStatuses {
			go func(relayUrl string) {
				defer wg.Done()

				// Connect to relay
				relay, err := nostr.RelayConnect(context.Background(), relayUrl)
				if err != nil {
					TheLog.Printf("Error connecting to relay %s: %v", relayUrl, err)
					return
				}
				defer relay.Close()

				// Create filters for the different event kinds we want to fetch
				filters := []nostr.Filter{
					// Kind 0: Metadata
					{
						Kinds:   []int{0},
						Authors: []string{pubkey},
						Limit:   1,
					},
					// Kind 10002: Relay List
					{
						Kinds:   []int{10002},
						Authors: []string{pubkey},
						Limit:   1,
					},
					// Kind 10050: DM Relay List
					{
						Kinds:   []int{10050},
						Authors: []string{pubkey},
						Limit:   1,
					},
				}

				// Subscribe to events
				sub, err := relay.Subscribe(context.Background(), filters)
				if err != nil {
					TheLog.Printf("Error subscribing to relay %s: %v", relayUrl, err)
					return
				}

				// Process events as they come in
				for evt := range sub.Events {
					TheLog.Printf("Received event of kind %d from relay %s", evt.Kind, relayUrl)

					if evt.Kind == 0 {
						foundMutex.Lock()
						foundMetadata = true
						foundMutex.Unlock()
						processMetadataEvent(g, evt, pubkey)
					} else if evt.Kind == 10002 {
						foundMutex.Lock()
						foundRelayList = true
						foundMutex.Unlock()
						processRelayListEvent(g, evt, pubkey)
					} else if evt.Kind == 10050 {
						foundMutex.Lock()
						foundDMRelayList = true
						foundMutex.Unlock()
						processDMRelayListEvent(g, evt, pubkey)
					}
				}
			}(relayStatus.Url)
		}

		// Don't wait for goroutines to finish, but start a separate goroutine to check results
		go func() {
			// Wait for all relay connections to finish or timeout after 6 seconds
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()

			// Either wait for all goroutines to finish or timeout after 6 seconds
			select {
			case <-done:
				TheLog.Printf("All relay connections finished")
			case <-time.After(6 * time.Second):
				TheLog.Printf("Timeout waiting for relay connections")
			}

			// Log the results but don't update the UI since showPersonData will handle that
			foundMutex.Lock()
			relayListFound := foundRelayList
			metadataFound := foundMetadata
			dmRelayListFound := foundDMRelayList
			foundMutex.Unlock()

			TheLog.Printf("Fetch results - Metadata: %v, RelayList: %v, DMRelayList: %v",
				metadataFound, relayListFound, dmRelayListFound)
		}()
	} else {
		// Use the relays from the relay_list table for this pubkey
		TheLog.Printf("Found %d relays for pubkey %s", len(relayList), pubkey)

		// Create a wait group to wait for all relay connections
		var wg sync.WaitGroup
		var foundMutex sync.Mutex
		var foundRelayList, foundMetadata, foundDMRelayList bool

		wg.Add(len(relayList))
		for _, relay := range relayList {
			// Only use relays that have read permission
			if !relay.Read {
				TheLog.Printf("Skipping relay %s (no read permission)", relay.Url)
				wg.Done()
				continue
			}

			go func(relayUrl string) {
				defer wg.Done()

				// Connect to relay
				relay, err := nostr.RelayConnect(context.Background(), relayUrl)
				if err != nil {
					TheLog.Printf("Error connecting to relay %s: %v", relayUrl, err)
					return
				}
				defer relay.Close()

				// Create filters for the different event kinds we want to fetch
				filters := []nostr.Filter{
					// Kind 0: Metadata
					{
						Kinds:   []int{0},
						Authors: []string{pubkey},
						Limit:   1,
					},
					// Kind 10002: Relay List
					{
						Kinds:   []int{10002},
						Authors: []string{pubkey},
						Limit:   1,
					},
					// Kind 10050: DM Relay List
					{
						Kinds:   []int{10050},
						Authors: []string{pubkey},
						Limit:   1,
					},
				}

				// Subscribe to events
				sub, err := relay.Subscribe(context.Background(), filters)
				if err != nil {
					TheLog.Printf("Error subscribing to relay %s: %v", relayUrl, err)
					return
				}

				// Process events as they come in
				for evt := range sub.Events {
					TheLog.Printf("Received event of kind %d from relay %s", evt.Kind, relayUrl)

					if evt.Kind == 0 {
						foundMutex.Lock()
						foundMetadata = true
						foundMutex.Unlock()
						processMetadataEvent(g, evt, pubkey)
					} else if evt.Kind == 10002 {
						foundMutex.Lock()
						foundRelayList = true
						foundMutex.Unlock()
						processRelayListEvent(g, evt, pubkey)
					} else if evt.Kind == 10050 {
						foundMutex.Lock()
						foundDMRelayList = true
						foundMutex.Unlock()
						processDMRelayListEvent(g, evt, pubkey)
					}
				}
			}(relay.Url)
		}

		// Don't wait for goroutines to finish, but start a separate goroutine to check results
		go func() {
			// Wait for all relay connections to finish or timeout after 6 seconds
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()

			// Either wait for all goroutines to finish or timeout after 6 seconds
			select {
			case <-done:
				TheLog.Printf("All relay connections finished")
			case <-time.After(6 * time.Second):
				TheLog.Printf("Timeout waiting for relay connections")
			}

			// Log the results but don't update the UI since showPersonData will handle that
			foundMutex.Lock()
			relayListFound := foundRelayList
			metadataFound := foundMetadata
			dmRelayListFound := foundDMRelayList
			foundMutex.Unlock()

			TheLog.Printf("Fetch results - Metadata: %v, RelayList: %v, DMRelayList: %v",
				metadataFound, relayListFound, dmRelayListFound)
		}()
	}
}

func processMetadataEvent(g *gocui.Gui, evt *nostr.Event, pubkey string) {
	if evt == nil || evt.Kind != 0 {
		return
	}

	TheLog.Printf("Processing metadata event: %s", evt.Content)

	// Parse the metadata from the event
	var metadataContent struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		About       string `json:"about"`
		Picture     string `json:"picture"`
		Website     string `json:"website"`
		Nip05       string `json:"nip05"`
		Lud16       string `json:"lud16"`
	}

	if err := json.Unmarshal([]byte(evt.Content), &metadataContent); err != nil {
		TheLog.Printf("Error parsing metadata: %v", err)

		// Save raw content if we can't parse it
		var existingMetadata Metadata
		result := DB.Where("pubkey_hex = ?", pubkey).First(&existingMetadata)

		if result.Error != nil {
			// Create new metadata with raw content
			npub, _ := nip19.EncodePublicKey(pubkey)

			metadata := Metadata{
				PubkeyHex:         pubkey,
				PubkeyNpub:        npub,
				RawJsonContent:    evt.Content,
				MetadataUpdatedAt: evt.CreatedAt.Time(),
				ContactsUpdatedAt: time.Unix(0, 0),
			}

			if err := DB.Create(&metadata).Error; err != nil {
				TheLog.Printf("Error saving metadata to database: %v", err)
			}
		}

		return
	}

	// Check if we already have this metadata
	var existingMetadata Metadata
	result := DB.Where("pubkey_hex = ?", pubkey).First(&existingMetadata)

	if result.Error != nil {
		// Create new metadata
		npub, _ := nip19.EncodePublicKey(pubkey)

		metadata := Metadata{
			PubkeyHex:         pubkey,
			PubkeyNpub:        npub,
			Name:              metadataContent.Name,
			DisplayName:       metadataContent.DisplayName,
			About:             metadataContent.About,
			Picture:           metadataContent.Picture,
			Website:           metadataContent.Website,
			Nip05:             metadataContent.Nip05,
			Lud16:             metadataContent.Lud16,
			MetadataUpdatedAt: evt.CreatedAt.Time(),
			ContactsUpdatedAt: time.Unix(0, 0),
		}

		if err := DB.Create(&metadata).Error; err != nil {
			TheLog.Printf("Error saving metadata to database: %v", err)
		}
	} else {
		// Only update if the new event is newer
		if existingMetadata.MetadataUpdatedAt.Before(evt.CreatedAt.Time()) {
			updates := map[string]interface{}{
				"name":                metadataContent.Name,
				"display_name":        metadataContent.DisplayName,
				"about":               metadataContent.About,
				"picture":             metadataContent.Picture,
				"website":             metadataContent.Website,
				"nip05":               metadataContent.Nip05,
				"lud16":               metadataContent.Lud16,
				"metadata_updated_at": evt.CreatedAt.Time(),
			}

			if err := DB.Model(&Metadata{}).Where("pubkey_hex = ?", pubkey).Updates(updates).Error; err != nil {
				TheLog.Printf("Error updating metadata in database: %v", err)
			}
		}
	}

	// Update the view with the new metadata
	g.Update(func(g *gocui.Gui) error {
		fetchResultsView, err := g.View("fetchresults")
		if err != nil {
			return err
		}

		// Query the database for the updated metadata
		var metadata Metadata
		if err := DB.Where("pubkey_hex = ?", pubkey).First(&metadata).Error; err != nil {
			return nil
		}

		// Clear the view and redisplay the updated metadata
		content := fetchResultsView.Buffer()
		if strings.Contains(content, "PROFILE METADATA") {
			// Get the view content up to the "PROFILE METADATA" line
			lines := strings.Split(content, "\n")
			fetchResultsView.Clear()

			var newContent []string
			var foundMetadataSection bool
			var skipLines bool

			for _, line := range lines {
				if strings.Contains(line, "===== PROFILE METADATA =====") {
					foundMetadataSection = true
					newContent = append(newContent, line)
					continue
				}

				if foundMetadataSection && !skipLines {
					if strings.HasPrefix(line, "Name:") ||
						strings.HasPrefix(line, "Display Name:") ||
						strings.HasPrefix(line, "About:") ||
						strings.HasPrefix(line, "NIP-05:") ||
						strings.HasPrefix(line, "Website:") ||
						strings.HasPrefix(line, "Lightning Address:") ||
						strings.HasPrefix(line, "Total Follows:") ||
						strings.HasPrefix(line, "Last Updated:") {
						continue
					}

					if strings.Contains(line, "=====") {
						skipLines = true
					}
				}

				if !foundMetadataSection || skipLines {
					newContent = append(newContent, line)
				}
			}

			for _, line := range newContent {
				fmt.Fprintln(fetchResultsView, line)
			}
		} else {
			// If no metadata section exists, add it
			fmt.Fprintf(fetchResultsView, "\n===== PROFILE METADATA =====\n")
		}

		// Display the updated metadata
		fmt.Fprintf(fetchResultsView, "Name: %s\n", metadata.Name)
		fmt.Fprintf(fetchResultsView, "Display Name: %s\n", metadata.DisplayName)
		fmt.Fprintf(fetchResultsView, "About: %s\n", metadata.About)
		fmt.Fprintf(fetchResultsView, "NIP-05: %s\n", metadata.Nip05)
		fmt.Fprintf(fetchResultsView, "Website: %s\n", metadata.Website)
		fmt.Fprintf(fetchResultsView, "Lightning Address: %s\n", metadata.Lud16)
		fmt.Fprintf(fetchResultsView, "Total Follows: %d\n", metadata.TotalFollows)
		fmt.Fprintf(fetchResultsView, "Last Updated: %s\n", metadata.MetadataUpdatedAt.Format("2006-01-02 15:04:05"))

		return nil
	})
}

func processDMRelayListEvent(g *gocui.Gui, evt *nostr.Event, pubkey string) {
	if evt == nil || evt.Kind != 10050 {
		return
	}

	TheLog.Printf("Processing DM relay list event with %d tags", len(evt.Tags))

	// Clear existing DM relay list for this pubkey
	DB.Where("pubkey_hex = ?", pubkey).Delete(&DMRelay{})

	for _, tag := range evt.Tags {
		if len(tag) >= 2 && tag[0] == "relay" {
			relayUrl := tag[1]

			// Create a new DM relay list entry
			dmRelay := DMRelay{
				PubkeyHex: pubkey,
				Url:       relayUrl,
			}

			// Save to database
			if err := DB.Create(&dmRelay).Error; err != nil {
				TheLog.Printf("Error saving DM relay to database: %v", err)
			}
		}
	}

	// Update the view with the DM relay list
	g.Update(func(g *gocui.Gui) error {
		fetchResultsView, err := g.View("fetchresults")
		if err != nil {
			return err
		}

		// Query the database for the updated DM relay list
		var dmRelays []DMRelay
		DB.Where("pubkey_hex = ?", pubkey).Find(&dmRelays)

		// Clear any existing DM relay list section
		content := fetchResultsView.Buffer()
		if strings.Contains(content, "DM RELAY LIST") {
			lines := strings.Split(content, "\n")
			fetchResultsView.Clear()

			var newContent []string
			var foundDMSection bool
			var skipLines bool

			for _, line := range lines {
				if strings.Contains(line, "===== DM RELAY LIST =====") {
					foundDMSection = true
					continue
				}

				if foundDMSection && !skipLines {
					if strings.HasPrefix(line, "  ") { // Relay entries start with two spaces
						continue
					}

					if strings.Contains(line, "=====") || strings.Contains(line, "Fetching") || strings.Contains(line, "Press ESC") {
						skipLines = false
						foundDMSection = false
					}
				}

				newContent = append(newContent, line)
			}

			for _, line := range newContent {
				fmt.Fprintln(fetchResultsView, line)
			}
		}

		if len(dmRelays) == 0 {
			fmt.Fprintf(fetchResultsView, "\n===== DM RELAY LIST =====\n")
			fmt.Fprintf(fetchResultsView, "  None\n")
			return nil
		}

		fmt.Fprintf(fetchResultsView, "\n===== DM RELAY LIST =====\n")
		for _, relay := range dmRelays {
			fmt.Fprintf(fetchResultsView, "  %s\n", relay.Url)
		}

		return nil
	})
}

func processRelayListEvent(g *gocui.Gui, evt *nostr.Event, pubkey string) {
	if evt == nil {
		TheLog.Printf("Error: Received nil event in processRelayListEvent")
		return
	}

	TheLog.Printf("Processing relay list event with %d tags", len(evt.Tags))

	// Clear existing relay list for this pubkey
	DB.Where("pubkey_hex = ?", pubkey).Delete(&RelayList{})

	// According to NIP-65, relay lists are stored in "r" tags
	// Format: ["r", "wss://relay.example.com", "read" or "write" (optional)]
	foundRelays := false

	for _, tag := range evt.Tags {
		if len(tag) >= 2 && tag[0] == "r" {
			relayUrl := tag[1]
			read := true
			write := true

			// Check if there's a read/write marker
			if len(tag) >= 3 {
				if tag[2] == "read" {
					write = false
				} else if tag[2] == "write" {
					read = false
				}
			}

			// Create a new relay list entry
			relay := RelayList{
				PubkeyHex: pubkey,
				Url:       relayUrl,
				Read:      read,
				Write:     write,
			}

			// Save to database
			if err := DB.Create(&relay).Error; err != nil {
				TheLog.Printf("Error saving relay to database: %v", err)
			} else {
				foundRelays = true
				TheLog.Printf("Saved relay %s [Read: %v, Write: %v]", relayUrl, read, write)
			}
		}
	}

	// If no relays were found in the tags, show an error
	if !foundRelays {
		g.Update(func(g *gocui.Gui) error {
			fetchResultsView, err := g.View("fetchresults")
			if err != nil {
				return err
			}

			fmt.Fprintf(fetchResultsView, "\nNo valid relay tags found in the event.\n")
			return nil
		})
		return
	}

	// Update the view with the new relay list
	g.Update(func(g *gocui.Gui) error {
		fetchResultsView, err := g.View("fetchresults")
		if err != nil {
			return err
		}

		// Clear any existing relay list section
		content := fetchResultsView.Buffer()
		if strings.Contains(content, "RELAY LIST") {
			lines := strings.Split(content, "\n")
			fetchResultsView.Clear()

			var newContent []string
			var foundRelaySection bool
			var skipLines bool

			for _, line := range lines {
				if strings.Contains(line, "===== RELAY LIST =====") {
					foundRelaySection = true
					continue
				}

				if foundRelaySection && !skipLines {
					if strings.HasPrefix(line, "  ") { // Relay entries start with two spaces
						continue
					}

					if strings.Contains(line, "=====") || strings.Contains(line, "Fetching") || strings.Contains(line, "Press ESC") {
						skipLines = false
						foundRelaySection = false
					}
				}

				newContent = append(newContent, line)
			}

			for _, line := range newContent {
				fmt.Fprintln(fetchResultsView, line)
			}
		}

		// Query the database for the updated relay list
		var relays []RelayList
		DB.Where("pubkey_hex = ?", pubkey).Find(&relays)

		if len(relays) == 0 {
			fmt.Fprintf(fetchResultsView, "\n===== RELAY LIST =====\n")
			fmt.Fprintf(fetchResultsView, "  None\n")
			return nil
		}

		fmt.Fprintf(fetchResultsView, "\n===== RELAY LIST =====\n")
		for _, relay := range relays {
			readStr := "✓"
			writeStr := "✓"
			if !relay.Read {
				readStr = "✗"
			}
			if !relay.Write {
				writeStr = "✗"
			}
			fmt.Fprintf(fetchResultsView, "  %s [Read: %s, Write: %s]\n", relay.Url, readStr, writeStr)
		}

		return nil
	})
}
