package utils

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

func ToCamelCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		words[i] = strings.ToLower(w)
		if i == 0 {
			continue
		}
		words[i] = strings.ToUpper(words[i][0:1]) + w[1:]
	}
	return strings.Join(words, "")
}

func UnixTimestamp(dateStr string) (int64, error) {
	// Parse input date
	date, err := time.Parse("02-01-2006", dateStr)
	if err != nil {
		return 0, err
	}

	// Get current time
	now := time.Now()

	// Combine date with current time (local)
	combined := time.Date(date.Year(), date.Month(), date.Day(),
		now.Hour(), now.Minute(), now.Second(), 0, time.Local)

	// Convert to Unix timestamp (UTC)
	return combined.Unix(), nil
}

type Resp struct {
	Error   string `json:"error"`
	Data    any    `json:"data"`
	Message string `json:"message"`
}

func JsonRes(w http.ResponseWriter, status int, resObj *Resp) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resObj)
}

func UpdateSubSequentNetStock(dbTx *sql.Tx, entryDate int64, compoundId string, w http.ResponseWriter) {
	var previousStock int
	err := dbTx.QueryRow("SELECT net_stock FROM entry WHERE compound_id = ? AND date < ? ORDER BY date DESC LIMIT 1", compoundId, entryDate).Scan(&previousStock)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		JsonRes(w, http.StatusInternalServerError, &Resp{
			Error: "Sorry, we're having trouble getting the stock information right now. Please try again later.",
		})
		logrus.Errorf("Error retrieving previous stock for compound '%s': %v", compoundId, err)
		return
	}

	rows, err := dbTx.Query(`
		SELECT
			e.id AS entry_id,
			e.type AS entry_type,
			q.num_of_units * q.quantity_per_unit AS quantity,
			e.date AS entry_date,
			e.net_stock AS current_net_stock
		FROM entry e
		JOIN compound c ON e.compound_id = c.id
		JOIN quantity q ON e.quantity_id = q.id
		WHERE
			e.compound_id = ? AND e.date >= ?
		ORDER BY
			e.date ASC
	`, compoundId, entryDate)
	if err != nil {
		JsonRes(w, http.StatusInternalServerError, &Resp{
			Error: "Sorry, we're having trouble getting the stock information right now. Please try again later.",
		})
		logrus.Errorf("Error retrieving subsequent entries for compound '%s': %v", compoundId, err)
		return
	}
	defer rows.Close()

	var updateQueriesBuilder strings.Builder
	for rows.Next() {
		var entryUpdate struct {
			EntryID         string
			EntryType       string
			Quantity        int
			EntryDate       int64
			CurrentNetStock int
		}
		err := rows.Scan(&entryUpdate.EntryID, &entryUpdate.EntryType, &entryUpdate.Quantity, &entryUpdate.EntryDate, &entryUpdate.CurrentNetStock)
		if err != nil {
			JsonRes(w, http.StatusInternalServerError, &Resp{
				Error: "Sorry, we're having trouble getting the stock information right now. Please try again later.",
			})
			logrus.Errorf("Error scanning subsequent entry: %v", err)
			return
		}

		switch entryUpdate.EntryType {
		case "incoming":
			previousStock += entryUpdate.Quantity
		case "outgoing":
			previousStock -= entryUpdate.Quantity
			if previousStock < 0 {
				JsonRes(w, http.StatusInternalServerError, &Resp{
					Error: "Sorry, we're having trouble getting the stock information right now. Please try again later.",
				})
				logrus.Errorf("Error calculating net stock after entry '%s': insufficient stock", entryUpdate.EntryID)
				return
			}
		}

		updateQueriesBuilder.WriteString(fmt.Sprintf("UPDATE entry SET net_stock = %d WHERE id = '%s';\n", previousStock, entryUpdate.EntryID))

	}

	updateQueries := updateQueriesBuilder.String()
	if updateQueries != "" {
		_, err = dbTx.Exec(updateQueries)
		if err != nil {
			JsonRes(w, http.StatusInternalServerError, &Resp{
				Error: "Sorry, we're having trouble getting the stock information right now. Please try again later.",
			})
			logrus.Errorf("Error updating subsequent entry net stock for compound '%s': %v", compoundId, err)
			return
		}
		logrus.Debugf("Updated net stock for subsequent entries of compound '%s'", compoundId)
	}
}
