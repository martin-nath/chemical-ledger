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

const (
	entryIncoming = "incoming"
	entryOutgoing = "outgoing"
)

func InsertData(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var entry utils.Entry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, "Invalid JSON data: "+err.Error(), http.StatusBadRequest)
		return
	}

	fmt.Println(entry)

	// Date validation
	_, dateFormatErr := time.Parse("2006-01-02", entry.Date)
	if dateFormatErr != nil || entry.Date > time.Now().Format("2006-01-02") {
		http.Error(w, "Invalid date", http.StatusBadRequest)
		return
	}

	if entry.Type == "" || entry.CompoundName == "" || entry.QuantityPerUnit <= 0 || entry.Scale == "" || entry.NumOfUnits <= 0 || entry.Type != entryIncoming && entry.Type != entryOutgoing {
		http.Error(w, "Missing or invalid required fields", http.StatusBadRequest)
		return
	}

	chemicalID := utils.ToCamelCase(entry.CompoundName)
	txnQuantity := entry.NumOfUnits * entry.QuantityPerUnit

	var currentStock int
	compoundCheckErr := db.Db.QueryRow("SELECT net_stock FROM entry WHERE compound_id = ? ORDER BY date DESC LIMIT 1", chemicalID).Scan(&currentStock)

	if compoundCheckErr != nil && !errors.Is(compoundCheckErr, sql.ErrNoRows) {
		log.Println("Error checking stock:", compoundCheckErr)
		http.Error(w, "Database error: "+compoundCheckErr.Error(), http.StatusInternalServerError)
		return		
	}
	
	switch entry.Type {
	case entryOutgoing:
		if compoundCheckErr == nil {
			if currentStock < txnQuantity {
				http.Error(w, "Stock is less than requested outgoing quantity", http.StatusBadRequest)
				return
			}
			currentStock -= txnQuantity
			break
		}

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
		if compoundCheckErr == nil {
			currentStock += txnQuantity
			break
		}

		// Insert new compound
		insertCompound := `INSERT INTO compound (id, name, scale) VALUES (?, ?, ?)`
		_, err := db.Db.Exec(insertCompound, chemicalID, entry.CompoundName, entry.Scale)
		if err != nil {
			log.Println("Error inserting compound:", err)
			http.Error(w, "Failed to insert compound: "+err.Error(), http.StatusInternalServerError)
			return
		}
		currentStock = txnQuantity
	}

	// Generate IDs
	quantityID := fmt.Sprintf("Q_%s_%d", chemicalID, time.Now().UnixNano())
	entryID := fmt.Sprintf("%s%s_%d", map[string]string{"incoming": "I", "outgoing": "O"}[entry.Type], chemicalID, time.Now().UnixNano())

	// Insert into quantity
	insertQty := `INSERT INTO quantity (id, num_of_units, quantity_per_unit) VALUES (?, ?, ?)`
	_, compoundCheckErr = db.Db.Exec(insertQty, quantityID, entry.NumOfUnits, entry.QuantityPerUnit)
	if compoundCheckErr != nil {
		log.Println("Error inserting quantity:", compoundCheckErr)
		http.Error(w, "Failed to insert quantity: "+compoundCheckErr.Error(), http.StatusInternalServerError)
		return
	}

	// Insert into entry
	insertEntry := `
		INSERT INTO entry (id, type, date, compound_id, remark, voucher_no, quantity_id, net_stock)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, compoundCheckErr = db.Db.Exec(insertEntry, entryID, entry.Type, entry.Date, chemicalID, entry.Remark, entry.VoucherNo, quantityID, currentStock)
	if compoundCheckErr != nil {
		log.Println("Error inserting entry:", compoundCheckErr)
		http.Error(w, "Failed to insert entry: "+compoundCheckErr.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, `{"message": "Entry inserted successfully", "entry_id": "%s"}`, entryID)
}
