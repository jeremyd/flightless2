package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/awesome-gocui/gocui"
	"gorm.io/gorm"
)

var AppInfo = "flightless v2.0.0-pre"

var TheLog *log.Logger
var Password []byte
var DB *gorm.DB

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

	go watchInterrupt()

	g, err := gocui.NewGui(gocui.OutputTrue, true)
	if err != nil {
		log.Panicln(err)
	}
	defer g.Close()

	g.SetManagerFunc(layout)
	if err := keybindings(g); err != nil {
		log.Panicln(err)
	}

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		log.Panicln(err)
	}

}
