package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/awesome-gocui/gocui"
	"gorm.io/gorm"
)

var AppInfo = "flightless v2.0.0-pre"

var TheLog *log.Logger
var Password []byte
var DB *gorm.DB
var TheGui *gocui.Gui

func main() {
	DB = GetGormConnection()
	RunMigrations()

	var login Login
	loginDbErr := DB.First(&login).Error

	if loginDbErr != nil || login.PasswordHash == "" {
		fmt.Println("no login found, create a new password")
		Password = GetNewPwd()
		login.PasswordHash = HashAndSalt(Password)
		DB.Create(&login)
		fmt.Println("login created, loading...")
	} else {
		Password = GetPwd()
		success := ComparePasswords(login.PasswordHash, Password)
		if success {
			fmt.Println("login success, loading...")
		} else {
			fmt.Println("login failed")
			os.Exit(1)
		}
	}

	// relays
	var relayUrls []string
	var relayStatuses []RelayStatus
	DB.Find(&relayStatuses)
	if len(relayStatuses) == 0 {
		TheLog.Println("error finding relay urls")
		relayUrls = []string{
			//"wss://relay.snort.social",
			//"wss://relay.damus.io",
			//"wss://nostr.zebedee.cloud",
			//"wss://eden.nostr.land",
			//"wss://nostr-pub.wellorder.net",
			//"wss://nostr-dev.wellorder.net",
			//"wss://relay.nostr.info",
			"wss://profiles.nostr1.com",
			//"wss://nostr21.com",
		}
	} else {
		for _, relayStatus := range relayStatuses {
			relayUrls = append(relayUrls, relayStatus.Url)
		}
	}

	CTX := context.Background()

	for _, url := range relayUrls {
		TheLog.Printf("connecting to relay: %s\n", url)
		doRelay(DB, CTX, url)
	}

	doDMRelays(DB, CTX)

	go watchInterrupt()

	g, err := gocui.NewGui(gocui.OutputTrue, true)
	if err != nil {
		log.Panicln(err)
	}
	defer g.Close()

	TheGui = g

	g.SetManagerFunc(layout)
	if err := keybindings(g); err != nil {
		log.Panicln(err)
	}

	// relay status manager!
	go func() {
		for {
			var RelayStatuses []RelayStatus
			DB.Find(&RelayStatuses)
			for _, relayStatus := range RelayStatuses {
				if relayStatus.Status == "waiting" {
					doRelay(DB, CTX, relayStatus.Url)
				} else if relayStatus.Status == "deleting" {
					TheLog.Printf("Processing relay marked for deletion: %s", relayStatus.Url)
					foundit := false
					for i, r := range nostrRelays {
						if r.URL == relayStatus.Url {
							TheLog.Printf("Found relay to delete: %s", r.URL)
							err := DB.Delete(&relayStatus).Error
							if err != nil {
								TheLog.Printf("Error deleting relay status: %v", err)
							} else {
								TheLog.Printf("Successfully deleted relay status for: %s", relayStatus.Url)
							}
							foundit = true
							r.Close()
							TheLog.Printf("Closed relay connection: %s", r.URL)

							// Remove from nostrRelays slice
							nostrRelays = append(nostrRelays[:i], nostrRelays[i+1:]...)
							TheLog.Printf("Removed relay from nostrRelays slice")
							break
						}
					}
					// if we didn't find it, delete the record anyway
					if !foundit {
						TheLog.Printf("Relay connection not found, deleting record anyway: %s", relayStatus.Url)
						err := DB.Delete(&relayStatus).Error
						if err != nil {
							TheLog.Printf("Error deleting relay status: %v", err)
						} else {
							TheLog.Printf("Successfully deleted relay status for: %s", relayStatus.Url)
						}
					}
				}
			}
			time.Sleep(1 * time.Second)
		}
	}()

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		log.Panicln(err)
	}

}
