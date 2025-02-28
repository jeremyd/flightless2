package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/awesome-gocui/gocui"
	"github.com/jeremyd/crusher17"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

var curViewNum int = 0
var selectableViews = []string{"v2", "v3", "v4"}
var v2Meta []Metadata
var searchTerm = ""
var followSearch = false
var CurrOffset = 0
var followPages []Metadata
var enterTwice = 0

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
	p, err := os.FindProcess(os.Getpid())

	if err != nil {
		return err
	}

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

func config(g *gocui.Gui, v *gocui.View) error {
	maxX, maxY := g.Size()
	accounts := []Account{}
	aerr := DB.Find(&accounts).Error
	if aerr != nil {
		TheLog.Printf("error getting accounts: %s", aerr)
	}
	if v, err := g.SetView("config", maxX/2-50, maxY/2-len(accounts), maxX/2+50, maxY/2+1, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}

		theKey := ""
		for _, acct := range accounts {
			theKey = Decrypt(string(Password), acct.Privatekey)
			if len(theKey) != 64 {
				fmt.Fprintf(v, "invalid key.. delete please: %s", theKey)
			} else {
				activeNotice := ""
				if acct.Active {
					activeNotice = "*"
				}
				fmt.Fprintf(v, "%s[%s ... ] for %s\n", activeNotice, theKey[0:5], acct.PubkeyNpub)
				// full priv key printing
				//fmt.Fprintf(v, "[%s] for %s\n", theKey, acct.Pubkey)
			}
		}

		v.Title = "Config Private Keys - [Enter]Use key - [ESC]Cancel - [n]ew key - [d]elete key - [g]enerate key - [p]rivate key reveal"
		v.Highlight = true
		v.SelBgColor = gocui.ColorGreen
		v.SelFgColor = gocui.ColorBlack
		v.Editable = false
		v.KeybindOnEdit = true
		if _, err := g.SetCurrentView("config"); err != nil {
			TheLog.Println("error setting current view to config")
			return nil
		}
	}
	return nil
}

func search(g *gocui.Gui, v *gocui.View) error {
	maxX, maxY := g.Size()
	if v, err := g.SetView("msg", maxX/2-30, maxY/2, maxX/2+30, maxY/2+2, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}
		v.Title = "Search"
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
	// zero out the highlighted list
	CurrOffset = 0
	msg, eM := g.View("msg")
	if eM != nil {
		return nil
	}
	searchTerm = "%" + strings.TrimSpace(msg.Buffer()) + "%"
	if searchTerm == "%%" {
		searchTerm = ""
	}
	if err := g.DeleteView("msg"); err != nil {
		return err
	}
	if _, err := g.SetCurrentView("v2"); err != nil {
		return err
	}
	refreshV2(g, v)
	refreshV3(g, 0)
	refreshV4(g, 0)
	return nil
}

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

	// Query the database for the metadata
	var metadata Metadata
	result := DB.Where("pubkey_hex = ?", pubkey).First(&metadata)
	if result.Error != nil {
		fmt.Fprintf(fetchResultsView, "No data found for pubkey: %s\n", pubkey)
		fmt.Fprintf(fetchResultsView, "Press ESC to close this view\n")
		return nil
	}

	// Display the metadata
	fmt.Fprintf(fetchResultsView, "Name: %s\n", metadata.Name)
	fmt.Fprintf(fetchResultsView, "Display Name: %s\n", metadata.DisplayName)
	fmt.Fprintf(fetchResultsView, "About: %s\n", metadata.About)
	fmt.Fprintf(fetchResultsView, "NIP-05: %s\n", metadata.Nip05)
	fmt.Fprintf(fetchResultsView, "Website: %s\n", metadata.Website)
	fmt.Fprintf(fetchResultsView, "Lightning Address: %s\n", metadata.Lud16)
	fmt.Fprintf(fetchResultsView, "Total Follows: %d\n", metadata.TotalFollows)
	fmt.Fprintf(fetchResultsView, "Last Updated: %s\n", metadata.MetadataUpdatedAt.Format("2006-01-02 15:04:05"))

	// Fetch and display relay list
	go fetchRelayList(g, pubkey)

	fmt.Fprintf(fetchResultsView, "\nFetching relay list...\n")
	fmt.Fprintf(fetchResultsView, "\nPress ESC to close this view\n")

	// Set the current view to the fetch results
	if _, err := g.SetCurrentView("fetchresults"); err != nil {
		return err
	}

	return nil
}

