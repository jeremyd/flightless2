package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"net/http"

	"github.com/jeremyd/crusher17"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"gorm.io/gorm"
)

var nostrSubs []*nostr.Subscription
var nostrRelays []*nostr.Relay

type RelayLimitation struct {
	AuthRequired bool `json:"auth_required"`
}

type RelayInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	PubKey      string          `json:"pubkey"`
	Contact     string          `json:"contact"`
	Supported   []int           `json:"supported_nips"`
	Software    string          `json:"software"`
	Version     string          `json:"version"`
	Limitation  RelayLimitation `json:"limitation"`
}

func checkRelayRequiresAuth(url string) bool {
	httpURL := strings.Replace(strings.Replace(url, "ws://", "http://", 1), "wss://", "https://", 1)

	client := &http.Client{
		Timeout: time.Second * 5,
	}

	req, err := http.NewRequest("GET", httpURL, nil)
	if err != nil {
		TheLog.Printf("Error creating request for relay info: %v\n", err)
		return false
	}

	req.Header.Set("Accept", "application/nostr+json")

	resp, err := client.Do(req)
	if err != nil {
		TheLog.Printf("Error getting relay info: %v\n", err)
		return false
	}
	defer resp.Body.Close()

	var info RelayInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		TheLog.Printf("Error decoding relay info: %v\n", err)
		return false
	}

	TheLog.Printf("Relay info: %+v\n", info)

	return info.Limitation.AuthRequired
}

func isHex(s string) bool {
	dst := make([]byte, hex.DecodedLen(len(s)))

	if _, err := hex.Decode(dst, []byte(s)); err != nil {
		return false
		// s is not a valid
	}
	return true
}

func watchInterrupt() {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		TheLog.Println("exiting gracefully")
		for _, s := range nostrSubs {
			s.Unsub()
			s.Close()
		}
		for _, r := range nostrRelays {
			TheLog.Printf("Closing connection to relay: %s\n", r.URL)
			r.Close()
			UpdateOrCreateRelayStatus(DB, r.URL, "connection error: app exit")
		}
		// give other relays time to close
		time.Sleep(3 * time.Second)
		os.Exit(0)
	}()
}

func UpdateOrCreateRelayStatus(db *gorm.DB, url string, status string) {
	var r RelayStatus
	if status == "connection established: EOSE" {
		r = RelayStatus{Url: url, Status: status, LastEOSE: time.Now()}
	} else if strings.HasPrefix(status, "connection error") {
		r = RelayStatus{Url: url, Status: status, LastDisco: time.Now()}
	} else {
		r = RelayStatus{Url: url, Status: status}
	}
	var s RelayStatus
	err := db.Model(&s).Where("url = ?", url).First(&s).Error
	if err == nil {
		db.Model(&r).Where("url = ?", url).Updates(&r)
	} else {
		db.Create(&r)
	}
}

