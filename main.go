package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	qrcode "github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	_ "github.com/mattn/go-sqlite3"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// Person represents a person with name and birthday
type Person struct {
	Name     string
	Birthday time.Time
	Phone    string
}

// Birthday message templates (English - for age <= 40)
var birthdayMessageTemplates = []string{
	"🎉 Happy Birthday, श्री %s जी! 🎂\n\nWishing you a day filled with happiness and positivity. Hope the year ahead is fantastic. 🎈\n\n--%s (%s)",
	"🌟 Happy Birthday, श्री %s जी! 🎊\n\nMay your special day bring enduring joy and memorable moments. Here's to another remarkable year ahead! 🥳\n\n--%s (%s)",
	"🎂 Warmest wishes on your special day, श्री %s जी! 🎉\n\nWishing you every success and happiness in the coming year. May your ambitions continue to soar. 🎁\n\n--%s (%s)",
	"🎈 Happy Birthday, श्री %s जी! 🌈\n\nMay laughter and good health accompany you today and always. Wishing you happiness and fulfillment throughout the year! 🥳\n\n--%s (%s)",
	"🎊 Wishing you a truly happy birthday, श्री %s जी! 🎂\n\nMay this year be filled with rewarding experiences and cherished moments. You deserve the very best each day. 🎉\n\n--%s (%s)",
	"🎁 Happy Birthday, श्री %s जी! 🎈\n\nSending you heartfelt wishes for growth and happiness. May this year bring you closer to your aspirations. 🥳\n\n--%s (%s)",
	"🌺 Warm birthday greetings to someone exceptional, श्री %s जी! 🎂\n\nMay your year ahead be filled with achievements and bright opportunities. 🎉\n\n--%s (%s)",
}

// Birthday message templates (Hindi - for age > 40)
var birthdayMessageTemplatesHindi = []string{
	"🎉 जन्मदिन मुबारक हो, श्री %s जी! 🎂\n\nआपका दिन खुशियों और सकारात्मकता से भरा हो। आशा है आने वाला साल आपके लिए शानदार रहेगा। 🎈\n\n--%s (%s)",
	"🌟 जन्मदिन की शुभकामनाएँ, श्री %s जी! 🎊\n\nयह खास दिन आपके लिए खुशियाँ और सुंदर यादें लेकर आए। आपके जीवन के एक और अद्भुत वर्ष के लिए शुभकामनाएँ! 🥳\n\n--%s (%s)",
	"🎂 आपके खास दिन पर हार्दिक शुभकामनाएँ, श्री %s जी! 🎉\n\nआने वाले साल में आपको सफलता और खुशी मिले। आपके सपने और भी ऊँचे हों। 🎁\n\n--%s (%s)",
	"🎈 जन्मदिन मुबारक हो, श्री %s जी! 🌈\n\nहँसी और अच्छे स्वास्थ्य का साथ हमेशा आपके साथ रहे। नए साल में आपको आनंद और उपलब्धियों की शुभकामनाएँ! 🥳\n\n--%s (%s)",
	"🎁 जन्मदिन मुबारक हो, श्री %s जी! 🎈\n\nआपको खुशियों व उन्नति की शुभकामनाएँ। आने वाला साल आपके लक्ष्य के और करीब लाए। 🥳\n\n--%s (%s)",
}

