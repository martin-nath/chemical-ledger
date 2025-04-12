package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/martin-nath/chemical-ledger/db"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func toCamelCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		words[i] = cases.Title(language.Und, cases.NoLower).String(w)
	}
	return strings.Join(words, "")
}

func InsertChemical(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var transaction db.Transaction
	if err := json.NewDecoder(r.Body).Decode(&transaction); err != nil {
		http.Error(w, "Invalid JSON data: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Set default for NoOfUnits if not provided (or <= 0)
	if transaction.NoOfUnits <= 0 {
		transaction.NoOfUnits = 1
	}

	// Validate required fields
	if transaction.Type == "" || transaction.Date == "" || transaction.CompoundName == "" ||
		transaction.QuantityPerUnit <= 0 || transaction.Unit == "" {
		http.Error(w, "Missing or invalid required fields", http.StatusBadRequest)
		return
	}

	// Compute chemical ID from chemical name in CamelCase
	chemicalID := toCamelCase(transaction.CompoundName)

	// Calculate total transaction quantity (units * quantity per unit)
	txnQuantity := transaction.NoOfUnits * transaction.QuantityPerUnit

	// Check if the chemical exists and get its current net_stock
	var currentStock int
	err := db.Db.QueryRow("SELECT net_stock FROM chemicals WHERE id = ?", chemicalID).Scan(&currentStock)
	if err != nil {
		if err == sql.ErrNoRows {
			// Chemical does not exist. For an Incoming transaction, insert it with initial stock.
			// For Outgoing, you cannot subtract stock from nothing.
			if transaction.Type == "Outgoing" {
				http.Error(w, "Cannot process outgoing transaction: stock is less", http.StatusBadRequest)
				return
			}
			// Insert new chemical with net_stock = txnQuantity
			insertChemicalQuery := `
                INSERT INTO chemicals (id, name, net_stock)
                VALUES (?, ?, ?)
            `
			_, err := db.Db.Exec(insertChemicalQuery, chemicalID, transaction.CompoundName, txnQuantity)
			if err != nil {
				log.Println("Error inserting chemical:", err)
				http.Error(w, "Failed to insert chemical: "+err.Error(), http.StatusInternalServerError)
				return
			}
			currentStock = txnQuantity // new stock set
		} else {
			log.Println("Error fetching chemical:", err)
			http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// Chemical exists; update net_stock based on transaction type
		if transaction.Type == "Incoming" {
			currentStock += txnQuantity
		} else if transaction.Type == "Outgoing" {
			if currentStock < txnQuantity {
				http.Error(w, "Stock is less than requested outgoing quantity", http.StatusBadRequest)
				return
			}
			currentStock -= txnQuantity
		} else {
			http.Error(w, "Invalid transaction type", http.StatusBadRequest)
			return
		}
		// Update the chemical's net_stock
		_, err := db.Db.Exec("UPDATE chemicals SET net_stock = ? WHERE id = ?", currentStock, chemicalID)
		if err != nil {
			log.Println("Error updating net_stock:", err)
			http.Error(w, "Failed to update net_stock: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Generate a custom transaction ID:
	// Start with "I" if Incoming or "O" if Outgoing plus the chemicalID and a Unix timestamp.
	var typePrefix string
	if transaction.Type == "Incoming" {
		typePrefix = "I"
	} else if transaction.Type == "Outgoing" {
		typePrefix = "O"
	} else {
		http.Error(w, "Invalid transaction type", http.StatusBadRequest)
		return
	}
	transactionID := fmt.Sprintf("%s%s_%d", typePrefix, chemicalID, time.Now().UnixNano())

	// Insert the transaction using the custom transaction ID and the chemicalID
	insertTransactionQuery := `
        INSERT INTO transactions (id, type, date, chemical_id, no_of_units, quantity_per_unit, unit, remark, voucher_no)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    `
	_, err = db.Db.Exec(insertTransactionQuery, transactionID, transaction.Type, transaction.Date, chemicalID,
		transaction.NoOfUnits, transaction.QuantityPerUnit, transaction.Unit, transaction.Remark, transaction.VoucherNo)
	if err != nil {
		log.Println("Error inserting transaction:", err)
		http.Error(w, "Failed to insert transaction: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintln(w, `{"message": "Chemical and transaction inserted successfully", "transaction_id": "`+transactionID+`"}`)
}

// FetchTransactions fetches transactions based on JSON filters.
// Expect JSON body even though GET is more conventionally paired with query parameters.
func FetchTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// Decode JSON body into a filter struct
	var filter struct {
		Type         string `json:"type"`
		CompoundName string `json:"compound_name"`
		FromDate     string `json:"from"`
		ToDate       string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&filter); err != nil {
		http.Error(w, "Invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Build the query dynamically
	query := `
        SELECT t.id, t.type, t.date, c.name AS compound_name,
               t.no_of_units, t.quantity_per_unit, t.unit, t.remark, t.voucher_no
        FROM transactions t
        JOIN chemicals c ON t.chemical_id = c.id
        WHERE 1=1
    `
	args := []any{}

	if filter.Type != "" && filter.Type != "Both" {
		query += " AND t.type = ?"
		args = append(args, filter.Type)
	}
	if filter.CompoundName != "" {
		// Convert filter compound name to camelCase so it matches the chemicals.id stored in transactions
		query += " AND c.id = ?"
		args = append(args, toCamelCase(filter.CompoundName))
	}
	if filter.FromDate != "" && filter.ToDate != "" {
		query += " AND DATE(t.date) BETWEEN DATE(?) AND DATE(?)"
		args = append(args, filter.FromDate, filter.ToDate)
	} else if filter.FromDate != "" {
		query += " AND DATE(t.date) = DATE(?)"
		args = append(args, filter.FromDate)
	}

	rows, err := db.Db.Query(query, args...)
	if err != nil {
		log.Println("Error executing query:", err)
		http.Error(w, "Database query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var transactions []db.Transaction
	for rows.Next() {
		var transaction db.Transaction
		err := rows.Scan(
			&transaction.ID, &transaction.Type, &transaction.Date, &transaction.CompoundName,
			&transaction.NoOfUnits, &transaction.QuantityPerUnit, &transaction.Unit, &transaction.Remark, &transaction.VoucherNo,
		)
		if err != nil {
			log.Println("Error scanning row:", err)
			http.Error(w, "Error scanning row: "+err.Error(), http.StatusInternalServerError)
			return
		}
		transactions = append(transactions, transaction)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(transactions)
}
