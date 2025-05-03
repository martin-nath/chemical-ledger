package utils

import (
	"chemical-ledger-backend/db"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Decodes the JSON request body into the given object
func DecodeJsonReq(r *http.Request, obj any) ErrorMessage {
	err := json.NewDecoder(r.Body).Decode(obj)
	if err != nil {
		slog.Error(err.Error())
		return REQUEST_BODY_DECODE_ERR
	}
	return NO_ERR
}

// Encodes the given object into JSON and writes it to the response
func EncodeJsonRes(w http.ResponseWriter, status int, obj *Resp) error {
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(obj)
}

// Encodes the given error into JSON and writes it to the response
func RespWithError(w http.ResponseWriter, status int, errStr ErrorMessage) {
	EncodeJsonRes(w, status, NewRespWithError(errStr))
}

// Encodes the given data into JSON and writes it to the response
func RespWithData(w http.ResponseWriter, status int, data any) {
	EncodeJsonRes(w, status, NewRespWithData(data))
}

// Gets the value of the given parameter from the URL query string
func GetParam(r *http.Request, param string) string {
	return r.URL.Query().Get(param)
}

// Gets the value of the given parameter from the URL query string and converts it to an integer
func GetIntParam(r *http.Request, param string) (int, error) {
	str := GetParam(r, param)
	if str == "" {
		return 0, nil
	}
	num, err := strconv.Atoi(str)
	if err != nil {
		return 0, err
	}
	return num, nil
}

// Retries the given function up to a maximum of 1 time if first time it returns error
func IfErrRetry(f func() error) error {
	const (
		maxRetries = 1
		retryDelay = 100 * time.Millisecond
	)

	var err error
	for _ = range maxRetries + 1 {
		err = f()
		if err == nil {
			return nil
		}
		time.Sleep(retryDelay)
	}

	return err
}

// Gets the Unix timestamp of the given date with the current time
func GetDateUnix(date string) int64 {
	t, _ := time.Parse("2006-01-02", date)

	now := time.Now().Local()
	nowDate := time.Date(t.Year(), t.Month(), t.Day(), now.Hour(), now.Minute(), now.Second(), 0, now.Location())

	return nowDate.Unix()
}

func MergeDateWithUnixTime(dateStr string, unixTime int64) (int64, error) {
	// Define IST as +05:30
	ist := time.FixedZone("IST", 5*60*60+30*60)

	// Parse the date string in IST
	date, err := time.ParseInLocation("2006-01-02", dateStr, ist)
	if err != nil {
		return 0, fmt.Errorf("invalid date format: %w", err)
	}

	// Convert the Unix timestamp to time.Time in IST
	t := time.Unix(unixTime, 0).In(ist)

	// Merge the date with the time from the Unix timestamp
	merged := time.Date(
		date.Year(), date.Month(), date.Day(),
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond(),
		ist,
	)

	return merged.Unix(), nil
}

func UpdateNetStockFromTodayOnwards(tx *sql.Tx, compoundId string, date int64) ErrorMessage {
	var netStock int
	err := IfErrRetry(func() error {
		err := tx.QueryRow("SELECT net_stock FROM entry WHERE compound_id = ? AND date < ? ORDER BY date DESC LIMIT 1", compoundId, date).Scan(&netStock)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return errors.New("error retrieving previous stock")
		}
		return nil
	})

	if err != nil {
		slog.Error(fmt.Sprintf("Error retrieving previous stock for compound '%s': %v", compoundId, err))
		return STOCK_RETRIEVAL_ERR
	}

	var rows *sql.Rows
	err = IfErrRetry(func() error {
		var queryErr error
		rows, queryErr = tx.Query(`
SELECT
	e.id,
	e.type,
	q.num_of_units * q.quantity_per_unit,
	e.date
FROM entry e
JOIN quantity q ON e.quantity_id = q.id
WHERE
	e.compound_id = ? AND e.date >= ?
ORDER BY
	e.date ASC
		`, compoundId, date)
		return queryErr
	})

	if err != nil {
		slog.Error(fmt.Sprintf("Error retrieving subsequent entries for compound '%s': %v", compoundId, err))
		return ENTRY_RETRIEVAL_ERR
	}

	defer rows.Close()

	var updateQueriesBuilder strings.Builder
	for rows.Next() {
		var entry struct {
			Id       string
			Type     string
			Quantity int
			Date     int64
		}
		err := rows.Scan(&entry.Id, &entry.Type, &entry.Quantity, &entry.Date)
		if err != nil {
			return ENTRY_UPDATE_SCAN_ERR
		}

		switch entry.Type {
		case ENTRY_TYPE_INCOMING:
			netStock += entry.Quantity
		case ENTRY_TYPE_OUTGOING:
			netStock -= entry.Quantity
		}

		if netStock < 0 {
			return INSUFFICIENT_STOCK_ERR
		}
		updateQueriesBuilder.WriteString(fmt.Sprintf("UPDATE entry SET net_stock = %d WHERE id = '%s';\n", netStock, entry.Id))
	}

	updateQueries := updateQueriesBuilder.String()
	if updateQueries != "" {
		_, err = tx.Exec(updateQueries)
		if err != nil {
			return SUBSEQUENT_UPDATE_ERR
		}
	}

	return NO_ERR
}

func CheckIfCompoundExists(compoundId string) (bool, error) {
	var compoundExists bool
	err := IfErrRetry(func() error {
		return db.Conn.QueryRow("SELECT EXISTS(SELECT 1 FROM compound WHERE id = ?)", compoundId).Scan(&compoundExists)
	})

	if err != nil {
		return false, err
	}

	return compoundExists, nil
}

func CheckIfLowerCaseCompoundExists(lowerCasedName string) (bool, error) {
	var lowerCaseCompoundExists bool
	err := IfErrRetry(func() error {
		return db.Conn.QueryRow("SELECT EXISTS(SELECT 1 FROM compound WHERE lower_case_name = ?)", lowerCasedName).Scan(&lowerCaseCompoundExists)
	})

	if err != nil {
		return false, err
	}

	return lowerCaseCompoundExists, nil
}

func GetLowerCasedCompoundName(compoundName string) string {
	subStrs := strings.Split(compoundName, " ")
	for i, subStr := range subStrs {
		subStrs[i] = strings.ToLower(subStr)
	}
	return strings.Join(subStrs, "-")
}