func main() {
	// Read sender and report info from environment variables
	senderName := os.Getenv("SENDER_NAME")
	senderNumber := os.Getenv("SENDER_NUMBER")
	reportNumber := os.Getenv("REPORT_NUMBER")

	if senderName == "" || senderNumber == "" || reportNumber == "" {
		log.Fatal("SENDER_NAME, SENDER_NUMBER, and REPORT_NUMBER environment variables must be set")
		return;
	}
	// Set up logging
	dbLog := waLog.Stdout("Database", "DEBUG", true)

	// Create database container for storing WhatsApp session
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:examplestore.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}

	// Get the first device store (or create a new one)
	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		panic(err)
	}

	clientLog := waLog.Stdout("Client", "DEBUG", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)
	client.AddEventHandler(eventHandler)

	// Check if client is already logged in
	if client.Store.ID == nil {
		// Generate QR code for initial login
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			panic(err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				// Display QR code in terminal
				fmt.Println("\n📱 Scan this QR code with WhatsApp on your phone:")
				fmt.Println("👆 Open WhatsApp > Settings > Linked Devices > Link a Device")
				fmt.Println()
				qrcode.GenerateHalfBlock(evt.Code, qrcode.L, os.Stdout)
				fmt.Println()
				fmt.Println("⏳ Waiting for QR code scan...")
				fmt.Printf("Raw QR code: %s\n", evt.Code)
			} else {
				fmt.Printf("🔄 Login event: %s\n", evt.Event)
				if evt.Event == "success" {
					fmt.Println("✅ Successfully connected to WhatsApp!")
					fmt.Println("🎉 Birthday bot is now ready!")
				}
			}
		}
	} else {
		// Already logged in, just connect
		err = client.Connect()
		if err != nil {
			panic(err)
		}
	}

	// Wait for client to be fully connected and ready
	fmt.Println("⏳ Waiting for WhatsApp connection to be fully ready...")
	for !client.IsConnected() {
		time.Sleep(1 * time.Second)
	}
	
	// Additional wait to ensure device sync is complete
	fmt.Println("🔄 Allowing time for device synchronization...")
	time.Sleep(10 * time.Second)
	fmt.Println("✅ WhatsApp client is ready!")

	// Load contacts from CSV
	people, err := loadPeopleFromCSV("birthdays.csv")
	if err != nil {
		log.Printf("Error loading CSV: %v", err)
		// Create example CSV file if it doesn't exist
		createExampleCSV()
		people, err = loadPeopleFromCSV("birthdays.csv")
		if err != nil {
			panic(err)
		}
	}

	log.Printf("Loaded %d people from CSV", len(people))

	// Check birthdays immediately on startup
	checkBirthdays(client, people, senderName, senderNumber, reportNumber)

	// Set up ticker to check birthdays daily at 9 AM
	now := time.Now()
	next9AM := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, now.Location())
	if now.After(next9AM) {
		next9AM = next9AM.Add(24 * time.Hour)
	}

	// Calculate duration until next 9 AM
	duration := next9AM.Sub(now)
	log.Printf("Next birthday check in: %v", duration)

	// Create timer for first check at 9 AM
	timer := time.NewTimer(duration)

	// Create ticker for subsequent daily checks
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Set up signal handling for graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	log.Println("Birthday reminder bot is running...")
	log.Println("Press Ctrl+C to stop")

	for {
		select {
		case <-timer.C:
			log.Println("Checking birthdays...")
			checkBirthdays(client, people, senderName, senderNumber, reportNumber)
			// Timer only fires once, so we rely on ticker for subsequent checks
		case <-ticker.C:
			log.Println("Daily birthday check...")
			checkBirthdays(client, people, senderName, senderNumber, reportNumber)
		case <-c:
			log.Println("Shutting down...")
			client.Disconnect()
			return
		}
	}
}

func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		log.Printf("Received message from %s: %s", v.Info.Sender, v.Message.GetConversation())
	case *events.Receipt:
		log.Printf("Message %s was %s", v.MessageIDs[0], v.Type)
	}
}

