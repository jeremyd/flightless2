package main

import (
	"log"
	"os"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Account struct {
	Pubkey       string `gorm:"primaryKey;size:65"`
	PubkeyNpub   string `gorm:"size:65"`
	Privatekey   string `gorm:"size:1024"` // encrypted
	Active       bool
	ChatMessages []ChatMessage `gorm:"foreignKey:ToPubkey;references:Pubkey"`
}

type Login struct {
	PasswordHash string `gorm:"size:256"` //salted and hashed
}

type Metadata struct {
	PubkeyHex         string `gorm:"primaryKey;size:65"`
	PubkeyNpub        string `gorm:"size:65"`
	Name              string `gorm:"size:1024"`
	About             string `gorm:"size:4096"`
	Nip05             string `gorm:"size:512"`
	Lud06             string `gorm:"size:2048"`
	Lud16             string `gorm:"size:512"`
	Website           string `gorm:"size:512"`
	DisplayName       string `gorm:"size:512"`
	Picture           string `gorm:"size:65535"`
	TotalFollows      int
	UpdatedAt         time.Time   `gorm:"autoUpdateTime"`
	ContactsUpdatedAt time.Time   `gorm:"default:CURRENT_TIMESTAMP"`
	MetadataUpdatedAt time.Time   `gorm:"default:CURRENT_TIMESTAMP"`
	Follows           []*Metadata `gorm:"many2many:metadata_follows;foreignKey:PubkeyHex;references:PubkeyHex"`
	DMRelays          []DMRelay   `gorm:"foreignKey:PubkeyHex;references:PubkeyHex"`
	RawJsonContent    string      `gorm:"size:512000"`
}

type DMRelay struct {
	ID        int64     `gorm:"primaryKey;autoIncrement"`
	PubkeyHex string    `gorm:"size:65"`
	Url       string    `gorm:"size:512"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
	CreatedAt time.Time `gorm:"autoUpdateTime"`
}

type RelayStatus struct {
	Url       string    `gorm:"primaryKey;size:512"`
	Status    string    `gorm:"size:512"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
	// change these defaults to something closer to zero
	LastEOSE  time.Time `gorm:"default:CURRENT_TIMESTAMP"`
	LastDisco time.Time `gorm:"default:CURRENT_TIMESTAMP"`
}

type ChatMessage struct {
	ID         int64     `gorm:"primaryKey;autoIncrement"`
	EventId    string    `gorm:"size:65"`
	FromPubkey string    `gorm:"size:65"`
	ToPubkey   string    `gorm:"size:65"`
	Content    string    `gorm:"size:65535"`
	Timestamp  time.Time `gorm:"autoUpdateTime"`
}

type RelayList struct {
	ID        int64     `gorm:"primaryKey;autoIncrement"`
	PubkeyHex string    `gorm:"size:65;index"`
	Url       string    `gorm:"size:512"`
	Read      bool      `gorm:"default:true"`
	Write     bool      `gorm:"default:true"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

func GetGormConnection() *gorm.DB {
	file, err := os.OpenFile("flightless.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		// Handle error
		panic(err)
	}

	TheLog = log.New(file, "", log.LstdFlags) // io writer
	newLogger := logger.New(
		TheLog,
		logger.Config{
			SlowThreshold:             time.Second,  // Slow SQL threshold
			LogLevel:                  logger.Error, // Log level
			IgnoreRecordNotFoundError: true,         // Ignore ErrRecordNotFound error for logger
			Colorful:                  false,        // Disable color
		},
	)

	dsn, foundDsn := os.LookupEnv("DB")
	if !foundDsn {
		dsn = "flightless.db?cache=shared&mode=rwc"
	}

	db, dberr := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: newLogger})

	if dberr != nil {
		panic(dberr)
	}
	db.Logger.LogMode(logger.Silent)

	return db
}

func RunMigrations() {
	if err := DB.AutoMigrate(&Login{}); err != nil {
		log.Fatalf("Failed to migrate Login table: %v", err)
	}
	if err := DB.AutoMigrate(&Account{}); err != nil {
		log.Fatalf("Failed to migrate Account table: %v", err)
	}
	if err := DB.AutoMigrate(&DMRelay{}); err != nil {
		log.Fatalf("Failed to migrate DMRelay table: %v", err)
	}
	if err := DB.AutoMigrate(&Metadata{}); err != nil {
		log.Fatalf("Failed to migrate Metadata table: %v", err)
	}
	if err := DB.AutoMigrate(&RelayStatus{}); err != nil {
		log.Fatalf("Failed to migrate RelayStatus table: %v", err)
	}
	if err := DB.AutoMigrate(&ChatMessage{}); err != nil {
		log.Fatalf("Failed to migrate ChatMessage table: %v", err)
	}
	if err := DB.AutoMigrate(&RelayList{}); err != nil {
		log.Fatalf("Failed to migrate RelayList table: %v", err)
	}
}
