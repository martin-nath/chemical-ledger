package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time" // For generating unique IDs

	"github.com/martin-nath/chemical-ledger/db" // Assuming your db package is here
)

// UpdateEntryHandler handles the update of an entry and recalculates subsequent net stock only if necessary.
func UpdateEntryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	entryID := r.URL.Query().Get("entry_id")
	newNumOfUnitsStr := r.URL.Query().Get("num_of_units")
	newQuantityPerUnitStr := r.URL.Query().Get("quantity_per_unit")
	newType := r.URL.Query().Get("new_type")
	newCompoundName := r.URL.Query().Get("new_compound_name")

	if entryID == "" {
		http.Error(w, "Missing required parameter: entry_id", http.StatusBadRequest)
		return
	}

	tx, err := db.Db.Begin()
	if err != nil {
		http.Error(w, "Failed to start transaction", http.StatusInternalServerError)
		log.Println("Transaction begin error:", err)
		return
	}
	defer tx.Rollback()

	var oldEntry struct {
		CompoundID        string
		Date              string
		QuantityID        string
		OldNumOfUnits     int
		OldQuantityPerUnit int
		OldType           string
		OldCompoundName   string
		OldNetStock       int
	}

	// Get the current entry details and compound name
	err = tx.QueryRow(`
		SELECT e.compound_id, e.date, e.quantity_id, q.num_of_units, q.quantity_per_unit, e.type, c.name, e.net_stock
		FROM entry e
		JOIN quantity q ON e.quantity_id = q.id
		JOIN compound c ON e.compound_id = c.id
		WHERE e.id = ?
	`, entryID).Scan(
		&oldEntry.CompoundID, &oldEntry.Date, &oldEntry.QuantityID,
		&oldEntry.OldNumOfUnits, &oldEntry.OldQuantityPerUnit, &oldEntry.OldType,
		&oldEntry.OldCompoundName, &oldEntry.OldNetStock,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Entry not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to retrieve entry details", http.StatusInternalServerError)
		log.Println("Error retrieving entry:", err)
		return
	}

	needsUpdate := false
	newCompoundID := oldEntry.CompoundID
	newNumOfUnits := oldEntry.OldNumOfUnits
	newQuantityPerUnit := oldEntry.OldQuantityPerUnit
	newEntryType := oldEntry.OldType

	// --- Handle Compound Name Update (if provided) ---
	if newCompoundName != "" && newCompoundName != oldEntry.OldCompoundName {
		needsUpdate = true
		var existingCompoundID string
		err = tx.QueryRow("SELECT id FROM compound WHERE name = ?", newCompoundName).Scan(&existingCompoundID)
		if err == sql.ErrNoRows {
			// If the new compound doesn't exist, insert it
			newCompoundID = generateUniqueID() // Implement your unique ID generation
			_, err = tx.Exec("INSERT INTO compound (id, name, scale) VALUES (?, ?, 'mg')", newCompoundID, newCompoundName)
			if err != nil {
				http.Error(w, "Failed to insert new compound", http.StatusInternalServerError)
				log.Println("Error inserting new compound:", err)
				return
			}
		} else if err != nil {
			http.Error(w, "Failed to query new compound", http.StatusInternalServerError)
			log.Println("Error querying new compound:", err)
			return
		} else {
			newCompoundID = existingCompoundID
		}

		// Recalculate net stock for the old compound's subsequent entries
		oldStockChange := oldEntry.OldNumOfUnits * oldEntry.OldQuantityPerUnit * getStockMultiplier(oldEntry.OldType)
		err = updateSubsequentNetStock(tx, oldEntry.CompoundID, oldEntry.Date, -oldStockChange) // Subtract the effect of this entry from the old compound
		if err != nil {
			http.Error(w, "Failed to update subsequent net stock for old compound", http.StatusInternalServerError)
			log.Println("Error updating subsequent net stock (old):", err)
			return
		}

		// Recalculate net stock for the new compound's subsequent entries
		newStockChange := oldEntry.OldNumOfUnits * oldEntry.OldQuantityPerUnit * getStockMultiplier(oldEntry.OldType)
		err = updateSubsequentNetStock(tx, newCompoundID, oldEntry.Date, newStockChange) // Add the effect of this entry to the new compound
		if err != nil {
			http.Error(w, "Failed to update subsequent net stock for new compound", http.StatusInternalServerError)
			log.Println("Error updating subsequent net stock (new):", err)
			return
		}
	}

	// --- Handle Quantity Updates (if provided) ---
	if newNumOfUnitsStr != "" {
		newNumOfUnitsParsed, err := strconv.Atoi(newNumOfUnitsStr)
		if err != nil {
			http.Error(w, "Invalid num_of_units", http.StatusBadRequest)
			return
		}
		if newNumOfUnitsParsed != oldEntry.OldNumOfUnits {
			needsUpdate = true
			newNumOfUnits = newNumOfUnitsParsed
		}
	}

	if newQuantityPerUnitStr != "" {
		newQuantityPerUnitParsed, err := strconv.Atoi(newQuantityPerUnitStr)
		if err != nil {
			http.Error(w, "Invalid quantity_per_unit", http.StatusBadRequest)
			return
		}
		if newQuantityPerUnitParsed != oldEntry.OldQuantityPerUnit {
			needsUpdate = true
			newQuantityPerUnit = newQuantityPerUnitParsed
		}
	}

	// --- Handle Type Update (if provided) ---
	if newType != "" {
		if newType != "incoming" && newType != "outgoing" {
			http.Error(w, "Invalid entry type", http.StatusBadRequest)
			return
		}
		if newType != oldEntry.OldType {
			needsUpdate = true
			newEntryType = newType
		}
	}

	if needsUpdate {
		// Calculate the new net stock for the current entry
		currentStockChange := newNumOfUnits * newQuantityPerUnit
		if newEntryType == "outgoing" {
			currentStockChange = -currentStockChange
		}
		newNetStock := oldEntry.OldNetStock + (currentStockChange - (oldEntry.OldNumOfUnits * oldEntry.OldQuantityPerUnit * getStockMultiplier(oldEntry.OldType)))

		// Update the quantity table if num_of_units or quantity_per_unit changed
		if newNumOfUnits != oldEntry.OldNumOfUnits || newQuantityPerUnit != oldEntry.OldQuantityPerUnit {
			_, err = tx.Exec(
				"UPDATE quantity SET num_of_units = ?, quantity_per_unit = ? WHERE id = ?",
				newNumOfUnits, newQuantityPerUnit, oldEntry.QuantityID,
			)
			if err != nil {
				http.Error(w, "Failed to update quantity", http.StatusInternalServerError)
				log.Println("Error updating quantity:", err)
				return
			}
		}

		// Update the entry table
		_, err = tx.Exec(
			"UPDATE entry SET compound_id = ?, type = ?, net_stock = ? WHERE id = ?",
			newCompoundID, newEntryType, newNetStock, entryID,
		)
		if err != nil {
			http.Error(w, "Failed to update entry", http.StatusInternalServerError)
			log.Println("Error updating entry:", err)
			return
		}

		// Recalculate net stock for subsequent entries of the (possibly new) compound
		stockDifference := newNetStock - oldEntry.OldNetStock // The difference in net stock for the current entry
		err = updateSubsequentNetStock(tx, newCompoundID, oldEntry.Date, stockDifference)
		if err != nil {
			http.Error(w, "Failed to update subsequent net stock", http.StatusInternalServerError)
			log.Println("Error updating subsequent net stock:", err)
			return
		}

		// Commit transaction
		if err := tx.Commit(); err != nil {
			http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
			log.Println("Commit error:", err)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Entry updated and subsequent net stock recalculated successfully"))

	} else {
		// No updates were necessary
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("No updates needed for this entry"))
	}
}