func loadPeopleFromCSV(filename string) ([]Person, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var people []Person
	fmt.Println("Parsing CSV records...", len(records), "rows found")
	for i, record := range records {
		// Skip header row
		if i == 0 {
			continue
		}

		if len(record) < 18 {
			log.Printf("Skipping row %d: not enough columns (expected 18+, got %d)", i+1, len(record))
			continue
		}

		// Extract relevant fields from the CSV
		name := strings.TrimSpace(record[5])           // Member Name(Eng.) - column 6 (index 5)
		birthdayStr := strings.TrimSpace(record[17])   // DOB - column 18 (index 17)
		primaryPhone := strings.TrimSpace(record[14])  // Mobile No. 1 - column 15 (index 14)
		whatsappPhone := strings.TrimSpace(record[15]) // Whatsapp No. 2 - column 16 (index 15)

		// Skip if name is empty
		if name == "" {
			log.Printf("Skipping row %d: empty name", i+1)
			continue
		}

		// Skip if no birthday information
		if birthdayStr == "" {
			log.Printf("Skipping %s: empty birthday", name)
			continue
		}

		// Choose phone number (prefer WhatsApp number, fallback to primary)
		phone := primaryPhone
		if phone == "" {
			phone = whatsappPhone
		}

		// Skip if no phone number available
		if phone == "" {
			log.Printf("Skipping %s: no phone number available", name)
			continue
		}

		// Parse birthday (expect format: M/D/YY or MM/DD/YYYY)
		var birthday time.Time
		for _, layout := range []string{
			"1/2/06",     // M/D/YY (e.g., 7/5/49)
			"01/02/06",   // MM/DD/YY (e.g., 07/05/49)
			"1/2/2006",   // M/D/YYYY (e.g., 7/5/1949)
			"01/02/2006", // MM/DD/YYYY (e.g., 07/05/1949)
			"02/01/2006", // DD/MM/YYYY
			"02-01-2006", // DD-MM-YYYY
			"2006-01-02", // YYYY-MM-DD
		} {
			if t, err := time.Parse(layout, birthdayStr); err == nil {
				// Handle 2-digit years (assume people are between 0-100 years old)
				if t.Year() < 1900 {
					// For 2-digit years: 00-30 -> 2000-2030, 31-99 -> 1931-1999
					if t.Year() <= 30 {
						t = t.AddDate(2000, 0, 0)
					} else {
						t = t.AddDate(1900, 0, 0)
					}
				}
				birthday = t
				break
			}
		}

		if birthday.IsZero() {
			log.Printf("Skipping %s: invalid date format %s", name, birthdayStr)
			continue
		}

		// Clean phone number (remove spaces, dashes, etc.)
		phone = strings.ReplaceAll(phone, " ", "")
		phone = strings.ReplaceAll(phone, "-", "")
		phone = strings.ReplaceAll(phone, "(", "")
		phone = strings.ReplaceAll(phone, ")", "")
		phone = strings.ReplaceAll(phone, "+", "") // Remove + sign as WhatsApp JID doesn't use it

		// Ensure phone number starts with country code (without +)
		if !strings.HasPrefix(phone, "91") {
			// Assuming Indian numbers, add 91 if not present
			phone = "91" + phone
		}

		people = append(people, Person{
			Name:     name,
			Birthday: birthday,
			Phone:    phone,
		})
	}

	return people, nil
}