func doRelay(db *gorm.DB, ctx context.Context, url string) bool {
	// get active pubkey from db
	var account Account
	db.Where("active = ?", true).First(&account)
	if account.Pubkey == "" {
		TheLog.Println("no active pubkey, skipping relay")
		return false
	}
	pubkey := account.Pubkey

	// check if connection already established
	var fr RelayStatus
	db.Model(fr).Where("url = ?", url).First(&fr)
	if strings.Contains(fr.Status, "established") {
		TheLog.Printf("connection already established to relay: %s\n", url)
	}

	// Connect with auth support
	relay, err := nostr.RelayConnect(ctx, url)
	if err != nil {
		TheLog.Printf("failed initial connection to relay: %s, %s; skipping relay", url, err)
		UpdateOrCreateRelayStatus(db, url, "failed initial connection")
		return false
	}
	nostrRelays = append(nostrRelays, relay)

	// Check if relay requires auth via NIP-11
	if account.Privatekey != "" && checkRelayRequiresAuth(url) {
		// Decrypt the private key using the global Password
		decryptedKey := Decrypt(string(Password), account.Privatekey)

		// Set up auth with signing function
		err = relay.Auth(ctx, func(evt *nostr.Event) error {
			return evt.Sign(decryptedKey)
		})
		if err != nil {
			TheLog.Printf("Failed to authenticate with relay %s: %v\n", url, err)
		} else {
			TheLog.Printf("Successfully authenticated with relay %s\n", url)
		}
	}

	UpdateOrCreateRelayStatus(db, url, "connection established")

	// what do we need for this pubkey for WoT:

	// the follow list (hop1)
	// the follow list of each follow (hop2)
	// hop3?

	hop1Filters := []nostr.Filter{
		{
			Kinds:   []int{0},
			Limit:   1,
			Authors: []string{pubkey},
		},
		{
			Kinds:   []int{3},
			Limit:   1,
			Authors: []string{pubkey},
		},
		{
			Kinds:   []int{10050},
			Limit:   1,
			Authors: []string{pubkey},
		},
		{
			Kinds: []int{1059},
			Limit: 2000,
			Tags: nostr.TagMap{
				"p": []string{pubkey},
			},
		},
	}

	// create a subscription and submit to relay
	sub, _ := relay.Subscribe(ctx, hop1Filters)
	nostrSubs = append(nostrSubs, sub)

	// subscribe to follows for each follow
	person := Metadata{
		PubkeyHex: pubkey,
	}

	var thisHopFollows []Metadata
	db.Model(&person).Association("Follows").Find(&thisHopFollows)

	// add in pubkeys that we have conversations with
	var allMessages []ChatMessage
	DB.Where("to_pubkey = ?", pubkey).Find(&allMessages)

	// group the messages by from_pubkey
	conversations := make(map[string][]ChatMessage)
	for _, message := range allMessages {
		conversations[message.FromPubkey] = append(conversations[message.FromPubkey], message)
	}

	for p, _ := range conversations {
		thisHopFollows = append(thisHopFollows, Metadata{PubkeyHex: p})
	}

	// Pick up where we left off for this relay based on last EOSE timestamp
	var rs RelayStatus
	db.Where("url = ?", url).First(&rs)

	sinceDisco := rs.LastDisco
	if sinceDisco.IsZero() {
		sinceDisco = time.Now().Add(-72 * time.Hour)
		TheLog.Printf("no known last disco time for %s, defaulting to 72 hrs\n", url)
	}
	since := rs.LastEOSE
	if since.IsZero() {
		since = time.Now().Add(-73 * time.Hour)
	}
	if sinceDisco.After(since) {
		since = sinceDisco
	}

	//filterTimestamp := nostr.Timestamp(since.Unix())

	// BATCH filters into chunks of 1000 per filter.
	var hop2Filters []nostr.Filter

	counter := 0
	lastCount := 0
	if len(thisHopFollows) > 1000 {
		for i := range thisHopFollows {
			if i > 0 && i%1000 == 0 {
				begin := i - 1000
				end := counter
				authors := thisHopFollows[begin:end]
				var authorPubkeys []string
				for _, a := range authors {
					authorPubkeys = append(authorPubkeys, a.PubkeyHex)
				}

				hop2Filters = append(hop2Filters, nostr.Filter{
					Kinds:   []int{0, 10050},
					Limit:   1000,
					Authors: authorPubkeys,
					//Since:   &filterTimestamp,
				})
				TheLog.Printf("adding chunk subscription for %d:%d", begin, end)
				lastCount = counter
			}
			counter += 1
		}
		// leftover
		if lastCount != counter+1 {
			begin := lastCount + 1
			end := len(thisHopFollows) - 1
			remainingAuthors := thisHopFollows[begin:end]
			var authorPubkeys []string
			for _, a := range remainingAuthors {
				authorPubkeys = append(authorPubkeys, a.PubkeyHex)
			}

			TheLog.Printf("adding leftover chunk subscription for %d:%d", lastCount, end)

			hop2Filters = append(hop2Filters, nostr.Filter{
				Kinds:   []int{0, 10050},
				Limit:   1000,
				Authors: authorPubkeys,
				//Since:   &filterTimestamp,
			})
		}
	} else {
		var authorPubkeys []string
		for _, a := range thisHopFollows {
			authorPubkeys = append(authorPubkeys, a.PubkeyHex)
		}
		hop2Filters = append(hop2Filters, nostr.Filter{
			Kinds:   []int{0, 10050},
			Limit:   1000,
			Authors: authorPubkeys,
			//Since:   &filterTimestamp,
		})
	}

	hop2Sub, _ := relay.Subscribe(ctx, hop2Filters)
	nostrSubs = append(nostrSubs, hop2Sub)

	go func() {
		processSub(sub, relay, pubkey)
	}()

	go func() {
		processSub(hop2Sub, relay, pubkey)
	}()

	return true
}

