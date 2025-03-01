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

func toggleConversationFollows(g *gocui.Gui, v *gocui.View) error {
	if v2MetaDisplay == 1 {
		v2MetaDisplay = 0
		refreshV2Conversations(g, v)
	} else {
		v2MetaDisplay = 1
		refreshV2(g, v)
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
			if v2MetaDisplay == 0 {
				refreshV2Conversations(g, v)
			} else {
				refreshV2(g, v)
			}
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
			if v2MetaDisplay == 0 {
				refreshV2Conversations(g, v)
			} else if v2MetaDisplay == 1 {
				refreshV2(g, v)
			}

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