func createExampleCSV() {
	file, err := os.Create("birthdays.csv")
	if err != nil {
		log.Printf("Error creating example CSV: %v", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header matching your Zone 4 CSV format
	writer.Write([]string{"Serial", "S.N.", "Member ID", "Family ID", "Member Count", "Member Name(Eng.)", "Member Name(Hindi)", "Father/Husband Name(Eng.)", "Father/Husband Name(Hindi)", "Age(Yrs.)", "Gotra", "Chokari", "Zone", "Birth Place", "Mobile No. 1", "Whatsapp No. 2", "Email", "DOB", "Gender", "Status", "Address", "Address Hindi", "Pincode"})

	// Write example data
	writer.Write([]string{"1", "10790", "409", "109", "1", "John Doe", "जॉन डो", "Late. Father Name", "स्व. पिता जी", "35 Yrs.", "Gotra", "Area", "Zone-4", "Jaipur", "9123456789", "9123456789", "john@email.com", "15/1/89", "Male", "Head of family", "Address Line 1", "पता लाइन 1", "302021"})
	writer.Write([]string{"2", "10791", "410", "109", "2", "Jane Smith", "जेन स्मिथ", "John Doe", "जॉन डो", "32 Yrs.", "Gotra", "Area", "Zone-4", "Jaipur", "9876543210", "9876543210", "jane@email.com", "22/12/92", "Female", "Member", "Address Line 2", "पता लाइन 2", "302021"})

	log.Println("Created example birthdays.csv file with your CSV format. Please replace with your actual Zone 4 Copy.csv data or rename your file to birthdays.csv")
}

func checkBirthdays(client *whatsmeow.Client, people []Person, senderName, senderNumber, reportNumber string) {
	today := time.Now()
	fmt.Println("Checking birthdays...", len(people), "people loaded")

	// Check if client is connected before proceeding
	if !client.IsConnected() {
		log.Println("❌ WhatsApp client is not connected. Cannot send birthday messages.")
		return
	}

	birthdayCount := 0
	var sentPeople []Person
	for _, person := range people {
		fmt.Println("Checking:", person.Name, person.Birthday.Format("2006-Jan-02"))
		// Check if today is their birthday (ignoring year)
		if person.Birthday.Day() == today.Day() && person.Birthday.Month() == today.Month() {
			birthdayCount++
			log.Printf("🎂 Found birthday for %s: %s", person.Name, person.Birthday.Format("2006-Jan-02"))
			age := today.Year() - person.Birthday.Year()

			// Select message template based on age (Hindi for age > 40, English otherwise)
			var message string
			if age > 40 {
				templateIndex := rand.Intn(len(birthdayMessageTemplatesHindi))
				message = fmt.Sprintf(birthdayMessageTemplatesHindi[templateIndex], person.Name, senderName, senderNumber)
				log.Printf("📝 Using Hindi template for %s", person.Name)
			} else {
				templateIndex := rand.Intn(len(birthdayMessageTemplates))
				message = fmt.Sprintf(birthdayMessageTemplates[templateIndex], person.Name, senderName, senderNumber)
				log.Printf("📝 Using English template for %s", person.Name)
			}

			err := sendMessage(client, person.Phone, message)
			if err != nil {
				log.Printf("❌ Error sending birthday message to %s (%s): %v", person.Name, person.Phone, err)
			} else {
				log.Printf("✅ Sent birthday message to %s (%s)", person.Name, person.Phone)
				sentPeople = append(sentPeople, person)
			}

			// Add a small delay between messages to avoid rate limiting
			time.Sleep(3 * time.Second)
		}
	}

	if birthdayCount == 0 {
		log.Println("📅 No birthdays today!")
	} else {
		log.Printf("🎉 Found %d birthday(s) today!", birthdayCount)
		// Report to the report number
		reportMsg := "Birthday messages sent today:\n"
		for _, p := range sentPeople {
			reportMsg += fmt.Sprintf("%s (%s)\n", p.Name, p.Phone)
		}
		if len(sentPeople) == 0 {
			reportMsg += "(No messages sent)"
		}
		err := sendMessage(client, reportNumber, reportMsg)
		if err != nil {
			log.Printf("❌ Error sending report to %s: %v", reportNumber, err)
		} else {
			log.Printf("✅ Sent report to %s", reportNumber)
		}
	}
}

func sendMessage(client *whatsmeow.Client, phoneNumber, message string) error {
	// Check if client is connected
	if !client.IsConnected() {
		return fmt.Errorf("WhatsApp client is not connected")
	}

	// Parse the phone number into a JID
	jid, err := types.ParseJID(phoneNumber + "@s.whatsapp.net")
	if err != nil {
		return fmt.Errorf("failed to parse phone number: %v", err)
	}

	// Create the message
	msg := &waProto.Message{
		Conversation: proto.String(message),
	}

	// Create context with timeout to avoid hanging
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Send the message
	_, err = client.SendMessage(ctx, jid, msg)
	if err != nil {
		return fmt.Errorf("failed to send message: %v", err)
	}

	return nil
}
