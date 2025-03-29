package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/awesome-gocui/gocui"
	"github.com/nbd-wtf/go-nostr"
)

// ZapRequest represents the data needed to create a zap request
type ZapRequest struct {
	Relays          []string
	EventID         string
	Amount          int64
	Comment         string
	Lud16           string
	RecipientPubkey string
}

// LNURLPayResponse represents the response from a LNURL-pay request
type LNURLPayResponse struct {
	Callback    string `json:"callback"`
	Tag         string `json:"tag"`
	MaxSendable int64  `json:"maxSendable"`
	MinSendable int64  `json:"minSendable"`
	Metadata    string `json:"metadata"`
	AllowsNostr bool   `json:"allowsNostr"`
	NostrPubkey string `json:"nostrPubkey"`
}

// LNURLPayValues represents the query parameters for a LNURL-pay callback
type LNURLPayValues struct {
	Amount          int64  `json:"amount"`
	Nostr           string `json:"nostr,omitempty"`
	LnurlPayComment string `json:"comment,omitempty"`
}

// LNURLPayInvoiceResponse represents the response from a LNURL-pay callback
type LNURLPayInvoiceResponse struct {
	PR            string `json:"pr"`
	SuccessAction *struct {
		Tag     string `json:"tag"`
		Message string `json:"message,omitempty"`
	} `json:"successAction,omitempty"`
	Routes []interface{} `json:"routes,omitempty"`
}

// CurrentZapRequest stores the active zap request data
var CurrentZapRequest *ZapRequest

// zapUserMenu opens a menu to send a zap to a user
func zapUserMenu(g *gocui.Gui, v *gocui.View) error {
	// Get the highlighted user's pubkey
	_, cy := v.Cursor()

	// Find the pubkey from the highlighted line

	// Check if the user has a lightning address
	metadata := displayV2Meta[cy]
	if metadata.Lud16 == "" {
		return fmt.Errorf("selected user does not have a lightning address")
	}

	// Create the zap menu
	maxX, maxY := g.Size()
	if v, err := g.SetView("zapmenu", maxX/2-40, maxY/2-10, maxX/2+40, maxY/2+10, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}

		v.Title = "Zap Menu"
		v.Highlight = true
		v.SelBgColor = activeTheme.HighlightBg
		v.SelFgColor = activeTheme.HighlightFg
		v.Editable = false
		v.BgColor = activeTheme.Bg
		v.FgColor = activeTheme.Fg

		// Display user information
		fmt.Fprintf(v, "Sending zap to: %s\n", metadata.Name)
		fmt.Fprintf(v, "Lightning Address: %s\n\n", metadata.Lud16)

		// Display amount options
		fmt.Fprintf(v, "1. 1,000 sats\n")
		fmt.Fprintf(v, "2. 5,000 sats\n")
		fmt.Fprintf(v, "3. 10,000 sats\n")
		fmt.Fprintf(v, "4. 21,000 sats\n")
		fmt.Fprintf(v, "5. 50,000 sats\n")
		fmt.Fprintf(v, "6. 100,000 sats\n")
		fmt.Fprintf(v, "7. Custom amount\n")
		fmt.Fprintf(v, "8. Cancel\n")

		// Store pubkey and lightning address in the global variable
		CurrentZapRequest = &ZapRequest{
			RecipientPubkey: metadata.PubkeyHex,
			Lud16:           metadata.Lud16,
		}

		// Set keybindings for the zap menu
		g.SetKeybinding("zapmenu", gocui.KeyEsc, gocui.ModNone, cancelZap)
		g.SetKeybinding("zapmenu", gocui.KeyEnter, gocui.ModNone, selectZapAmount)
		g.SetKeybinding("zapmenu", gocui.KeyArrowDown, gocui.ModNone, cursorDown)
		g.SetKeybinding("zapmenu", gocui.KeyArrowUp, gocui.ModNone, cursorUp)

		// Set vim-style cursor movement
		g.SetKeybinding("zapmenu", rune(0x6a), gocui.ModNone, cursorDown) // j
		g.SetKeybinding("zapmenu", rune(0x6b), gocui.ModNone, cursorUp)   // k

		if _, err := g.SetCurrentView("zapmenu"); err != nil {
			return err
		}
	}
	return nil
}