func fetchRelayList(g *gocui.Gui, pubkey string) {
	ctx := context.Background()

	// First check if we already have relay list data in the database
	var relays []RelayList
	DB.Where("pubkey_hex = ?", pubkey).Find(&relays)

	// If we have relay data, update the view
	if len(relays) > 0 {
		g.Update(func(g *gocui.Gui) error {
			fetchResultsView, err := g.View("fetchresults")
			if err != nil {
				return err
			}

			fmt.Fprintf(fetchResultsView, "\nRelay List:\n")
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
		return
	}

	// Get active relays from the database
	var relayStatuses []RelayStatus
	DB.Find(&relayStatuses)

	if len(relayStatuses) == 0 {
		g.Update(func(g *gocui.Gui) error {
			fetchResultsView, err := g.View("fetchresults")
			if err != nil {
				return err
			}

			fmt.Fprintf(fetchResultsView, "\nNo relays configured to fetch data from.\n")
			return nil
		})
		return
	}

	// Create filters for kind 10002 events
	filters := []nostr.Filter{
		{
			Kinds:   []int{10002},
			Limit:   5,
			Authors: []string{pubkey},
		},
	}

	// Track if we found any relay list events
	foundRelayList := false
	var foundRelayListMutex sync.Mutex

	// Use a wait group to track all goroutines
	var wg sync.WaitGroup

	// Try to fetch from each relay
	for _, relayStatus := range relayStatuses {
		wg.Add(1)

		// Use a separate goroutine for each relay
		go func(relayUrl string) {
			defer wg.Done()

			// Connect to the relay
			relay, err := nostr.RelayConnect(ctx, relayUrl)
			if err != nil {
				TheLog.Printf("Failed to connect to relay %s: %v", relayUrl, err)
				return
			}
			defer relay.Close()

			// Create a subscription
			sub, err := relay.Subscribe(ctx, filters)
			if err != nil {
				TheLog.Printf("Failed to subscribe to relay %s: %v", relayUrl, err)
				return
			}
			defer sub.Unsub()

			// Handle End of Stored Events in a separate goroutine
			eoseReceived := make(chan struct{})
			go func() {
				<-sub.EndOfStoredEvents
				TheLog.Printf("Got EOSE from %s", relayUrl)
				close(eoseReceived)
			}()

			// Set a timeout for the subscription
			timeout := time.After(5 * time.Second)

			// Process events from this subscription
			for {
				select {
				case <-timeout:
					TheLog.Printf("Timeout waiting for events from %s", relayUrl)
					return
				case <-eoseReceived:
					TheLog.Printf("Finished processing stored events from %s", relayUrl)
					return
				case evt, ok := <-sub.Events:
					if !ok {
						// Channel closed
						TheLog.Printf("Event channel closed for %s", relayUrl)
						return
					}

					if evt != nil && evt.Kind == 10002 {
						TheLog.Printf("Found relay list event from %s: %s", relayUrl, evt.Content)
						foundRelayListMutex.Lock()
						foundRelayList = true
						foundRelayListMutex.Unlock()

						// Process the relay list event
						processRelayListEvent(g, evt, pubkey)
					}
				}
			}
		}(relayStatus.Url)
	}

	// Don't wait for goroutines to finish, but start a separate goroutine to check results
	go func() {
		// Wait for all relay connections to finish or timeout
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

		// Check if we found any relay lists
		foundRelayListMutex.Lock()
		relayListFound := foundRelayList
		foundRelayListMutex.Unlock()

		if !relayListFound {
			g.Update(func(g *gocui.Gui) error {
				fetchResultsView, err := g.View("fetchresults")
				if err != nil {
					return err
				}

				fmt.Fprintf(fetchResultsView, "\nNo relay list found for this pubkey.\n")
				return nil
			})
		}
	}()

	// Fetch additional data from outbox relays
	go fetchFromOutboxRelays(g, pubkey)
}

func fetchFromOutboxRelays(g *gocui.Gui, pubkey string) {
	ctx := context.Background()
	
	// Get the relay list for this pubkey
	var relays []RelayList
	DB.Where("pubkey_hex = ?", pubkey).Find(&relays)
	
	if len(relays) == 0 {
		TheLog.Printf("No relay list found for pubkey %s, cannot fetch additional data", pubkey)
		return
	}
	
	// Create filters for kind 0 (profile) and kind 10050 (DM relays) events
	filters := []nostr.Filter{
		{
			Kinds:   []int{0},
			Limit:   1,
			Authors: []string{pubkey},
		},
		{
			Kinds:   []int{10050},
			Limit:   1,
			Authors: []string{pubkey},
		},
	}
	
	// Use a wait group to track all goroutines
	var wg sync.WaitGroup
	
	// Track if we found any events
	foundProfile := false
	foundDMRelays := false
	var foundMutex sync.Mutex
	
	// Connect to each relay in the relay list (prioritize write relays)
	for _, relay := range relays {
		// Only use relays marked for write (outbox relays)
		if relay.Write {
			wg.Add(1)
			
			go func(relayUrl string) {
				defer wg.Done()
				
				// Connect to the relay
				relay, err := nostr.RelayConnect(ctx, relayUrl)
				if err != nil {
					TheLog.Printf("Failed to connect to outbox relay %s: %v", relayUrl, err)
					return
				}
				defer relay.Close()
				
				// Create a subscription
				sub, err := relay.Subscribe(ctx, filters)
				if err != nil {
					TheLog.Printf("Failed to subscribe to outbox relay %s: %v", relayUrl, err)
					return
				}
				defer sub.Unsub()
				
				// Handle End of Stored Events in a separate goroutine
				eoseReceived := make(chan struct{})
				go func() {
					<-sub.EndOfStoredEvents
					TheLog.Printf("Got EOSE from outbox relay %s", relayUrl)
					close(eoseReceived)
				}()
				
				// Set a timeout for the subscription
				timeout := time.After(5 * time.Second)
				
				// Process events from this subscription
				for {
					select {
					case <-timeout:
						TheLog.Printf("Timeout waiting for events from outbox relay %s", relayUrl)
						return
					case <-eoseReceived:
						TheLog.Printf("Finished processing stored events from outbox relay %s", relayUrl)
						return
					case evt, ok := <-sub.Events:
						if !ok {
							// Channel closed
							TheLog.Printf("Event channel closed for outbox relay %s", relayUrl)
							return
						}
						
						if evt != nil {
							if evt.Kind == 0 {
								// Process profile metadata
								TheLog.Printf("Found profile metadata from outbox relay %s", relayUrl)
								foundMutex.Lock()
								foundProfile = true
								foundMutex.Unlock()
								
								// Process the metadata event
								processMetadataEvent(g, evt)
							} else if evt.Kind == 10050 {
								// Process DM relay list
								TheLog.Printf("Found DM relay list from outbox relay %s", relayUrl)
								foundMutex.Lock()
								foundDMRelays = true
								foundMutex.Unlock()
								
								// Process the DM relay list event
								processDMRelayListEvent(g, evt, pubkey)
							}
						}
					}
				}
			}(relay.Url)
		}
	}
	
	// Don't wait for goroutines to finish, but start a separate goroutine to check results
	go func() {
		// Wait for all relay connections to finish or timeout
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		
		// Either wait for all goroutines to finish or timeout after 6 seconds
		select {
		case <-done:
			TheLog.Printf("All outbox relay connections finished")
		case <-time.After(6 * time.Second):
			TheLog.Printf("Timeout waiting for outbox relay connections")
		}
		
		// Update the UI with the results
		g.Update(func(g *gocui.Gui) error {
			fetchResultsView, err := g.View("fetchresults")
			if err != nil {
				return err
			}
			
			foundMutex.Lock()
			profileFound := foundProfile
			dmRelaysFound := foundDMRelays
			foundMutex.Unlock()
			
			if !profileFound && !dmRelaysFound {
				fmt.Fprintf(fetchResultsView, "\nNo additional data found from outbox relays.\n")
			} else {
				if profileFound {
					fmt.Fprintf(fetchResultsView, "\nUpdated profile metadata from outbox relays.\n")
				}
				
				if dmRelaysFound {
					fmt.Fprintf(fetchResultsView, "\nUpdated DM relay list from outbox relays.\n")
				}
			}
			
			return nil
		})
	}()
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
			fmt.Fprintf(fetchResultsView, "Raw event: %+v\n", evt)
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

		// First clear any "Fetching relay list..." message
		content := fetchResultsView.Buffer()
		if strings.Contains(content, "Fetching relay list...") {
			// Get the view content up to the "Fetching relay list..." line
			lines := strings.Split(content, "\n")
			fetchResultsView.Clear()

			for _, line := range lines {
				if strings.Contains(line, "Fetching relay list...") {
					break
				}
				fmt.Fprintln(fetchResultsView, line)
			}
		}

		// Query the database for the updated relay list
		var relays []RelayList
		DB.Where("pubkey_hex = ?", pubkey).Find(&relays)

		if len(relays) == 0 {
			fmt.Fprintf(fetchResultsView, "\nNo relays found in the relay list.\n")
			return nil
		}

		fmt.Fprintf(fetchResultsView, "\nRelay List:\n")
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

		fmt.Fprintf(fetchResultsView, "\nPress ESC to close this view\n")
		return nil
	})
}

