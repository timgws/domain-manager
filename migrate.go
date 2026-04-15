package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/asdine/storm/v3"
	"github.com/spf13/cobra"
	bolt "go.etcd.io/bbolt"
)

type legacyDomainRecord struct {
	ID        int    `storm:"id,increment"`
	Domain    string `storm:"unique"`
	Username  string
	CreatedAt time.Time
}

var migrateDomainsForce bool
var migrateDomainsDryRun bool
var migrateDomainsDropOld bool

var migrateDomainsCmd = &cobra.Command{
	Use:   "migrate-domains",
	Short: "Migrate legacy DomainRecord entries into DomainData",
	RunE: func(cmd *cobra.Command, args []string) error {
		db := initDB()
		defer db.Close()

		legacyRecords, err := loadLegacyDomainRecords(db)
		if err != nil {
			return err
		}

		if len(legacyRecords) == 0 {
			fmt.Println("No legacy DomainRecord records found.")
			return nil
		}

		fmt.Printf("Found %d legacy DomainRecord entries.\n", len(legacyRecords))

		if migrateDomainsDryRun {
			for _, record := range legacyRecords {
				fmt.Printf("- %s (%s)\n", record.Domain, record.Username)
			}
			return nil
		}

		if err := prepareDomainDataBucket(db, migrateDomainsForce); err != nil {
			return err
		}

		for _, record := range legacyRecords {
			data := DomainData{
				ID:        record.ID,
				Domain:    record.Domain,
				Username:  record.Username,
				CreatedAt: record.CreatedAt,
			}
			enrichDomain(&data)

			if err := db.Save(&data); err != nil {
				return fmt.Errorf("failed to migrate %s: %w", record.Domain, err)
			}
		}

		if migrateDomainsDropOld {
			if err := dropRawBucket(db, "DomainRecord"); err != nil {
				return fmt.Errorf("failed to drop legacy bucket: %w", err)
			}
			fmt.Println("🧹 Dropped legacy DomainRecord bucket.")
		}

		fmt.Printf("✅ Migrated %d domains into DomainData.\n", len(legacyRecords))
		return nil
	},
}

func loadLegacyDomainRecords(db *storm.DB) ([]legacyDomainRecord, error) {
	var records []legacyDomainRecord

	err := db.Bolt.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("DomainRecord"))
		if bucket == nil {
			return nil
		}

		return bucket.ForEach(func(k, v []byte) error {
			// nested buckets (indexes/metadata) have nil values
			if v == nil {
				return nil
			}

			var record legacyDomainRecord
			if err := db.Codec().Unmarshal(v, &record); err != nil {
				return fmt.Errorf("failed to decode legacy record %x: %w", k, err)
			}

			if record.Domain == "" {
				return nil
			}

			records = append(records, record)
			return nil
		})
	})

	return records, err
}

func prepareDomainDataBucket(db *storm.DB, force bool) error {
	var existing []DomainData
	if err := db.All(&existing); err != nil {
		return err
	}

	if len(existing) == 0 {
		return nil
	}

	if !force {
		return fmt.Errorf(
			"DomainData already contains %d record(s); rerun with --force to replace them",
			len(existing),
		)
	}

	if err := db.Drop(&DomainData{}); err != nil && !errors.Is(err, storm.ErrNotFound) {
		return fmt.Errorf("failed to clear DomainData bucket: %w", err)
	}

	return nil
}

func dropRawBucket(db *storm.DB, name string) error {
	return db.Bolt.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(name))
		if bucket == nil {
			return nil
		}
		return tx.DeleteBucket([]byte(name))
	})
}