// InsertNewEntryHandler handles the insertion of a new entry and updates subsequent net stock.
func InsertNewEntryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	compoundID := r.URL.Query().Get("compound_id")
	entryDate := r.URL.Query().Get("date")
	numOfUnitsStr := r.URL.Query().Get("num_of_units")
	quantityPerUnitStr := r.URL.Query().Get("quantity_per_unit")
	entryType := r.URL.Query().Get("type")
	remark := r.URL.Query().Get("remark")      // Optional
	voucherNo := r.URL.Query().Get("voucher_no") // Optional

	if compoundID == "" || entryDate == "" || numOfUnitsStr == "" || quantityPerUnitStr == "" || entryType == "" {
		http.Error(w, "Missing required parameters", http.StatusBadRequest)
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

	if entryType != "incoming" && entryType != "outgoing" {
		http.Error(w, "Invalid entry type", http.StatusBadRequest)
		return
	}

	tx, err := db.Db.Begin()
	if err != nil {
		http.Error(w, "Failed to start transaction", http.StatusInternalServerError)
		log.Println("Transaction begin error:", err)
		return
	}
	defer tx.Rollback()

	err = insertNewEntryAndUpdateNetStock(tx, compoundID, entryDate, numOfUnits, quantityPerUnit, entryType, remark, voucherNo)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to insert new entry: %v", err), http.StatusInternalServerError)
		log.Println("Error inserting new entry:", err)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		log.Println("Commit error:", err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("New entry created successfully and net stock updated."))
}