func processMetadataEvent(g *gocui.Gui, evt *nostr.Event) {
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
		return
	}
	
	// Check if we already have this metadata
	var existingMetadata Metadata
	result := DB.Where("pubkey_hex = ?", evt.PubKey).First(&existingMetadata)
	
	if result.Error != nil {
		// Create new metadata
		npub, _ := nip19.EncodePublicKey(evt.PubKey)
		
		metadata := Metadata{
			PubkeyHex:         evt.PubKey,
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
			
			if err := DB.Model(&Metadata{}).Where("pubkey_hex = ?", evt.PubKey).Updates(updates).Error; err != nil {
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
		if err := DB.Where("pubkey_hex = ?", evt.PubKey).First(&metadata).Error; err != nil {
			return nil
		}
		
		// Clear the view and redisplay the updated metadata
		content := fetchResultsView.Buffer()
		if strings.Contains(content, "Name:") {
			// Get the view content up to the "Name:" line
			lines := strings.Split(content, "\n")
			fetchResultsView.Clear()
			
			var newContent []string
			for i, line := range lines {
				if strings.HasPrefix(line, "Name:") {
					// Add all lines before "Name:"
					newContent = append(newContent, lines[:i]...)
					break
				}
			}
			
			for _, line := range newContent {
				fmt.Fprintln(fetchResultsView, line)
			}
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
	
	// According to Nostr standards, DM relay lists are stored in "relay" tags
	foundRelays := false
	
	// Update the view with the DM relay list
	g.Update(func(g *gocui.Gui) error {
		fetchResultsView, err := g.View("fetchresults")
		if err != nil {
			return err
		}
		
		fmt.Fprintf(fetchResultsView, "\nDM Relay List:\n")
		
		for _, tag := range evt.Tags {
			if len(tag) >= 2 && tag[0] == "relay" {
				relayUrl := tag[1]
				fmt.Fprintf(fetchResultsView, "  %s\n", relayUrl)
				foundRelays = true
			}
		}
		
		if !foundRelays {
			fmt.Fprintf(fetchResultsView, "  No DM relays found\n")
		}
		
		return nil
	})
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

func doFetch(g *gocui.Gui, v *gocui.View) error {
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

	var pubkey string

	// Check if we're coming from the fetch dialog or directly from v2
	fetchView, fetchErr := g.View("fetch")
	if fetchErr == nil {
		// Coming from fetch dialog - get input from the dialog
		pubkeyInput := strings.TrimSpace(fetchView.Buffer())

		// Check if input is empty
		if pubkeyInput == "" {
			if err := g.DeleteView("fetch"); err != nil {
				return err
			}
			if err := g.DeleteView("fetchresults"); err != nil {
				return err
			}
			if _, err := g.SetCurrentView("v2"); err != nil {
				return err
			}
			return nil
		}

		// Process the pubkey/npub input
		pubkey = pubkeyInput
		// Check if it's an npub and convert to hex if needed
		if strings.HasPrefix(pubkeyInput, "npub") {
			_, decodedPubkey, err := nip19.Decode(pubkeyInput)
			if err != nil {
				fmt.Fprintf(fetchResultsView, "Error: Invalid npub format\n")
				return nil
			}
			pubkey = decodedPubkey.(string)
		}

		// Delete the fetch input view
		if err := g.DeleteView("fetch"); err != nil {
			return err
		}
	} else {
		// Coming directly from v2 - get pubkey from the cursor position
		v2, _ := g.View("v2")
		_, cy := v2.Cursor()

		if len(displayV2Meta) == 0 || cy >= len(displayV2Meta) {
			TheLog.Println("out of bounds of the displayV2Meta", cy)
			if err := g.DeleteView("fetchresults"); err != nil {
				return err
			}
			return nil
		}
		pubkey = displayV2Meta[cy].PubkeyHex
	}

	// Query the database for the metadata
	var metadata Metadata
	result := DB.Where("pubkey_hex = ?", pubkey).First(&metadata)
	if result.Error != nil {
		fmt.Fprintf(fetchResultsView, "No data found for pubkey: %s\n", pubkey)
		fmt.Fprintf(fetchResultsView, "Press ESC to close this view\n")
		return nil
	}

	// Display the metadata
	fmt.Fprintf(fetchResultsView, "Name: %s\n", metadata.Name)
	fmt.Fprintf(fetchResultsView, "Display Name: %s\n", metadata.DisplayName)
	fmt.Fprintf(fetchResultsView, "About: %s\n", metadata.About)
	fmt.Fprintf(fetchResultsView, "NIP-05: %s\n", metadata.Nip05)
	fmt.Fprintf(fetchResultsView, "Website: %s\n", metadata.Website)
	fmt.Fprintf(fetchResultsView, "Lightning Address: %s\n", metadata.Lud16)
	fmt.Fprintf(fetchResultsView, "Total Follows: %d\n", metadata.TotalFollows)
	fmt.Fprintf(fetchResultsView, "Last Updated: %s\n", metadata.MetadataUpdatedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(fetchResultsView, "\nPress ESC to close this view\n")

	// Set the current view to the fetch results
	if _, err := g.SetCurrentView("fetchresults"); err != nil {
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
	NoticeColor := "\033[1;36m%s\033[0m"
	s := fmt.Sprintf("(%s)earch", fmt.Sprintf(NoticeColor, "S"))
	q := fmt.Sprintf("(%s)uit", fmt.Sprintf(NoticeColor, "Q"))
	r := fmt.Sprintf("(%s)efresh", fmt.Sprintf(NoticeColor, "R"))
	t := fmt.Sprintf("(%s)next window", fmt.Sprintf(NoticeColor, "TAB"))
	a := fmt.Sprintf("(%s)dd relay", fmt.Sprintf(NoticeColor, "A"))

	fmt.Fprintf(v5, "%-30s%-30s%-30s%-30s%-30s\n", s, q, r, t, a)
	z := fmt.Sprintf("(%s)Select ALL", fmt.Sprintf(NoticeColor, "Z"))
	d := fmt.Sprintf("(%s)elete relay", fmt.Sprintf(NoticeColor, "D"))
	c := fmt.Sprintf("(%s)onfigure keys", fmt.Sprintf(NoticeColor, "C"))
	fe := fmt.Sprintf("(%s)etch person", fmt.Sprintf(NoticeColor, "F"))
	p := fmt.Sprintf("(%s)ubkey lookup", fmt.Sprintf(NoticeColor, "P"))
	fmt.Fprintf(v5, "%-30s%-30s%-30s%-30s%-30s\n\n", z, d, c, fe, p)

	g.DeleteKeybinding("v5", gocui.KeyEnter, gocui.ModNone)
	g.SetCurrentView("v2")
	return nil
}

func confirmPostInput(g *gocui.Gui, v *gocui.View) error {
	v.Title = "Confirm Post? ENTER to post / ESC to cancel"
	v.BgColor = uiColorBg
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

		// Check if we're at the end of the list
		totalItems := len(v2Meta)
		if followSearch {
			totalItems = len(followPages)
		}

		if (cy + CurrOffset + 1) >= totalItems {
			return nil
		}

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
			refreshV2Conversations(g, v)
			v.SetHighlight(0, true)
			refreshV3(g, 0)
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
			refreshV2Conversations(g, v)

			// Move cursor to bottom of view unless we're at the start
			newY := vSizeY - 2
			if err := v.SetCursor(cx, newY); err != nil {
				ox, oy := v.Origin()
				if err := v.SetOrigin(ox, oy-1); err != nil {
					return err
				}
			}
			v.SetHighlight(newY, true)
			refreshV3(g, newY)
			return nil
		}

		// Move cursor up one line
		if err := v.SetCursor(cx, cy-1); err != nil {
			ox, oy := v.Origin()
			if err := v.SetOrigin(ox, oy-1); err != nil {
				return err
			}
		} else {
		}
		v.SetHighlight(cy-1, true)
		refreshV3(g, cy-1)
		refreshV4(g, cy-1)
	}
	return nil
}

func configNew(
	g *gocui.Gui,
	v *gocui.View,
) error {
	maxX, maxY := g.Size()
	g.DeleteView("config")
	if v, err := g.SetView("confignew", maxX/2-50, maxY/2-1, maxX/2+50, maxY/2+1, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}

		v.Title = "New/Edit Private Key - [Enter]Save - [ESC]Cancel -"
		v.Highlight = true
		v.SelBgColor = gocui.ColorGreen
		v.SelFgColor = gocui.ColorBlack
		v.Editable = true
		v.KeybindOnEdit = true
		if _, err := g.SetCurrentView("confignew"); err != nil {
			return err
		}
	}
	return nil
}

func activateConfig(
	g *gocui.Gui,
	v *gocui.View,
) error {
	cView, _ := g.View("config")
	_, cy := cView.Cursor()
	accounts := []Account{}
	aerr := DB.Find(&accounts).Error
	if aerr != nil {
		TheLog.Printf("error getting accounts: %s", aerr)
	}

	var acct Account
	DB.Model(&acct).Where("pubkey = ?", accounts[cy].Pubkey).Update("active", true)
	DB.Model(&acct).Where("pubkey != ?", accounts[cy].Pubkey).Update("active", false)

	g.DeleteView("config")
	g.SetCurrentView("v2")
	v2, _ := g.View("v2")
	v2.SetCursor(0, 0)
	refreshV2Conversations(g, v)
	refreshV3(g, 0)

	ctx := context.Background()
	doDMRelays(DB, ctx)
	return nil
}

func generateConfig(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		sk := nostr.GeneratePrivateKey()
		encKey := Encrypt(string(Password), sk)
		pk, ep := nostr.GetPublicKey(sk)
		npub, ep2 := nip19.EncodePublicKey(pk)
		if ep != nil || ep2 != nil {
			TheLog.Printf("error getting public key: %s", ep)
		}
		account := Account{Privatekey: encKey, Pubkey: pk, PubkeyNpub: npub, Active: true}
		e2 := DB.Save(&account).Error
		if e2 != nil {
			TheLog.Printf("error saving private key: %s", e2)
		}

		g.SetCurrentView("v2")
		g.DeleteView("config")
	}
	return nil
}

func configShowPrivateKey(
	g *gocui.Gui,
	v *gocui.View,
) error {
	maxX, maxY := g.Size()
	cView, _ := g.View("config")
	_, cy := cView.Cursor()
	accounts := []Account{}
	aerr := DB.Find(&accounts).Error
	if aerr != nil {
		TheLog.Printf("error getting accounts: %s", aerr)
	}
	sk := Decrypt(string(Password), accounts[cy].Privatekey)
	g.DeleteView("config")
	if v, err := g.SetView("configshow", maxX/2-50, maxY/2-1, maxX/2+50, maxY/2+1, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}

		fmt.Fprintf(v, "%s", sk)
		v.Title = "*** Showing Private Key ***  [ESC]Dismiss"
		v.Highlight = true
		v.SelBgColor = gocui.ColorGreen
		v.SelFgColor = gocui.ColorBlack
		v.Editable = false
		v.KeybindOnEdit = true
		if _, err := g.SetCurrentView("configshow"); err != nil {
			return err
		}
	}
	return nil
}

func doConfigNew(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		line := v.Buffer()
		if line == "" {
			TheLog.Println("no private key entered")
			g.SetCurrentView("v2")
			g.DeleteView("confignew")
			return nil
		}
		//fmt.Println(line)
		//fmt.Println("saving config")
		useKey := line
		if line[0:1] == "n" {
			prefix, value, err := nip19.Decode(line)
			if err != nil {
				TheLog.Println("error decoding nip19 key")
				return nil
			} else {
				TheLog.Println("decoded nip19 " + prefix + "to hex")
				useKey = value.(string)
			}
		}
		if !isHex(useKey) {
			TheLog.Println("private key was invalid")
			g.SetCurrentView("v2")
			g.DeleteView("confignew")
			return nil
		}

		encKey := Encrypt(string(Password), useKey)
		pk, ep := nostr.GetPublicKey(useKey)
		npub, ep2 := nip19.EncodePublicKey(pk)
		if ep != nil || ep2 != nil {
			TheLog.Printf("error getting public key: %s", ep)
		}
		account := Account{Privatekey: encKey, Pubkey: pk, PubkeyNpub: npub, Active: true}
		e2 := DB.Save(&account).Error
		if e2 != nil {
			TheLog.Printf("error saving private key: %s", e2)
		}

		g.SetCurrentView("v2")
		g.DeleteView("confignew")
		//refresh(g, v)
	}
	return nil
}