// cancelZap closes the zap menu
func cancelZap(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("zapmenu")
	g.SetCurrentView("v2")
	return nil
}

// selectZapAmount processes the selected zap amount
func selectZapAmount(g *gocui.Gui, v *gocui.View) error {
	_, cy := v.Cursor()

	// Get the zap request data
	zapReq := CurrentZapRequest

	// Set amount based on selection
	switch cy {
	case 0: // 1,000 sats
		zapReq.Amount = 1000 * 1000 // Convert to millisats
		return processZap(g, v, zapReq)
	case 1: // 5,000 sats
		zapReq.Amount = 5000 * 1000
		return processZap(g, v, zapReq)
	case 2: // 10,000 sats
		zapReq.Amount = 10000 * 1000
		return processZap(g, v, zapReq)
	case 3: // 21,000 sats
		zapReq.Amount = 21000 * 1000
		return processZap(g, v, zapReq)
	case 4: // 50,000 sats
		zapReq.Amount = 50000 * 1000
		return processZap(g, v, zapReq)
	case 5: // 100,000 sats
		zapReq.Amount = 100000 * 1000
		return processZap(g, v, zapReq)
	case 6: // Custom amount
		return openCustomAmountInput(g, v, zapReq)
	case 7: // Cancel
		return cancelZap(g, v)
	default:
		return nil
	}
}

// openCustomAmountInput opens an input for a custom zap amount
func openCustomAmountInput(g *gocui.Gui, v *gocui.View, zapReq *ZapRequest) error {
	maxX, maxY := g.Size()
	g.DeleteView("zapmenu")

	if v, err := g.SetView("zapamount", maxX/2-30, maxY/2-3, maxX/2+30, maxY/2+3, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}

		v.Title = "Enter Amount (sats)"
		v.Editable = true
		v.BgColor = activeTheme.Bg
		v.FgColor = activeTheme.Fg

		// Set keybindings for the amount input
		g.SetKeybinding("zapamount", gocui.KeyEsc, gocui.ModNone, cancelZapAmount)
		g.SetKeybinding("zapamount", gocui.KeyEnter, gocui.ModNone, submitZapAmount)

		if _, err := g.SetCurrentView("zapamount"); err != nil {
			return err
		}
	}
	return nil
}

// cancelZapAmount cancels the custom amount input
func cancelZapAmount(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("zapamount")
	g.SetCurrentView("v2")
	return nil
}

// submitZapAmount processes the custom amount input
func submitZapAmount(g *gocui.Gui, v *gocui.View) error {
	// Use the global zapReq
	zapReq := CurrentZapRequest
	if zapReq == nil {
		return fmt.Errorf("invalid zap request data")
	}

	// Get the amount from the input
	amountStr := strings.TrimSpace(v.Buffer())
	amount, err := strconv.ParseInt(amountStr, 10, 64)
	if err != nil || amount <= 0 {
		return showError(g, "Invalid amount, please enter a positive number")
	}

	// Convert to millisats
	zapReq.Amount = amount * 1000

	// Process the zap
	g.DeleteView("zapamount")
	return processZap(g, v, zapReq)
}

