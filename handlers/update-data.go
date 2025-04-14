package handlers

import (
	"log"
	"net/http"
	"strconv"

	"github.com/martin-nath/chemical-ledger/db"
)

func UpdateEntryHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPut {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	entryIDStr := r.URL.Query().Get("entry_id")
	numOfUnitsStr := r.URL.Query().Get("num_of_units")
	quantityPerUnitStr := r.URL.Query().Get("quantity_per_unit")
	entryType := r.URL.Query().Get("new_type")

	if entryIDStr == "" && numOfUnitsStr == "" && quantityPerUnitStr == "" && entryType == "" {
		http.Error(w, "Missing required parameters", http.StatusBadRequest)
		return
	}

	entryID, err := strconv.Atoi(entryIDStr)
	if err != nil {
		http.Error(w, "Invalid entry ID", http.StatusBadRequest)
		return
	}
	numOfUnits, err := strconv.Atoi(numOfUnitsStr)
	if err != nil {
		http.Error(w, "Invalid num_of_units", http.StatusBadRequest)
		return
	}
	quantityPerUnit, err := strconv.Atoi(quantityPerUnitStr)
	if err != nil {
		http.Error(w, "Invalid quantity_per_unit", http.StatusBadRequest)
		return
	}

	// Start a transaction
	tx, err := db.Db.Begin()
	if err != nil {
		http.Error(w, "Failed to start transaction", http.StatusInternalServerError)
		log.Println("Transaction begin error:", err)
		return
	}
	defer tx.Rollback()

	// Get compound_id and date for the target entry
	var compoundID int
	var entryDate string
	err = tx.QueryRow(`SELECT compound_id, date FROM entry WHERE id = ?`, entryID).Scan(&compoundID, &entryDate)
	if err != nil {
		http.Error(w, "Entry not found", http.StatusNotFound)
		log.Println("Select error:", err)
		return
	}

	// Update the target entry
	_, err = tx.Exec(`UPDATE entry SET num_of_units = ?, quantity_per_unit = ?, type = ? WHERE id = ?`,
		numOfUnits, quantityPerUnit, entryType, entryID)
	if err != nil {
		http.Error(w, "Failed to update entry", http.StatusInternalServerError)
		log.Println("Update error:", err)
		return
	}

	// Fetch all entries of the same compound ordered by date, id
	rows, err := tx.Query(`
		SELECT id, type, num_of_units, quantity_per_unit
		FROM entry
		WHERE compound_id = ?
		ORDER BY date, id
	`, compoundID)
	if err != nil {
		http.Error(w, "Failed to fetch entries", http.StatusInternalServerError)
		log.Println("Query error:", err)
		return
	}
	defer rows.Close()

	// Prepare update statement
	updateStmt, err := tx.Prepare(`UPDATE entry SET net_stock = ? WHERE id = ?`)
	if err != nil {
		http.Error(w, "Failed to prepare update", http.StatusInternalServerError)
		log.Println("Prepare error:", err)
		return
	}
	defer updateStmt.Close()

	// Recalculate net_stock
	var netStock int
	for rows.Next() {
		var id, units, qty int
		var typ string
		if err := rows.Scan(&id, &typ, &units, &qty); err != nil {
			log.Println("Row scan error:", err)
			continue
		}

		if typ == "incoming" {
			netStock += units * qty
		} else if typ == "outgoing" {
			netStock -= units * qty
		}

		if _, err := updateStmt.Exec(netStock, id); err != nil {
			log.Println("Update net_stock failed for id", id, ":", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		log.Println("Commit error:", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Entry updated and net_stock recalculated successfully"))
}
