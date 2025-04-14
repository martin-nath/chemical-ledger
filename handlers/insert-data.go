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
	"github.com/martin-nath/chemical-ledger/utils"
)

func InsertData(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost && r.Method != http.MethodDelete && r.Method != http.MethodPut {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var entry utils.Entry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, "Invalid JSON data: "+err.Error(), http.StatusBadRequest)
		return
	}

	fmt.Println(entry)

	if r.Method == http.MethodDelete {
		// Delete entry
		deleteEntry := `DELETE FROM compound WHERE id = ?;`
		_, err := db.Db.Exec(deleteEntry, entry.ID)
		if err != nil {
			log.Println("Error deleting entry:", err)
			http.Error(w, "Failed to delete entry: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"message": "Entry deleted successfully", "entry_id": "%s"}`, entry.ID)
		return
	}

	// Date validation
	_, dateFormatErr := time.Parse("2006-01-02", entry.Date)
	if dateFormatErr != nil || entry.Date > time.Now().Format("2006-01-02") {
		http.Error(w, "Invalid date", http.StatusBadRequest)
		return
	}

	if entry.Type == "" || entry.CompoundName == "" || entry.QuantityPerUnit <= 0 || entry.Scale == "" || entry.NumOfUnits <= 0 {
		http.Error(w, "Missing or invalid required fields", http.StatusBadRequest)
		return
	}

	chemicalID := utils.ToCamelCase(entry.CompoundName)
	txnQuantity := entry.NumOfUnits * entry.QuantityPerUnit

	const (
		entryIncoming = "incoming"
		entryOutgoing = "outgoing"
	)

	var currentStock int
	err := db.Db.QueryRow("SELECT net_stock FROM entry WHERE compound_id = ? ORDER BY date DESC LIMIT 1", chemicalID).Scan(&currentStock)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Chemical not in entry log
		switch entry.Type {
		case entryOutgoing:
			// Check if the compound exists in the compound table
			var compoundExists bool
			err := db.Db.QueryRow("SELECT EXISTS(SELECT 1 FROM compound WHERE id = ?)", chemicalID).Scan(&compoundExists)
			if err != nil {
				log.Println("Error checking compound existence:", err)
				http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
				return
			}

			if !compoundExists {
				http.Error(w, "Compound not found", http.StatusNotFound)
				return
			}

		case entryIncoming:
			// Insert new compound
			insertCompound := `INSERT INTO compound (id, name, scale) VALUES (?, ?, ?)`
			_, err := db.Db.Exec(insertCompound, chemicalID, entry.CompoundName, entry.Scale)
			if err != nil {
				log.Println("Error inserting compound:", err)
				http.Error(w, "Failed to insert compound: "+err.Error(), http.StatusInternalServerError)
				return
			}
			currentStock = txnQuantity

		default:
			http.Error(w, "Invalid entry type", http.StatusBadRequest)
			return
		}

	case err != nil:
		log.Println("Error checking stock:", err)
		http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
		return

	default:
		// Existing compound
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
	}

	// Generate IDs
	quantityID := fmt.Sprintf("Q_%s_%d", chemicalID, time.Now().UnixNano())
	entryID := fmt.Sprintf("%s%s_%d", map[string]string{"incoming": "I", "outgoing": "O"}[entry.Type], chemicalID, time.Now().UnixNano())

	// Insert into quantity
	insertQty := `INSERT INTO quantity (id, num_of_units, quantity_per_unit) VALUES (?, ?, ?)`
	_, err = db.Db.Exec(insertQty, quantityID, entry.NumOfUnits, entry.QuantityPerUnit)
	if err != nil {
		log.Println("Error inserting quantity:", err)
		http.Error(w, "Failed to insert quantity: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Insert into entry
	insertEntry := `
		INSERT INTO entry (id, type, date, compound_id, remark, voucher_no, quantity_id, net_stock)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = db.Db.Exec(insertEntry, entryID, entry.Type, entry.Date, chemicalID, entry.Remark, entry.VoucherNo, quantityID, currentStock)
	if err != nil {
		log.Println("Error inserting entry:", err)
		http.Error(w, "Failed to insert entry: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, `{"message": "Entry inserted successfully", "entry_id": "%s"}`, entryID)
}