// processZap initiates the zap process according to NIP-57
func processZap(g *gocui.Gui, v *gocui.View, zapReq *ZapRequest) error {
	// Show zap processing view
	maxX, maxY := g.Size()
	if v, err := g.SetView("zapprocessing", maxX/2-40, maxY/2-8, maxX/2+40, maxY/2+8, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}

		v.Title = "Processing Zap"
		v.BgColor = activeTheme.Bg
		v.FgColor = activeTheme.Fg

		fmt.Fprintf(v, "Sending %d sats to %s\n\n", zapReq.Amount/1000, zapReq.Lud16)
		fmt.Fprintf(v, "Resolving lightning address...\n")

		// Process in a goroutine to avoid blocking the UI
		go func() {
			// Get active account for signing
			var account Account
			DB.Where("active = ?", true).First(&account)
			if account.Pubkey == "" {
				g.Update(func(g *gocui.Gui) error {
					return showError(g, "No active account found")
				})
				return
			}

			// Parse the lightning address
			parts := strings.Split(zapReq.Lud16, "@")
			if len(parts) != 2 {
				g.Update(func(g *gocui.Gui) error {
					return showError(g, "Invalid lightning address format")
				})
				return
			}
			username, domain := parts[0], parts[1]

			// Construct the LNURL-pay endpoint
			lnurlEndpoint := fmt.Sprintf("https://%s/.well-known/lnurlp/%s", domain, username)

			// 1. Fetch the LNURL-pay data
			g.Update(func(g *gocui.Gui) error {
				if v, err := g.View("zapprocessing"); err == nil {
					fmt.Fprintf(v, "Fetching LNURL-pay data...\n")
				}
				return nil
			})

			resp, err := http.Get(lnurlEndpoint)
			if err != nil {
				g.Update(func(g *gocui.Gui) error {
					return showError(g, fmt.Sprintf("Error fetching LNURL-pay data: %v", err))
				})
				return
			}
			defer resp.Body.Close()

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				g.Update(func(g *gocui.Gui) error {
					return showError(g, fmt.Sprintf("Error reading LNURL-pay response: %v", err))
				})
				return
			}

			var lnurlPayResp LNURLPayResponse
			if err := json.Unmarshal(body, &lnurlPayResp); err != nil {
				g.Update(func(g *gocui.Gui) error {
					return showError(g, fmt.Sprintf("Error parsing LNURL-pay response: %v", err))
				})
				return
			}

			// Check if the amount is within allowed range
			if zapReq.Amount < lnurlPayResp.MinSendable || zapReq.Amount > lnurlPayResp.MaxSendable {
				g.Update(func(g *gocui.Gui) error {
					return showError(g, fmt.Sprintf("Amount out of range. Min: %d, Max: %d millisats",
						lnurlPayResp.MinSendable, lnurlPayResp.MaxSendable))
				})
				return
			}

			// Check if Nostr is allowed
			if !lnurlPayResp.AllowsNostr || lnurlPayResp.NostrPubkey == "" {
				g.Update(func(g *gocui.Gui) error {
					return showError(g, "This lightning address does not support Nostr zaps")
				})
				return
			}

			// 2. Create a zap request event (kind 9734)
			g.Update(func(g *gocui.Gui) error {
				if v, err := g.View("zapprocessing"); err == nil {
					fmt.Fprintf(v, "Creating zap request...\n")
				}
				return nil
			})

			// Get relays for the zap request
			var relayStatuses []RelayStatus
			DB.Find(&relayStatuses)
			var relays []string
			for _, relay := range relayStatuses {
				relays = append(relays, relay.Url)
			}

			// Create zap request event
			var tags nostr.Tags
			tags = append(tags, nostr.Tag{"p", zapReq.RecipientPubkey})
			tags = append(tags, nostr.Tag{"relays"})
			for _, r := range relays {
				tags[len(tags)-1] = append(tags[len(tags)-1], r)
			}
			tags = append(tags, nostr.Tag{"amount", fmt.Sprintf("%d", zapReq.Amount)})

			zapRequestEvent := nostr.Event{
				Kind:      9734, // Zap Request
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Tags:      tags,
				Content:   zapReq.Comment,
			}

			// Sign the zap request with the active account's private key
			decryptedPrivateKey := Decrypt(string(Password), account.Privatekey)

			zapRequestEvent.PubKey = account.Pubkey
			err = zapRequestEvent.Sign(decryptedPrivateKey)
			if err != nil {
				g.Update(func(g *gocui.Gui) error {
					return showError(g, fmt.Sprintf("Error signing zap request: %v", err))
				})
				return
			}

			// Serialize the zap request event
			zapRequestJSON, err := json.Marshal(zapRequestEvent)
			if err != nil {
				g.Update(func(g *gocui.Gui) error {
					return showError(g, fmt.Sprintf("Error serializing zap request: %v", err))
				})
				return
			}

			// 3. Send the zap request to the LNURL-pay callback
			g.Update(func(g *gocui.Gui) error {
				if v, err := g.View("zapprocessing"); err == nil {
					fmt.Fprintf(v, "Requesting invoice...\n")
				}
				return nil
			})

			// Prepare callback URL with parameters
			callbackURL, err := url.Parse(lnurlPayResp.Callback)
			if err != nil {
				g.Update(func(g *gocui.Gui) error {
					return showError(g, fmt.Sprintf("Error parsing callback URL: %v", err))
				})
				return
			}

			query := callbackURL.Query()
			query.Set("amount", fmt.Sprintf("%d", zapReq.Amount))
			query.Set("nostr", string(zapRequestJSON))
			if zapReq.Comment != "" {
				query.Set("comment", zapReq.Comment)
			}
			callbackURL.RawQuery = query.Encode()

			// Send the request to get the invoice
			invoiceResp, err := http.Get(callbackURL.String())
			if err != nil {
				g.Update(func(g *gocui.Gui) error {
					return showError(g, fmt.Sprintf("Error fetching invoice: %v", err))
				})
				return
			}
			defer invoiceResp.Body.Close()

			invoiceBody, err := ioutil.ReadAll(invoiceResp.Body)
			if err != nil {
				g.Update(func(g *gocui.Gui) error {
					return showError(g, fmt.Sprintf("Error reading invoice response: %v", err))
				})
				return
			}

			var invoiceData LNURLPayInvoiceResponse
			if err := json.Unmarshal(invoiceBody, &invoiceData); err != nil {
				g.Update(func(g *gocui.Gui) error {
					return showError(g, fmt.Sprintf("Error parsing invoice response: %v", err))
				})
				return
			}

			// 4. Display the invoice
			g.Update(func(g *gocui.Gui) error {
				g.DeleteView("zapprocessing")

				maxX, maxY := g.Size()
				if v, err := g.SetView("zapinvoice", maxX/2-40, maxY/2-12, maxX/2+40, maxY/2+12, 0); err != nil {
					if !errors.Is(err, gocui.ErrUnknownView) {
						return err
					}

					v.Title = "Lightning Invoice"
					v.BgColor = activeTheme.Bg
					v.FgColor = activeTheme.Fg

					// Display the invoice details
					fmt.Fprintf(v, "Amount: %d sats\n\n", zapReq.Amount/1000)
					
					// Display ASCII QR code representation
					fmt.Fprintf(v, "QR Code:\n")
					fmt.Fprintf(v, generateTextQRCode(invoiceData.PR))
					fmt.Fprintf(v, "\nInvoice:\n%s\n\n", invoiceData.PR)
					fmt.Fprintf(v, "[Press ESC to close]\n")

					// Set keybinding to close the invoice view
					g.SetKeybinding("zapinvoice", gocui.KeyEsc, gocui.ModNone, closeZapInvoice)

					if _, err := g.SetCurrentView("zapinvoice"); err != nil {
						return err
					}
				}
				return nil
			})
		}()

		// Set keybinding to cancel the processing
		g.SetKeybinding("zapprocessing", gocui.KeyEsc, gocui.ModNone, cancelZapProcessing)

		if _, err := g.SetCurrentView("zapprocessing"); err != nil {
			return err
		}
	}
	return nil
}