func cancelConfig(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("config")
	g.SetCurrentView("v2")
	return nil
}

func doConfigDel(g *gocui.Gui, v *gocui.View) error {
	cView, _ := g.View("config")
	_, cy := cView.Cursor()
	accounts := []Account{}
	aerr := DB.Find(&accounts).Error
	if aerr != nil {
		TheLog.Printf("error getting accounts: %s", aerr)
	}
	if v != nil {
		line := v.Buffer()
		if line == "" {
			g.SetCurrentView("v2")
			g.DeleteView("config")
			return nil
		}
		e2 := DB.Delete(&accounts[cy]).Error
		if e2 != nil {
			TheLog.Printf("error deleting private key: %s", e2)
		}

		// activate a different account if there are any
		DB.Find(&accounts)
		anyActive := false
		anyAccounts := (len(accounts) > 0)
		for _, a := range accounts {
			if a.Active {
				anyActive = true
			}
		}
		if !anyActive && anyAccounts {
			accounts[0].Active = true
			DB.Save(&accounts[0])
		}

		g.DeleteView("config")
	}
	return nil
}

func cursorDownConfig(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		_, cy := v.Cursor()
		_, sy := v.Size()
		// end of view
		if cy >= sy-1 {
			return nil
		}
		// move cursor and origin
		if err := v.SetCursor(0, cy+1); err != nil {
			ox, oy := v.Origin()
			if err := v.SetOrigin(ox, oy+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func cursorUpConfig(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		_, cy := v.Cursor()
		// top of view
		if cy == 0 {
			return nil
		}
		// move cursor and origin
		if err := v.SetCursor(0, cy-1); err != nil {
			ox, oy := v.Origin()
			if err := v.SetOrigin(ox, oy-1); err != nil {
				return err
			}
		}
	}
	return nil
}

func cancelConfigNew(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("confignew")
	g.SetCurrentView("v2")
	return nil
}

func cancelConfigShow(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("configshow")
	g.SetCurrentView("v2")
	return nil
}

// RELAYS
func addRelay(g *gocui.Gui, v *gocui.View) error {
	maxX, maxY := g.Size()
	//prevViewName := v.Name()
	if v, err := g.SetView("addrelay", maxX/2-30, maxY/2, maxX/2+30, maxY/2+2, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}
		if _, err := g.SetCurrentView("addrelay"); err != nil {
			return err
		}
		v.Title = "Add Relay? [enter] to save / [ESC] to cancel"
		v.Editable = true
		v.KeybindOnEdit = true
		/*
			v2, _ := g.View("v2")
			_, cy := v2.Cursor()
			if prevViewName == "v2" && len(v2Meta) > 0 {
				curM := v2Meta[cy]
				var curServer RecommendServer
				ViewDB.Model(&curM).Association("Servers").Find(&curServer, "recommended_by = ?", curM.PubkeyHex)
				if curServer.Url == "" {
				} else {
					fmt.Fprintf(v, "%s", curServer.Url)
				}
			}
		*/
	}
	return nil
}

func doAddRelay(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		line := v.Buffer()
		if line == "" {
			g.SetCurrentView("v2")
			g.DeleteView("addrelay")
			refreshV4(g, 0)
			return nil
		}
		err := DB.Create(&RelayStatus{Url: line, Status: "waiting", LastEOSE: time.Unix(0, 0), LastDisco: time.Unix(0, 0)}).Error
		if err != nil {
			TheLog.Println("error adding relay")
		}
		g.DeleteView("addrelay")
		refreshV4(g, 0)
		g.SetCurrentView("v2")
	}
	return nil
}