func processSub(sub *nostr.Subscription, relay *nostr.Relay, pubkey string) {

	go func() {
		<-sub.EndOfStoredEvents
		TheLog.Printf("got EOSE from %s\n", relay.URL)
		UpdateOrCreateRelayStatus(DB, relay.URL, "connection established: EOSE")
	}()

	if sub != nil {
		for ev := range sub.Events {
			if ev.Kind == 0 {
				// Metadata
				m := Metadata{}
				err := json.Unmarshal([]byte(ev.Content), &m)
				unmarshalSuccess := false
				if err != nil {
					TheLog.Printf("%s: %v", err, ev.Content)
					m.RawJsonContent = ev.Content
				} else {
					unmarshalSuccess = true
				}
				m.PubkeyHex = ev.PubKey
				npub, errEncode := nip19.EncodePublicKey(ev.PubKey)
				if errEncode == nil {
					m.PubkeyNpub = npub
				}
				m.MetadataUpdatedAt = ev.CreatedAt.Time()
				m.ContactsUpdatedAt = time.Unix(0, 0)
				if len(m.Picture) > 65535 {
					//TheLog.Println("too big a picture for profile, skipping" + ev.PubKey)
					m.Picture = ""
					//continue
				}
				// check timestamps
				var checkMeta Metadata
				notFoundErr := DB.First(&checkMeta, "pubkey_hex = ?", m.PubkeyHex).Error
				if notFoundErr != nil {
					err := DB.Save(&m).Error
					if err != nil {
						TheLog.Printf("Error saving metadata was: %s", err)
					}
					TheLog.Printf("Created metadata for %s, %s\n", m.Name, m.Nip05)
				} else {
					if checkMeta.MetadataUpdatedAt.After(ev.CreatedAt.Time()) || checkMeta.MetadataUpdatedAt.Equal(ev.CreatedAt.Time()) {
						//TheLog.Println("skipping old metadata for " + ev.PubKey)
						continue
					} else {
						rowsUpdated := DB.Model(Metadata{}).Where("pubkey_hex = ?", m.PubkeyHex).Updates(&m).RowsAffected
						if rowsUpdated > 0 {
							TheLog.Printf("Updated metadata for %s, %s\n", m.Name, m.Nip05)
						} else {
							//
							// here we need go store the record anyway, with a pubkey, and the 'rawjson'
							TheLog.Printf("UNCOOL NESTED JSON FOR METADATA DETECTED, falling back to RAW json %v, unmarshalsuccess was: %v", m, unmarshalSuccess)
						}
					}
				}
			} else if ev.Kind == 10050 {
				var person Metadata
				notFoundError := DB.First(&person, "pubkey_hex = ?", ev.PubKey).Error
				if notFoundError != nil {
					//TheLog.Printf("Creating blank metadata for %s\n", ev.PubKey)
					person = Metadata{
						PubkeyHex: ev.PubKey,
						// set time to january 1st 1970
						MetadataUpdatedAt: time.Unix(0, 0),
						ContactsUpdatedAt: time.Unix(0, 0),
					}
					DB.Create(&person)
				}
				relayTags := []string{"relay"}
				allRelayTags := ev.Tags.GetAll(relayTags)
				for _, relayTag := range allRelayTags {
					r := DMRelay{}
					// First check if this relay URL exists for this pubkey
					var existingRelay DMRelay
					err := DB.Where("pubkey_hex = ? AND url = ?", ev.PubKey, relayTag[1]).First(&existingRelay).Error
					if err != nil {
						// URL doesn't exist, create new entry
						r = DMRelay{
							PubkeyHex: ev.PubKey,
							Url:       relayTag[1],
							CreatedAt: time.Now(),
							UpdatedAt: time.Now(),
						}
						DB.Create(&r)
					}
				}
				// Remove URLs that are no longer in the tags
				var existingUrls []string
				for _, tag := range allRelayTags {
					existingUrls = append(existingUrls, tag[1])
				}
				DB.Where("pubkey_hex = ? AND url NOT IN ?", ev.PubKey, existingUrls).Delete(&DMRelay{})
			} else if ev.Kind == 3 {

				// Contact List
				pTags := []string{"p"}
				allPTags := ev.Tags.GetAll(pTags)
				var person Metadata
				notFoundError := DB.First(&person, "pubkey_hex = ?", ev.PubKey).Error
				if notFoundError != nil {
					//TheLog.Printf("Creating blank metadata for %s\n", ev.PubKey)
					person = Metadata{
						PubkeyHex:    ev.PubKey,
						TotalFollows: len(allPTags),
						// set time to january 1st 1970
						MetadataUpdatedAt: time.Unix(0, 0),
						ContactsUpdatedAt: ev.CreatedAt.Time(),
					}
					DB.Create(&person)
				} else {
					if person.ContactsUpdatedAt.After(ev.CreatedAt.Time()) {
						// double check the timestamp for this follow list, don't update if older than most recent
						TheLog.Printf("skipping old contact list for " + ev.PubKey)
						continue
					} else {
						DB.Model(&person).Omit("updated_at").Update("total_follows", len(allPTags))
						DB.Model(&person).Omit("updated_at").Update("contacts_updated_at", ev.CreatedAt.Time())
						//TheLog.Printf("updating (%d) follows for %s: %s\n", len(allPTags), person.Name, person.PubkeyHex)
					}
				}

				// purge followers that have been 'unfollowed'
				var oldFollows []Metadata
				DB.Model(&person).Association("Follows").Find(&oldFollows)
				for _, oldFollow := range oldFollows {
					found := false
					for _, n := range allPTags {
						if len(n) >= 2 && n[1] == oldFollow.PubkeyHex {
							found = true
						}
					}
					if !found {
						DB.Exec("delete from metadata_follows where metadata_pubkey_hex = ? and follow_pubkey_hex = ?", person.PubkeyHex, oldFollow.PubkeyHex)
					}
				}

				// Add follows
				for _, followPerson := range person.Follows {
					DB.Exec("INSERT OR IGNORE INTO metadata_follows (metadata_pubkey_hex, follow_pubkey_hex) VALUES (?, ?)", person.PubkeyHex, followPerson.PubkeyHex)
				}

				for _, c := range allPTags {
					// if the pubkey fails the sanitization (is a hex value) skip it

					if len(c) < 2 || !isHex(c[1]) {
						TheLog.Printf("skipping invalid pubkey from follow list: %d, %s ", len(c), c[1])
						continue
					}
					var followPerson Metadata
					notFoundFollow := DB.First(&followPerson, "pubkey_hex = ?", c[1]).Error

					if notFoundFollow != nil {
						// follow user not found, need to create it
						var newUser Metadata
						// follow user recommend server suggestion if it exists
						if len(c) >= 3 && c[2] != "" {
							newUser = Metadata{
								PubkeyHex:         c[1],
								ContactsUpdatedAt: time.Unix(0, 0),
								MetadataUpdatedAt: time.Unix(0, 0),
							}
						} else {
							newUser = Metadata{PubkeyHex: c[1], ContactsUpdatedAt: time.Unix(0, 0), MetadataUpdatedAt: time.Unix(0, 0)}
						}
						createNewErr := DB.Omit("Follows").Create(&newUser).Error
						if createNewErr != nil {
							TheLog.Println("Error creating user for follow: ", createNewErr)
						}
						// use gorm insert statement to update the join table
						DB.Exec("INSERT OR IGNORE INTO metadata_follows (metadata_pubkey_hex, follow_pubkey_hex) VALUES (?, ?)", person.PubkeyHex, newUser.PubkeyHex)
					} else {
						// use gorm insert statement to update the join table
						DB.Exec("INSERT OR IGNORE INTO metadata_follows (metadata_pubkey_hex, follow_pubkey_hex) VALUES (?, ?)", person.PubkeyHex, followPerson.PubkeyHex)
					}
				}
			} else if ev.Kind == 1059 {
				// Message
				m := ChatMessage{}
				err := DB.First(&m, "event_id = ?", ev.ID).Error
				if err != nil {
					// Get active account for private key
					var account Account
					DB.Where("active = ?", true).First(&account)

					// Decrypt the message using crusher17
					sk := Decrypt(string(Password), account.Privatekey)

					decryptedContent, err := crusher17.ReceiveEvent(sk, ev)
					if err != nil {
						TheLog.Printf("Error decrypting message: %v", err)
						continue
					}

					var k14 nostr.Event
					err2 := json.Unmarshal([]byte(decryptedContent), &k14)
					if err2 != nil {
						TheLog.Printf("Error unmarshalling k14 event: %v", err2)
						continue
					}

					// Create new chat message

					firstPtag := k14.Tags.GetFirst([]string{"p"})
					if firstPtag == nil {
						TheLog.Printf("Error getting first p tag")
						continue
					}
					tagValue := firstPtag.Value()
					if tagValue == "" || !isHex(tagValue) {
						TheLog.Printf("skipping invalid pubkey from message: %s", tagValue)
						continue
					}

					m = ChatMessage{
						FromPubkey: k14.PubKey,
						ToPubkey:   tagValue,
						Content:    k14.Content,
						EventId:    ev.ID,
						Timestamp:  time.Unix(int64(k14.CreatedAt), 0),
					}

					if err := DB.Create(&m).Error; err != nil {
						TheLog.Printf("Error creating chat message: %v", err)
					} else {
						//TheLog.Printf("Received chat message from %s, content: %s", m.FromPubkey, m.Content)
					}
				}
			}
		}
	}

}