// generateTextQRCode creates a simple text-based QR code representation
// This is a very basic implementation and does not generate scannable QR codes
func generateTextQRCode(data string) string {
	// Limit the size of the QR code
	size := 21 // Standard QR size

	// Initialize the QR matrix with empty spaces
	matrix := make([][]bool, size)
	for i := range matrix {
		matrix[i] = make([]bool, size)
	}

	// Add the three finder patterns (corners)
	// Top-left finder pattern
	addFinderPattern(matrix, 0, 0)
	// Top-right finder pattern
	addFinderPattern(matrix, 0, size-7)
	// Bottom-left finder pattern
	addFinderPattern(matrix, size-7, 0)

	// Add timing patterns (the lines connecting finder patterns)
	for i := 8; i < size-8; i++ {
		matrix[6][i] = i%2 == 0 // Horizontal timing pattern
		matrix[i][6] = i%2 == 0 // Vertical timing pattern
	}

	// Add alignment pattern for larger QR codes
	if size >= 25 {
		addAlignmentPattern(matrix, size-9, size-9)
	}

	// Simple data pattern based on the input string
	// This just creates a pseudo-unique pattern and is not a real QR encoding
	charSum := 0
	for _, c := range data {
		charSum += int(c)
	}
	
	// Fill some of the data area with a pattern derived from the input
	for i := 9; i < size-9; i++ {
		for j := 9; j < size-9; j++ {
			// Create a pseudo-random pattern based on positions and input data
			matrix[i][j] = (i*j + charSum) % 3 == 0
		}
	}

	// Convert the matrix to a string
	var result strings.Builder
	result.WriteString("╔")
	for i := 0; i < size+2; i++ {
		result.WriteString("═")
	}
	result.WriteString("╗\n")

	for i := 0; i < size; i++ {
		result.WriteString("║ ")
		for j := 0; j < size; j++ {
			if matrix[i][j] {
				result.WriteString("██")
			} else {
				result.WriteString("  ")
			}
		}
		result.WriteString(" ║\n")
	}

	result.WriteString("╚")
	for i := 0; i < size+2; i++ {
		result.WriteString("═")
	}
	result.WriteString("╝")

	return result.String()
}

