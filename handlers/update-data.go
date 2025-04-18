package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/martin-nath/chemical-ledger/db"
	"github.com/martin-nath/chemical-ledger/utils"
	"github.com/sirupsen/logrus"
)

func UpdateData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var entry utils.Entry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{
			Error: Id_req_body_decode_error,
		})
		logrus.Errorf("Error decoding request body: %v", err)
		return
	}

	logrus.Infof("Received request to update data: %+v", entry)

	if entry.CompoundId == "" || entry.QuantityPerUnit <= 0 || entry.NumOfUnits <= 0 || (entry.Type != entryIncoming && entry.Type != entryOutgoing) {
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{
			Error: Id_missingFields_or_inappropriate_value,
		})
		logrus.Warn("Missing or invalid required fields in the request.")
		return
	}

	entryDate, err := utils.UnixTimestamp(entry.Date)
	if err != nil {
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{
			Error: Id_invalid_date_format,
		})
		logrus.Warnf("Invalid date format provided: %s, error: %v", entry.Date, err)
		return
	}

	var dbTx *sql.Tx

	dbTx, err = db.Db.Begin()
	if err != nil {
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{
			Error: "Sorry, we're having trouble getting the stock information right now. Please try again later.",
		})
		logrus.Errorf("Error starting transaction: %v", err)
		return
	}
	defer dbTx.Commit()

	var oldType, oldDate, oldCompoundId string
	err = dbTx.QueryRow(`SELECT type FROM entry where id = ?`, entry.ID).Scan(&oldType)
	if err != nil {
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{
			Error: "Sorry, we're having trouble getting the stock information right now. Please try again later.",
		})
		logrus.Errorf("Error retrieving old entry type: %v", err)
		return
	}

	err = dbTx.QueryRow(`SELECT date FROM entry where id = ?`, entry.ID).Scan(&oldDate)
	if err != nil {
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{
			Error: "Sorry, we're having trouble getting the stock information right now. Please try again later.",
		})
		logrus.Errorf("Error retrieving old entry date: %v", err)
		return
	}

	err = dbTx.QueryRow(`SELECT compound_id FROM entry where id = ?, date < ?`, entry.ID, entryDate).Scan(&oldCompoundId)
	fmt.Println("Old compound ID:", oldCompoundId)
	if err != nil {
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{
			Error: "Sorry, we're having trouble getting the stock information right now. Please try again later.",
		})
		logrus.Errorf("Error retrieving old entry compound ID: %v", err)
		return
	}

	// Date parsing and validation
	parsedDate, err := time.Parse("02-01-2006", entry.Date)
	if err != nil {
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{
			Error: Id_invalid_date_format,
		})
		logrus.Warnf("Invalid date format provided: %s, error: %v", entry.Date, err)
		return
	}

	if parsedDate.After(time.Now()) {
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{
			Error: Id_future_date_error,
		})
		logrus.Warnf("Future date provided: %s", entry.Date)
		return
	}

	if entry.Type != oldType {
		utils.UpdateSubSequentNetStock(dbTx, entryDate, oldCompoundId, w)
		return
	}

	if entry.Date != oldDate {
		utils.UpdateSubSequentNetStock(dbTx, entryDate, oldCompoundId, w)
		return
	}

	if entry.CompoundId != oldCompoundId {

		var wg sync.WaitGroup

		wg.Add(2)
		go utils.UpdateSubSequentNetStock(dbTx, entryDate, oldCompoundId, w, &wg)
		go utils.UpdateSubSequentNetStock(dbTx, entryDate, oldCompoundId, w, &wg)

		wg.Wait()
		return
	}
}
