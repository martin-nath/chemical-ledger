package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/martin-nath/chemical-ledger/db"
)

func InsertData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var entry Entry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, "Invalid JSON data: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if entry.Type == "" || entry.Date == "" || entry.CompoundName == "" ||
		entry.QuantityPerUnit <= 0 || entry.Scale == "" || entry.NumOfUnits <= 0 {
		http.Error(w, "Missing or invalid required fields", http.StatusBadRequest)
		return
	}

	if entry.Date > time.Now().Format("2006-01-02") {
		http.Error(w, "Invalid date", http.StatusBadRequest)
		return
	}

	_, dateErr := time.Parse("2006-01-02", entry.Date)
	if dateErr != nil {
		http.Error(w, "Invalid date", http.StatusBadRequest)
		return
	}

	// Compute chemical ID from chemical name in CamelCase
	chemicalID := toCamelCase(entry.CompoundName)

	// Calculate total entry quantity (units * quantity per unit)
	txnQuantity := entry.NumOfUnits * entry.QuantityPerUnit

	const (
		entryIncoming = "Incoming"
		entryOutgoing = "Outgoing"
	)

	var currentStock int
	err := db.Db.QueryRow("SELECT net_stock FROM chemicals WHERE id = ?", chemicalID).Scan(&currentStock)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Chemical doesn't exist
		switch entry.Type {
		case entryOutgoing:
			http.Error(w, "Cannot process outgoing entry: stock is less", http.StatusBadRequest)
			return

		case entryIncoming:
			insertQuery := `
                INSERT INTO chemicals (id, name, net_stock)
                VALUES (?, ?, ?)
            `
			_, err := db.Db.Exec(insertQuery, chemicalID, entry.CompoundName, txnQuantity)
			if err != nil {
				log.Println("Error inserting chemical:", err)
				http.Error(w, "Failed to insert chemical: "+err.Error(), http.StatusInternalServerError)
				return
			}
			currentStock = txnQuantity
			w.WriteHeader(http.StatusOK)
			return

		default:
			http.Error(w, "Invalid entry type", http.StatusBadRequest)
			return
		}

	case err != nil:
		log.Println("Error fetching chemical:", err)
		http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
		return

	default:
		// Chemical exists
		switch entry.Type {
		case entryIncoming:
			currentStock += txnQuantity

		case entryOutgoing:
			if currentStock < txnQuantity {
				http.Error(w, "Stock is less than requested outgoing quantity", http.StatusBadRequest)
				return
			}
			currentStock -= txnQuantity

		default:
			http.Error(w, "Invalid entry type", http.StatusBadRequest)
			return
		}

		_, err := db.Db.Exec("UPDATE chemicals SET net_stock = ? WHERE id = ?", currentStock, chemicalID)
		if err != nil {
			log.Println("Error updating net_stock:", err)
			http.Error(w, "Failed to update net_stock: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Generate a custom entry ID:
	// Start with "I" if Incoming or "O" if Outgoing plus the chemicalID and a Unix timestamp.
	var typePrefix string

	switch entry.Type {
	case "Incoming":
		typePrefix = "I"
	case "Outgoing":
		typePrefix = "O"
	case "Both":
		break
	default:
		http.Error(w, "Invalid entry type", http.StatusBadRequest)
		return
	}

	entryID := fmt.Sprintf("%s%s_%d", typePrefix, chemicalID, time.Now().UnixNano())

	// Insert the entry using the custom entry ID and the chemicalID
	insertentryQuery := `
        INSERT INTO entrys (id, type, date, chemical_id, no_of_units, quantity_per_unit, unit, remark, voucher_no)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    `
	_, err = db.Db.Exec(insertentryQuery, entryID, entry.Type, entry.Date, chemicalID,
		entry.NumOfUnits, entry.QuantityPerUnit, entry.Scale, entry.Remark, entry.VoucherNo)
	if err != nil {
		log.Println("Error inserting entry:", err)
		http.Error(w, "Failed to insert entry: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintln(w, `{"message": "Chemical and entry inserted successfully", "entry_id": "`+entryID+`"}`)
}