func cancelAddRelay(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("addrelay")
	g.SetCurrentView("v2")
	return nil
}

func cursorDownV4(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		cx, cy := v.Cursor()
		//relays := []Relay{}
		//DB.Find(&relays)
		if cy < len(strings.Split(v.Buffer(), "\n"))-1 {
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

func cursorUpV4(g *gocui.Gui, v *gocui.View) error {
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

func delRelay(g *gocui.Gui, v *gocui.View) error {
	_, cy := v.Cursor()
	if cy < len(strings.Split(v.Buffer(), "\n"))-1 {
		lines := strings.Split(v.Buffer(), "\n")
		if len(lines) <= cy {
			return nil
		}

		line := strings.TrimSpace(lines[cy])
		if line == "" {
			return nil
		}

		// Extract relay URL from the line
		parts := strings.Split(line, " ")
		if len(parts) < 2 {
			return nil
		}
		relayUrl := parts[1]

		// Find existing relay status
		var status RelayStatus
		result := DB.Where("url = ?", relayUrl).First(&status)
		if result.Error != nil {
			TheLog.Printf("error finding relay status: %v", result.Error)
			return result.Error
		}

		// Update status to deleting
		status.Status = "deleting"
		if err := DB.Save(&status).Error; err != nil {
			TheLog.Printf("error updating relay status: %v", err)
			return err
		}

		TheLog.Printf("marked relay %s for deletion", relayUrl)
	}
	return nil
}
