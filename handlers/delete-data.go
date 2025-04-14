package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"

	"github.com/martin-nath/chemical-ledger/db"
)

func DeleteEntryHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPut {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	entryIDStr := r.URL.Query().Get("id")
	if entryIDStr == "" {
		http.Error(w, "Missing entry ID", http.StatusBadRequest)
		return
	}

	entryID, err := strconv.Atoi(entryIDStr)
	if err != nil {
		http.Error(w, "Invalid entry ID", http.StatusBadRequest)
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

	// Get compound_id of the entry to be deleted
	var compoundID int
	err = tx.QueryRow(`SELECT compound_id FROM entry WHERE id = ?`, entryID).Scan(&compoundID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Entry not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to find entry", http.StatusInternalServerError)
			log.Println("Select error:", err)
		}
		return
	}

	// Delete the entry
	_, err = tx.Exec(`DELETE FROM entry WHERE id = ?`, entryID)
	if err != nil {
		http.Error(w, "Failed to delete entry", http.StatusInternalServerError)
		log.Println("Delete error:", err)
		return
	}

	// Recalculate net_stock for this compound
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

	updateStmt, err := tx.Prepare(`UPDATE entry SET net_stock = ? WHERE id = ?`)
	if err != nil {
		http.Error(w, "Failed to prepare update", http.StatusInternalServerError)
		log.Println("Prepare error:", err)
		return
	}
	defer updateStmt.Close()

	// Recalculate and update net_stock
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
			log.Println("Failed to update net_stock for id", id, ":", err)
		}
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		log.Println("Commit error:", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Entry deleted and net_stock recalculated successfully"))
}