// insertNewEntryAndUpdateNetStock inserts a new entry and updates the net stock of subsequent entries.
func insertNewEntryAndUpdateNetStock(tx *sql.Tx, compoundID string, entryDate string, numOfUnits int, quantityPerUnit int, entryType string, remark string, voucherNo string) error {
	// 1. Generate a unique ID for the new entry and quantity
	newEntryID := generateUniqueID()
	newQuantityID := generateUniqueID()

	// 2. Calculate the stock change for the new entry
	stockChange := numOfUnits * quantityPerUnit * getStockMultiplier(entryType)

	// 3. Insert the new quantity
	_, err := tx.Exec("INSERT INTO quantity (id, num_of_units, quantity_per_unit) VALUES (?, ?, ?)", newQuantityID, numOfUnits, quantityPerUnit)
	if err != nil {
		return fmt.Errorf("failed to insert quantity: %w", err)
	}

	// 4. Determine the net stock for the new entry based on the preceding entry
	var precedingNetStock int
	err = tx.QueryRow("SELECT net_stock FROM entry WHERE compound_id = ? AND date < ? ORDER BY date DESC LIMIT 1", compoundID, entryDate).Scan(&precedingNetStock)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to retrieve preceding net stock: %w", err)
	}
	newNetStock := precedingNetStock + stockChange

	// 5. Insert the new entry
	_, err = tx.Exec(
		"INSERT INTO entry (id, type, compound_id, date, remark, voucher_no, quantity_id, net_stock) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		newEntryID, entryType, compoundID, entryDate, remark, voucherNo, newQuantityID, newNetStock,
	)
	if err != nil {
		return fmt.Errorf("failed to insert entry: %w", err)
	}

	// 6. Update the net stock of subsequent entries
	stmt, err := tx.Prepare(`
		UPDATE entry
		SET net_stock = net_stock + ?
		WHERE compound_id = ? AND date >= ? AND id != ?
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare update statement: %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(stockChange, compoundID, entryDate, newEntryID)
	if err != nil {
		return fmt.Errorf("failed to update subsequent net stock: %w", err)
	}

	return nil
}

// updateSubsequentNetStock updates the net stock of all subsequent entries for a given compound.
func updateSubsequentNetStock(tx *sql.Tx, compoundID string, updatedDate string, stockDifference int) error {
	stmt, err := tx.Prepare(`
		UPDATE entry
		SET net_stock = net_stock + ?
		WHERE compound_id = ? AND date > ?
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(stockDifference, compoundID, updatedDate)
	return err
}

// Helper function to generate a unique ID (replace with your actual implementation)
func generateUniqueID() string {
	return fmt.Sprintf("uid-%d", time.Now().UnixNano())
}

// Helper function to get the stock multiplier based on entry type
func getStockMultiplier(entryType string) int {
	if entryType == "incoming" {
		return 1
	} else if entryType == "outgoing" {
		return -1
	}
	return 0 // Should not happen based on the CHECK constraint
}