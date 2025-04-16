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
	scaleMg       = "mg"
	scaleMl       = "ml"
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
	parsedDate, dateFormatErr := time.Parse("02-01-2006", entry.Date)
	if dateFormatErr != nil {
		http.Error(w, "Invalid date", http.StatusBadRequest)
		return
	}

	if parsedDate.After(time.Now()) {
		http.Error(w, "Date cannot be in the future", http.StatusBadRequest)
		return
	}
	
	EntryDate, err := utils.UnixTimestamp(entry.Date)
	fmt.Println(EntryDate)

	if err != nil {
		fmt.Println(error.Error(err))
		http.Error(w, "Invalid date 2", http.StatusBadRequest)
		return
	}

	if entry.CompoundName == "" || entry.QuantityPerUnit <= 0 || (entry.Scale != scaleMg && entry.Scale != scaleMl) || entry.NumOfUnits <= 0 || (entry.Type != entryIncoming && entry.Type != entryOutgoing) {
		http.Error(w, "Missing or invalid required fields", http.StatusBadRequest)
		return
	}

	compoundID := utils.ToCamelCase(entry.CompoundName)
	txnQuantity := entry.NumOfUnits * entry.QuantityPerUnit

	var currentStock int
	compoundCheckErr := db.Db.QueryRow("SELECT net_stock FROM entry WHERE compound_id = ? ORDER BY date DESC LIMIT 1", compoundID).Scan(&currentStock)
	compoundNotFound := errors.Is(compoundCheckErr, sql.ErrNoRows)
	// fmt.Println(currentStock)
	// fmt.Println(compoundCheckErr.Error())

	// tempCurrStock := currentStock

	switch entry.Type {
	case entryOutgoing:

		if compoundNotFound {
			http.Error(w, "Compound not found", http.StatusNotFound)
			return
		}

		if compoundCheckErr != nil {
			http.Error(w, "Database error: "+compoundCheckErr.Error(), http.StatusInternalServerError)
			return
		}

		if currentStock < txnQuantity {
			http.Error(w, "Stock is less than requested outgoing quantity", http.StatusBadRequest)
			return
		}

		fmt.Println("Before - Outgoing: ", currentStock)
		currentStock -= txnQuantity
		fmt.Println("After - Outgoing: ", currentStock)

	case entryIncoming:

		if compoundNotFound {
			// Insert new compound

			insertCompound := `INSERT INTO compound (id, name, scale) VALUES (?, ?, ?)`
			_, err := db.Db.Exec(insertCompound, compoundID, entry.CompoundName, entry.Scale)
			if err != nil {
				log.Println("Error inserting compound:", err)
				http.Error(w, "Failed to insert compound: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}

		if compoundCheckErr != nil && !compoundNotFound {
			http.Error(w, "Database error: "+compoundCheckErr.Error(), http.StatusInternalServerError)
			return
		}

		currentStock += txnQuantity

	}

	// Generate IDs
	quantityID := fmt.Sprintf("Q_%s_%d", compoundID, time.Now().UnixNano())
	entryID := fmt.Sprintf("%s%s_%d", map[string]string{"incoming": "I", "outgoing": "O"}[entry.Type], compoundID, time.Now().UnixNano())

	// Insert into quantity
	insertQty := `INSERT INTO quantity (id, num_of_units, quantity_per_unit) VALUES (?, ?, ?)`
	_, insertQtyErr := db.Db.Exec(insertQty, quantityID, entry.NumOfUnits, entry.QuantityPerUnit)
	if insertQtyErr != nil {
		log.Println("Error inserting quantity:", compoundCheckErr)
		http.Error(w, "Failed to insert quantity: "+compoundCheckErr.Error(), http.StatusInternalServerError)
		return
	}

	// fmt.Println(currentStock)

	// Insert into entry
	insertEntry := `
		INSERT INTO entry (id, type, date, compound_id, remark, voucher_no, quantity_id, net_stock)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, entryInsertErr := db.Db.Exec(insertEntry, entryID, entry.Type, EntryDate, compoundID, entry.Remark, entry.VoucherNo, quantityID, currentStock)
	if entryInsertErr != nil {
		log.Println("Error inserting entry:", compoundCheckErr)
		http.Error(w, "Failed to insert entry: "+compoundCheckErr.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, `{"message": "Entry inserted successfully", "entry_id": "%s"}`, entryID)
}

// func InsertData(w http.ResponseWriter, r *http.Request) {
// 	if r.Method != http.MethodPost {
// 		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
// 		return
// 	}

// 	var entry utils.Entry
// 	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
// 		http.Error(w, "Invalid JSON data: "+err.Error(), http.StatusBadRequest)
// 		return
// 	}

// 	if entry.Type != "incoming" && entry.Type != "outgoing" {
// 		http.Error(w, "Invalid type", http.StatusBadRequest)
// 		return
// 	}

// 	// Parse and validate date
// 	entryDate, err := time.Parse("2006-01-02", entry.Date)
// 	if err != nil || entryDate.After(time.Now()) {
// 		http.Error(w, "Invalid date", http.StatusBadRequest)
// 		return
// 	}

// 	if entry.Type == "" || entry.CompoundName == "" || entry.QuantityPerUnit <= 0 || entry.Scale == "" || entry.NumOfUnits <= 0 || (entry.Type != entryIncoming && entry.Type != entryOutgoing) {
// 		http.Error(w, "Missing or invalid required fields", http.StatusBadRequest)
// 		return
// 	}

// 	chemicalID := utils.ToCamelCase(entry.CompoundName)
// 	txnQuantity := entry.NumOfUnits * entry.QuantityPerUnit

// 	tx, err := db.Db.Begin()
// 	if err != nil {
// 		http.Error(w, "Failed to start transaction: "+err.Error(), http.StatusInternalServerError)
// 		return
// 	}
// 	defer tx.Rollback() // rollback on panic or error

// 	// var currentStock int
// 	// err = tx.QueryRow("SELECT net_stock FROM entry WHERE compound_id = ? ORDER BY date DESC LIMIT 1", chemicalID).Scan(&currentStock)
// 	// if err != nil && !errors.Is(err, sql.ErrNoRows) {
// 	// 	http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
// 	// 	return
// 	// }

// 	var currentStock int
// 	err = tx.QueryRow("SELECT net_stock FROM entry WHERE compound_id = ? ORDER BY date DESC LIMIT 1", chemicalID).Scan(&currentStock)

// 	if !errors.Is(err, sql.ErrNoRows) {

// 	if err != nil {
// 			http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
// 			return
// 	}

// 	switch entry.Type {
// 	case entryOutgoing:
// 		// Check compound exists
// 		var exists bool
// 		err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM compound WHERE id = ?)", chemicalID).Scan(&exists)
// 		if err != nil {
// 			tx.Rollback()
// 			http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
// 			return
// 		}
// 		if !exists {
// 			tx.Rollback()
// 			http.Error(w, "Compound not found", http.StatusNotFound)
// 			return
// 		}
// 		if currentStock < txnQuantity {
// 			tx.Rollback()
// 			http.Error(w, "Insufficient stock", http.StatusBadRequest)
// 			return
// 		}
// 		currentStock -= txnQuantity

// 	case entryIncoming:
// 		// Check compound exists before inserting
// 		var exists bool
// 		err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM compound WHERE id = ?)", chemicalID).Scan(&exists)
// 		if err != nil {
// 			tx.Rollback()
// 			http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
// 			return
// 		}
// 		if !exists {
// 			_, err := tx.Exec(`INSERT INTO compound (id, name, scale) VALUES (?, ?, ?)`, chemicalID, entry.CompoundName, entry.Scale)
// 			if err != nil {
// 				tx.Rollback()
// 				http.Error(w, "Failed to insert compound: "+err.Error(), http.StatusInternalServerError)
// 				return
// 			}
// 		}
// 		currentStock += txnQuantity
// 	}

// 	quantityID := fmt.Sprintf("Q_%s_%d", chemicalID, time.Now().UnixNano())
// 	entryID := fmt.Sprintf("%s%s_%d", map[string]string{"incoming": "I", "outgoing": "O"}[entry.Type], chemicalID, time.Now().UnixNano())

// 	_, err = tx.Exec(`INSERT INTO quantity (id, num_of_units, quantity_per_unit) VALUES (?, ?, ?)`, quantityID, entry.NumOfUnits, entry.QuantityPerUnit)
// 	if err != nil {
// 		http.Error(w, "Failed to insert quantity: "+err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	_, err = tx.Exec(`
// 		INSERT INTO entry (id, type, date, compound_id, remark, voucher_no, quantity_id, net_stock)
// 		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
// 		entryID, entry.Type, entryDate.Format("2006-01-02"), chemicalID, entry.Remark, entry.VoucherNo, quantityID, currentStock)
// 	if err != nil {
// 		http.Error(w, "Failed to insert entry: "+err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	if err := tx.Commit(); err != nil {
// 		http.Error(w, "Transaction commit failed: "+err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	w.WriteHeader(http.StatusCreated)
// 	fmt.Fprintf(w, `{"message": "Entry inserted successfully", "entry_id": "%s"}`, entryID)
// }
// }