// addFinderPattern adds a finder pattern at the specified position
func addFinderPattern(matrix [][]bool, row, col int) {
	// Outer border
	for i := 0; i < 7; i++ {
		matrix[row][col+i] = true
		matrix[row+6][col+i] = true
		matrix[row+i][col] = true
		matrix[row+i][col+6] = true
	}

	// Inner square
	for i := 2; i < 5; i++ {
		for j := 2; j < 5; j++ {
			matrix[row+i][col+j] = true
		}
	}
}

// addAlignmentPattern adds an alignment pattern at the specified position
func addAlignmentPattern(matrix [][]bool, row, col int) {
	// Outer border
	for i := 0; i < 5; i++ {
		matrix[row][col+i] = true
		matrix[row+4][col+i] = true
		matrix[row+i][col] = true
		matrix[row+i][col+4] = true
	}

	// Center dot
	matrix[row+2][col+2] = true
}

// showError displays an error message in a popup
func showError(g *gocui.Gui, message string) error {
	maxX, maxY := g.Size()

	// Close any existing error view
	g.DeleteView("error")

	if v, err := g.SetView("error", maxX/2-30, maxY/2-4, maxX/2+30, maxY/2+4, 0); err != nil {
		if !errors.Is(err, gocui.ErrUnknownView) {
			return err
		}

		v.Title = "Error"
		v.BgColor = activeTheme.Bg
		v.FgColor = activeTheme.Fg

		fmt.Fprintf(v, "%s\n\n", message)
		fmt.Fprintf(v, "[Press ESC to close]\n")

		// Set keybinding to close the error view
		g.SetKeybinding("error", gocui.KeyEsc, gocui.ModNone, closeErrorView)

		if _, err := g.SetCurrentView("error"); err != nil {
			return err
		}
	}
	return nil
}

// closeErrorView closes the error view
func closeErrorView(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("error")
	g.SetCurrentView("v2")
	return nil
}

// cancelZapProcessing cancels the zap processing
func cancelZapProcessing(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("zapprocessing")
	g.SetCurrentView("v2")
	return nil
}

// closeZapInvoice closes the zap invoice view
func closeZapInvoice(g *gocui.Gui, v *gocui.View) error {
	g.DeleteView("zapinvoice")
	g.SetCurrentView("v2")
	return nil
}
